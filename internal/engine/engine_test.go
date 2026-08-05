package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newDB creates a temp SQLite file with schema executed and returns its path.
func newDB(t *testing.T, schema string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	return path
}

// countRows returns how many rows a query over path returns.
func countRows(t *testing.T, path, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// writeFile writes data to path, failing the test on error.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// readFile returns the contents of path, failing the test on error.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSQLDeleteCascadesInOneTxn(t *testing.T) {
	path := newDB(t, `
CREATE TABLE session (id TEXT PRIMARY KEY, title TEXT);
CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE);
CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL REFERENCES message(id) ON DELETE CASCADE);
INSERT INTO session VALUES ('s1','a'),('s2','b');
INSERT INTO message VALUES ('m1','s1'),('m2','s1'),('m3','s2');
INSERT INTO part VALUES ('p1','m1'),('p2','m2');`)
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "s1", Agent: "OpenCode",
		Actions: []Action{{Kind: SQLDelete, Store: path, SQL: "DELETE FROM session WHERE id = ?", Args: []any{"s1"}}},
	}}}
	res := Execute(context.Background(), plan)
	if res.Deleted() != 1 {
		t.Fatalf("deleted = %d, want 1 (%v)", res.Deleted(), res.Sessions[0].Err)
	}
	if n := countRows(t, path, "SELECT COUNT(*) FROM session"); n != 1 {
		t.Fatalf("session rows = %d, want 1", n)
	}
	if n := countRows(t, path, "SELECT COUNT(*) FROM message"); n != 1 {
		t.Fatalf("message rows = %d, want 1 (cascade should delete s1's messages)", n)
	}
	if n := countRows(t, path, "SELECT COUNT(*) FROM part"); n != 0 {
		t.Fatalf("part rows = %d, want 0 (cascade should delete through message)", n)
	}
	if n := countRows(t, path, "SELECT COUNT(*) FROM message WHERE session_id='s2'"); n != 1 {
		t.Fatalf("s2 message rows = %d, want 1 untouched", n)
	}
}

func TestMissingArtifactsAreNoop(t *testing.T) {
	dir := t.TempDir()
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "x", Agent: "Pi",
		Actions: []Action{
			{Kind: RemoveFile, Path: filepath.Join(dir, "gone.jsonl")},
			{Kind: RemoveTree, Path: filepath.Join(dir, "gone-dir")},
			{Kind: RemoveGlob, Path: filepath.Join(dir, "*.jsonl")},
			{Kind: DropJSONLLines, Path: filepath.Join(dir, "gone-index.jsonl"), IDs: []string{"a"}},
			{Kind: DropJSONKeys, Path: filepath.Join(dir, "gone-index.json"), IDs: []string{"a"}},
		},
	}}}
	res := Execute(context.Background(), plan)
	if res.Deleted() != 1 {
		t.Fatalf("deleted = %d, want 1 (missing artifacts must not fail): %v", res.Deleted(), res.Sessions[0].Err)
	}
}

func TestReRunIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-a.jsonl")
	writeFile(t, rollout, []byte("data"))
	path := newDB(t, "CREATE TABLE threads (id TEXT PRIMARY KEY); INSERT INTO threads VALUES ('a');")
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "a", Agent: "Codex",
		Actions: []Action{
			{Kind: RemoveFile, Path: rollout},
			{Kind: SQLDelete, Store: path, SQL: "DELETE FROM threads WHERE id = ?", Args: []any{"a"}},
		},
	}}}
	for run := 1; run <= 2; run++ {
		res := Execute(context.Background(), plan)
		if res.Deleted() != 1 || res.Failed() != 0 {
			t.Fatalf("run %d: deleted=%d failed=%d, want 1/0", run, res.Deleted(), res.Failed())
		}
		if _, err := os.Stat(rollout); !os.IsNotExist(err) {
			t.Fatalf("run %d: rollout still present", run)
		}
		if n := countRows(t, path, "SELECT COUNT(*) FROM threads"); n != 0 {
			t.Fatalf("run %d: threads rows = %d, want 0", run, n)
		}
	}
}

func TestContinueOnErrorKeepsRecordLast(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "s1.jsonl")
	file2 := filepath.Join(dir, "s2.jsonl")
	writeFile(t, file1, []byte("x"))
	writeFile(t, file2, []byte("x"))
	path := newDB(t, "CREATE TABLE sessions (id TEXT PRIMARY KEY); INSERT INTO sessions VALUES ('s1'),('s2'),('s3');")
	plan := &Plan{Sessions: []*SessionPlan{
		{ // fails on the record; the file action already ran (files first)
			ID: "s1", Agent: "Copilot",
			Actions: []Action{
				{Kind: RemoveFile, Path: file1},
				{Kind: SQLDelete, Store: path, SQL: "DELETE FROM missing_table WHERE id = ?", Args: []any{"s1"}},
			},
		},
		{
			ID: "s2", Agent: "Copilot",
			Actions: []Action{
				{Kind: RemoveFile, Path: file2},
				{Kind: SQLDelete, Store: path, SQL: "DELETE FROM sessions WHERE id = ?", Args: []any{"s2"}},
			},
		},
		{ // proves the sweep continues past a failed session
			ID: "s3", Agent: "Copilot",
			Actions: []Action{{Kind: SQLDelete, Store: path, SQL: "DELETE FROM sessions WHERE id = ?", Args: []any{"s3"}}},
		},
	}}
	res := Execute(context.Background(), plan)
	if res.Deleted() != 2 || res.Failed() != 1 {
		t.Fatalf("deleted=%d failed=%d, want 2/1", res.Deleted(), res.Failed())
	}
	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Fatal("s1 file must be gone: file actions run before the record fails")
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Fatal("s2 file must be gone")
	}
	// s1's record remains (visible ghost, self-heals on re-run); s2 and s3 are gone.
	if n := countRows(t, path, "SELECT COUNT(*) FROM sessions"); n != 1 {
		t.Fatalf("session rows = %d, want 1 (only s1 remains)", n)
	}
	if n := countRows(t, path, "SELECT COUNT(*) FROM sessions WHERE id='s1'"); n != 1 {
		t.Fatal("s1's record must be left in place after a failed record delete")
	}
}

func TestBusyStoreFailsSessionNotSweep(t *testing.T) {
	pathA := newDB(t, "CREATE TABLE t (id TEXT PRIMARY KEY); INSERT INTO t VALUES ('a');")
	pathB := newDB(t, "CREATE TABLE t (id TEXT PRIMARY KEY); INSERT INTO t VALUES ('b');")

	locker, err := sql.Open("sqlite", "file:"+pathA+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	locker.SetMaxOpenConns(1)
	if _, err := locker.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, rbErr := locker.Exec("ROLLBACK"); rbErr != nil {
			t.Errorf("release lock: %v", rbErr)
		}
		if err := locker.Close(); err != nil {
			t.Errorf("close locker: %v", err)
		}
	}()

	old := busyTimeout
	busyTimeout = 150 * time.Millisecond
	defer func() { busyTimeout = old }()

	plan := &Plan{Sessions: []*SessionPlan{
		{ID: "a", Agent: "X", Actions: []Action{{Kind: SQLDelete, Store: pathA, SQL: "DELETE FROM t WHERE id='a'"}}},
		{ID: "b", Agent: "X", Actions: []Action{{Kind: SQLDelete, Store: pathB, SQL: "DELETE FROM t WHERE id='b'"}}},
	}}
	res := Execute(context.Background(), plan)
	if res.Deleted() != 1 || res.Failed() != 1 {
		t.Fatalf("deleted=%d failed=%d, want 1/1 (busy store fails its session only)", res.Deleted(), res.Failed())
	}
	if n := countRows(t, pathA, "SELECT COUNT(*) FROM t"); n != 1 {
		t.Fatalf("locked store rows = %d, want 1 untouched", n)
	}
	if n := countRows(t, pathB, "SELECT COUNT(*) FROM t"); n != 0 {
		t.Fatalf("unlocked store rows = %d, want 0", n)
	}
}

func TestDropJSONLLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session_index.jsonl")
	writeFile(t, path, []byte("{\"id\":\"a\",\"n\":1}\n{\"id\":\"b\",\"n\":2}\n{\"id\":\"c\",\"n\":3}\n"))
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "b", Agent: "Codex",
		Actions: []Action{{Kind: DropJSONLLines, Path: path, IDs: []string{"b"}}},
	}}}
	Execute(context.Background(), plan)
	data := readFile(t, path)
	for _, id := range []string{"a", "c"} {
		if !strings.Contains(string(data), `"id":"`+id+`"`) {
			t.Fatalf("line for %s missing from index:\n%s", id, data)
		}
	}
	if strings.Contains(string(data), `"id":"b"`) {
		t.Fatalf("line for b still in index:\n%s", data)
	}
}

func TestDropJSONKeysObjectAndArray(t *testing.T) {
	obj := filepath.Join(t.TempDir(), "sessions-index.json")
	writeFile(t, obj, []byte(`{"a":{"s":1},"b":{"s":2},"c":{"s":3}}`))
	arr := filepath.Join(t.TempDir(), "arr.json")
	writeFile(t, arr, []byte(`[{"id":"a"},{"id":"b"},{"id":"c"}]`))
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "b", Agent: "Claude Code",
		Actions: []Action{
			{Kind: DropJSONKeys, Path: obj, IDs: []string{"b"}},
			{Kind: DropJSONKeys, Path: arr, IDs: []string{"b"}},
		},
	}}}
	Execute(context.Background(), plan)
	var m map[string]any
	raw := readFile(t, obj)
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("object index invalid: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("object index keys = %d, want 2", len(m))
	}
	var items []map[string]any
	raw = readFile(t, arr)
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("array index invalid: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("array index items = %d, want 2", len(items))
	}
}

func TestStripKV(t *testing.T) {
	path := newDB(t, `
CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB);
INSERT INTO ItemTable VALUES ('composer.composerData', '{"allComposers":[{"id":"c1"},{"id":"c2"},{"id":"c3"}],"selectedComposerIds":["c2"],"lastFocusedComposerIds":["c3"],"other":"x"}');`)
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "c2", Agent: "Cursor",
		Actions: []Action{{Kind: StripKV, Store: path, Table: "ItemTable", Column: "value", Key: "composer.composerData", IDs: []string{"c2"}}},
	}}}
	res := Execute(context.Background(), plan)
	if res.Deleted() != 1 {
		t.Fatalf("deleted = %d, want 1: %v", res.Deleted(), res.Sessions[0].Err)
	}
	var raw []byte
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow("SELECT value FROM ItemTable WHERE key='composer.composerData'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ids := func(key string) []string {
		list, _ := doc[key].([]any)
		var out []string
		for _, v := range list {
			m, _ := v.(map[string]any)
			id, _ := m["id"].(string)
			out = append(out, id)
		}
		return out
	}
	if got := ids("allComposers"); len(got) != 2 || got[0] != "c1" || got[1] != "c3" {
		t.Fatalf("allComposers = %v, want [c1 c3]", got)
	}
	selected, _ := doc["selectedComposerIds"].([]any)
	if len(selected) != 0 {
		t.Fatalf("selectedComposerIds = %v, want empty", selected)
	}
	focused, _ := doc["lastFocusedComposerIds"].([]any)
	if len(focused) != 1 {
		t.Fatalf("lastFocusedComposerIds = %v, want [c3]", focused)
	}
	if doc["other"] != "x" {
		t.Fatalf("unrelated field lost: other = %v, want x", doc["other"])
	}
}

func TestReclaimTotalsAndVacuumHint(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.jsonl")
	writeFile(t, file, []byte("x"))
	path := newDB(t, "CREATE TABLE t (id TEXT PRIMARY KEY);")
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "a", Agent: "X",
		Actions: []Action{
			{Kind: RemoveFile, Path: file, Bytes: 100},
			{Kind: SQLDelete, Store: path, SQL: "DELETE FROM t WHERE id='a'"},
		},
	}}}
	if got, want := plan.Reclaim(), int64(100); got != want {
		t.Fatalf("plan reclaim = %d, want %d", got, want)
	}
	res := Execute(context.Background(), plan)
	if res.BytesReclaimed() != 100 {
		t.Fatalf("bytes reclaimed = %d, want 100", res.BytesReclaimed())
	}
	if !res.NeedsVacuum() {
		t.Fatal("NeedsVacuum = false, want true after an SQL delete")
	}
	fileOnly := &Plan{Sessions: []*SessionPlan{{
		ID: "b", Agent: "Pi",
		Actions: []Action{{Kind: RemoveFile, Path: file, Bytes: 7}},
	}}}
	if res2 := Execute(context.Background(), fileOnly); res2.NeedsVacuum() {
		t.Fatal("NeedsVacuum = true for a files-only plan, want false")
	}
}

func TestDryRunPlanEqualsExecution(t *testing.T) {
	// A codex-shaped session: rollout file + shell snapshot + index line + row.
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-a.jsonl")
	shellDir := filepath.Join(dir, "shell_snapshots")
	if err := os.MkdirAll(shellDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, rollout, []byte("transcript"))
	writeFile(t, filepath.Join(shellDir, "a.sh"), []byte("env"))
	index := filepath.Join(dir, "session_index.jsonl")
	writeFile(t, index, []byte("{\"id\":\"a\",\"t\":1}\n{\"id\":\"b\",\"t\":2}\n"))
	path := newDB(t, "CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT); INSERT INTO threads VALUES ('a','rollout-a.jsonl');")
	plan := &Plan{Sessions: []*SessionPlan{{
		ID: "a", Agent: "Codex",
		Actions: []Action{
			{Kind: RemoveFile, Path: rollout, Bytes: 10},
			{Kind: RemoveGlob, Path: filepath.Join(shellDir, "*.sh"), Bytes: 4},
			{Kind: DropJSONLLines, Path: index, IDs: []string{"a"}},
			{Kind: SQLDelete, Store: path, SQL: "DELETE FROM threads WHERE id = ?", Args: []any{"a"}},
		},
	}}}
	// The dry-run number IS the plan's reclaim; the sweep must reclaim exactly that.
	if res := Execute(context.Background(), plan); res.BytesReclaimed() != plan.Reclaim() {
		t.Fatalf("bytes reclaimed %d != plan reclaim %d", res.BytesReclaimed(), plan.Reclaim())
	}
	if n := countRows(t, path, "SELECT COUNT(*) FROM threads"); n != 0 {
		t.Fatalf("threads rows = %d, want 0", n)
	}
	if data := readFile(t, index); strings.Contains(string(data), `"id":"a"`) {
		t.Fatalf("index line for a remains:\n%s", data)
	}
}
