package inventory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/testutil"
)

// fixNow is a fixed clock for scans; none of the readers depend on now yet.
var fixNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// writeNested writes data to root/name creating any parent directories.
func writeNested(t *testing.T, root, name string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasAction(plan *engine.SessionPlan, kind engine.ActionKind) bool {
	for _, a := range plan.Actions {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

func TestScanOpenCode(t *testing.T) {
	root := testutil.StoreRoot(t)
	testutil.SeedDB(t, root, "opencode.db", `
		CREATE TABLE session (id TEXT PRIMARY KEY, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER);
		INSERT INTO session VALUES ('sess-1', NULL, 'grill ticket 06', '/tmp/agent-sweeper', 1735689600000, 1735776000000);`)
	writeNested(t, root, filepath.Join("storage", "message", "sess-1", "1.txt"), []byte("hi"))
	writeNested(t, root, filepath.Join("storage", "part", "sess-1", "2.txt"), []byte("bye"))

	sessions, err := scanOpenCode(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "sess-1" || s.Title != "grill ticket 06" || s.CWD != "/tmp/agent-sweeper" {
		t.Fatalf("bad session: %+v", s)
	}
	if s.SizeBytes <= 0 {
		t.Fatalf("size should reflect artifact trees, got %d", s.SizeBytes)
	}
	if s.LastActivity != time.UnixMilli(1735776000000) {
		t.Fatalf("last activity should be time_updated, got %v", s.LastActivity)
	}

	plan := planBuilderFor("OpenCode", root, &s)
	if !hasAction(plan, engine.RemoveTree) {
		t.Fatal("plan should remove the storage trees")
	}
	if !hasAction(plan, engine.SQLDelete) {
		t.Fatal("plan should delete the session row")
	}
	if plan.Reclaim() != s.SizeBytes-s.StoreBytes {
		t.Fatalf("reclaim %d should equal detection size %d minus store %d", plan.Reclaim(), s.SizeBytes, s.StoreBytes)
	}
}

// TestScanOpenCodeFoldsChildren checks that a session tree (parent with
// compacted children) is folded into one root whose plan sweeps the whole
// subtree and deletes event/event_sequence rows.
func TestScanOpenCodeFoldsChildren(t *testing.T) {
	root := testutil.StoreRoot(t)
	testutil.SeedDB(t, root, "opencode.db", `
		CREATE TABLE session (id TEXT PRIMARY KEY, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER);
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE, data TEXT);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL REFERENCES message(id) ON DELETE CASCADE, session_id TEXT NOT NULL, data TEXT);
		CREATE TABLE event (id TEXT PRIMARY KEY, aggregate_id TEXT NOT NULL, seq INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL);
		CREATE TABLE event_sequence (aggregate_id TEXT PRIMARY KEY, seq INTEGER NOT NULL);
		INSERT INTO session VALUES ('root-1', NULL, 'main', '/tmp/x', 1000, 2000);
		INSERT INTO session VALUES ('child-1', 'root-1', 'explore', '/tmp/x', 3000, 4000);
		INSERT INTO session VALUES ('grand-1', 'child-1', 'general', '/tmp/x', 5000, 6000);
		INSERT INTO message VALUES ('m1','root-1','root-msg'),('m2','child-1','child-msg');
		INSERT INTO part VALUES ('p1','m1','root-1','root-part'),('p2','m2','child-1','child-part');
		INSERT INTO event VALUES ('e1','root-1',0,'x','root-event'),('e2','child-1',0,'x','child-event'),('e3','grand-1',0,'x','grand-event');
		INSERT INTO event_sequence VALUES ('root-1',1),('child-1',1),('grand-1',1);`)
	writeNested(t, root, filepath.Join("storage", "message", "root-1", "a.txt"), []byte("root-artifact"))
	writeNested(t, root, filepath.Join("storage", "message", "child-1", "a.txt"), []byte("child-artifact"))
	writeNested(t, root, filepath.Join("storage", "part", "grand-1", "a.txt"), []byte("grand-artifact"))

	sessions, err := scanOpenCode(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 folded root, got %d", len(sessions))
	}
	sr := sessions[0]
	if sr.ID != "root-1" {
		t.Fatalf("folded root should be root-1, got %s", sr.ID)
	}
	if len(sr.Children) != 2 {
		t.Fatalf("children should include child-1 and grand-1, got %v", sr.Children)
	}
	if sr.LastActivity != time.UnixMilli(6000) {
		t.Fatalf("last activity should be the newest child's, got %v", sr.LastActivity)
	}
	if sr.StoreBytes <= 0 {
		t.Fatalf("store bytes should include DB rows, got %d", sr.StoreBytes)
	}

	plan := planBuilderFor("OpenCode", root, &sr)
	var eventDelete, seqDelete bool
	var sqlDeletes int
	for _, a := range plan.Actions {
		if a.Kind == engine.SQLDelete {
			sqlDeletes++
			if strings.Contains(a.SQL, "event") && !strings.Contains(a.SQL, "event_sequence") {
				eventDelete = true
			}
			if strings.Contains(a.SQL, "event_sequence") {
				seqDelete = true
			}
		}
	}
	if !eventDelete {
		t.Fatal("plan should delete event rows (no FK path from session)")
	}
	if !seqDelete {
		t.Fatal("plan should delete event_sequence rows")
	}
	if sqlDeletes < 3 {
		t.Fatalf("want session+event+event_sequence deletes, got %d SQL actions", sqlDeletes)
	}
	// The SQLite delete actions must carry their byte weight as StoreBytes, not
	// Bytes (which is for files); otherwise sweep stats under-report the DB.
	var carriedStoreBytes int64
	for _, a := range plan.Actions {
		if a.Kind == engine.SQLDelete && a.Store != "" {
			carriedStoreBytes += a.StoreBytes
			if a.Bytes != 0 {
				t.Fatalf("SQL delete action must not set Bytes (fs field), got %d", a.Bytes)
			}
		}
	}
	if carriedStoreBytes <= 0 {
		t.Fatal("SQL delete actions must carry nonzero StoreBytes")
	}
	// RemoveTree actions must cover both root and child storage trees.
	var removeTrees int
	for _, a := range plan.Actions {
		if a.Kind == engine.RemoveTree {
			removeTrees++
		}
	}
	if removeTrees != 3 {
		t.Fatalf("want 3 remove-tree actions (root + 2 children), got %d", removeTrees)
	}
}

// TestOpenCodePlanDeletesEventRows executes the plan against the seeded store
// and asserts the event log rows are gone.
func TestOpenCodePlanDeletesEventRows(t *testing.T) {
	root := testutil.StoreRoot(t)
	dbPath := testutil.SeedDB(t, root, "opencode.db", `
		CREATE TABLE session (id TEXT PRIMARY KEY, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER);
		CREATE TABLE event (id TEXT PRIMARY KEY, aggregate_id TEXT NOT NULL, seq INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL);
		CREATE TABLE event_sequence (aggregate_id TEXT PRIMARY KEY, seq INTEGER NOT NULL);
		INSERT INTO session VALUES ('s-1', NULL, 'x', '/tmp/x', 1000, 2000);
		INSERT INTO event VALUES ('e1','s-1',0,'a','data'),('e2','s-1',1,'b','data');
		INSERT INTO event_sequence VALUES ('s-1',2);`)
	s := model.Session{ID: "s-1", Title: "x"}
	plan := opencodePlan(root, &s)
	res := engine.Execute(context.Background(), &engine.Plan{Sessions: []*engine.SessionPlan{plan}})
	if res.Deleted() != 1 {
		t.Fatalf("deleted = %d, want 1: %v", res.Deleted(), res.Sessions[0].Err)
	}
	if n := testutil.CountRows(t, dbPath, "SELECT COUNT(*) FROM session"); n != 0 {
		t.Fatalf("session rows = %d, want 0", n)
	}
	if n := testutil.CountRows(t, dbPath, "SELECT COUNT(*) FROM event"); n != 0 {
		t.Fatalf("event rows = %d, want 0 (v2 event log must be deleted)", n)
	}
	if n := testutil.CountRows(t, dbPath, "SELECT COUNT(*) FROM event_sequence"); n != 0 {
		t.Fatalf("event_sequence rows = %d, want 0", n)
	}
}

func TestScanOpenCodeMissingStore(t *testing.T) {
	root := testutil.StoreRoot(t)
	sessions, err := scanOpenCode(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("missing store should yield no sessions, got %d", len(sessions))
	}
}

// TestDiscoveredDropsAbsentAndEmptyAgents checks that Discovered keeps only
// agents that were found with at least one session, so lists never clutter on
// agents that do not exist on the machine.
func TestDiscoveredDropsAbsentAndEmptyAgents(t *testing.T) {
	inv := Inventory{Agents: []model.Agent{
		{Name: "Found", Found: true, Sessions: []model.Session{{ID: "a"}}},
		{Name: "EmptyStore", Found: true, Sessions: nil}, // store exists but nothing reclaimable
		{Name: "Missing", Found: false},                  // store not present
		{Name: "EmptyMissing", Found: false, Sessions: []model.Session{{ID: "b"}}}, // absent despite a session marker
	}}
	got := inv.Discovered()
	if len(got) != 1 || got[0].Name != "Found" {
		t.Fatalf("Discovered() = %d agents (%v), want just Found", len(got), agentNames(got))
	}
}

func agentNames(agents []model.Agent) []string {
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name)
	}
	return names
}

func TestScanCopilot(t *testing.T) {
	root := testutil.StoreRoot(t)
	testutil.SeedDB(t, root, "session-store.db", `
		CREATE TABLE sessions (id TEXT PRIMARY KEY, cwd TEXT, repository TEXT, branch TEXT, summary TEXT, created_at TEXT, updated_at TEXT);
		INSERT INTO sessions VALUES ('cp-1', '/tmp/a', 'github.com/x/y', 'main', 'migrate', '2025-01-01T00:00:00Z', '2025-01-02T00:00:00Z');`)
	writeNested(t, root, filepath.Join("session-state", "cp-1", "state.json"), []byte("{}"))

	sessions, err := scanCopilot(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "cp-1" || s.Title != "migrate" || s.Repo != "github.com/x/y" || s.Branch != "main" {
		t.Fatalf("bad session: %+v", s)
	}

	plan := planBuilderFor("Copilot", root, &s)
	if !hasAction(plan, engine.RemoveTree) {
		t.Fatal("plan should remove the session-state dir")
	}
	if !hasAction(plan, engine.SQLDelete) {
		t.Fatal("plan should delete store rows")
	}
}

func TestScanClaude(t *testing.T) {
	root := testutil.StoreRoot(t)
	slug := claudeSlug("/tmp/agent-sweeper")
	dir := filepath.Join(root, "projects", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	header := `{"type":"summary","cwd":"/tmp/agent-sweeper","gitBranch":"main","summary":"todos tidy"}` + "\n"
	writeNested(t, root, filepath.Join("projects", slug, "cc-1.jsonl"), []byte(header))
	writeNested(t, root, filepath.Join("projects", slug, "cc-1", "sub.jsonl"), []byte(`{"type":"subagent"}`+"\n"))

	sessions, err := scanClaude(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("subagent transcripts are not sessions: want 1, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "cc-1" || s.CWD != "/tmp/agent-sweeper" || s.Branch != "main" || s.Title != "todos tidy" {
		t.Fatalf("bad session: %+v", s)
	}
	if s.SizeBytes <= 0 {
		t.Fatalf("size should include the subagent tree, got %d", s.SizeBytes)
	}

	plan := planBuilderFor("Claude Code", root, &s)
	if !hasAction(plan, engine.RemoveFile) {
		t.Fatal("plan should remove the transcript")
	}
	if !hasAction(plan, engine.RemoveTree) {
		t.Fatal("plan should remove the subagent dir")
	}
}

func TestScanCodex(t *testing.T) {
	root := testutil.StoreRoot(t)
	id := "019f14ea-d583-77b2-9e7e-8272326bb07a"
	rel := filepath.Join("sessions", "2025", "06", "30", "rollout-2025-06-30T10-00-00-"+id+".jsonl")
	writeNested(t, root, rel, []byte(`{"id":"sx-9","title":"goreleaser config"}`+"\n"))
	file := filepath.Join(root, filepath.FromSlash(rel))
	testutil.SeedDB(t, root, "state_0.sqlite", `
		CREATE TABLE threads (id TEXT PRIMARY KEY, cwd TEXT, title TEXT);
		INSERT INTO threads VALUES ('`+id+`', '/tmp/codex', 'goreleaser config');`)

	sessions, err := scanCodex(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != id || s.CWD != "/tmp/codex" || s.Title != "goreleaser config" {
		t.Fatalf("bad session: %+v", s)
	}

	plan := planBuilderFor("Codex", root, &s)
	if !hasAction(plan, engine.RemoveFile) {
		t.Fatal("plan should remove the rollout file")
	}
	if !hasAction(plan, engine.SQLDelete) {
		t.Fatal("plan should delete the threads row")
	}
	if plan.Reclaim() != statBytes(file) {
		t.Fatalf("reclaim %d should equal rollout size %d", plan.Reclaim(), statBytes(file))
	}
}

func TestScanPi(t *testing.T) {
	root := testutil.StoreRoot(t)
	writeNested(t, root,
		filepath.Join("sessions", "tmp-omnivue", "2025-01-01T00-00-00_pi-1.jsonl"),
		[]byte(`{"cwd":"/tmp/omnivue","title":"jsonl header parse"}`+"\n"))
	file := filepath.Join(root, "sessions", "tmp-omnivue", "2025-01-01T00-00-00_pi-1.jsonl")

	sessions, err := scanPi(root, fixNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "pi-1" || s.CWD != "/tmp/omnivue" || s.Title != "jsonl header parse" {
		t.Fatalf("bad session: %+v", s)
	}

	plan := planBuilderFor("Pi", root, &s)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != engine.RemoveFile {
		t.Fatalf("pi plan should be exactly one remove-file action, got %+v", plan.Actions)
	}
	if plan.Reclaim() != statBytes(file) {
		t.Fatalf("reclaim %d should equal file size %d", plan.Reclaim(), statBytes(file))
	}
}

func TestCursorTranscriptSession(t *testing.T) {
	root := testutil.StoreRoot(t)
	dir := filepath.Join(root, "projects", "tmp-x", "agent-transcripts", "cur-7")
	writeNested(t, root,
		filepath.Join("projects", "tmp-x", "agent-transcripts", "cur-7", "cur-7.jsonl"),
		[]byte(`{"cwd":"/tmp/x","type":"message"}`+"\n"))

	s := cursorTranscriptSession(dir)
	if s.ID != "cur-7" || s.CWD != "/tmp/x" {
		t.Fatalf("bad transcript session: %+v", s)
	}
	if s.SizeBytes <= 0 {
		t.Fatalf("size should reflect the transcript dir, got %d", s.SizeBytes)
	}

	plan := planBuilderFor("Cursor", root, &s)
	if !hasAction(plan, engine.RemoveTree) {
		t.Fatal("cursor plan should remove the transcript dir")
	}
}
