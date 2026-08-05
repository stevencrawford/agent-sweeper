package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/paths"
)

// scanCursor enumerates sessions from the global state.vscdb composer keys,
// supplemented by the per-project agent transcripts. The content-addressed
// blob tables are never read as sessions (they are shared, decision 08).
func scanCursor(root string, _ time.Time) ([]model.Session, error) {
	sessions := map[string]*model.Session{}
	var order []string

	add := func(s model.Session) {
		if s.ID == "" {
			return
		}
		if existing, ok := sessions[s.ID]; ok {
			mergeCursorSession(existing, s)
			return
		}
		cp := s
		sessions[s.ID] = &cp
		order = append(order, s.ID)
	}

	for id, meta := range cursorComposers(paths.CursorGlobalState()) {
		s := model.Session{ID: id, Title: meta.title, LastActivity: meta.last}
		add(s)
	}
	for _, file := range cursorTranscripts(root) {
		add(cursorTranscriptSession(file))
	}

	out := make([]model.Session, 0, len(order))
	for _, id := range order {
		out = append(out, *sessions[id])
	}
	return out, nil
}

// mergeCursorSession unions the richer metadata of two views of one composer
// (the DB row and the transcript file).
func mergeCursorSession(dst *model.Session, src model.Session) {
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.CWD == "" {
		dst.CWD = src.CWD
		dst.Label = src.Label
		dst.Repo = src.Repo
	}
	if src.LastActivity.After(dst.LastActivity) {
		dst.LastActivity = src.LastActivity
	}
	if dst.SizeBytes == 0 {
		dst.SizeBytes = src.SizeBytes
	}
}

// composerMeta is the metadata read from a global-DB composerData row.
type composerMeta struct {
	title string
	last  time.Time
}

// cursorComposers reads the composer ids and their metadata from the global
// state.vscdb cursorDiskKV table.
func cursorComposers(global string) map[string]composerMeta {
	out := map[string]composerMeta{}
	if !exists(global) || !storeHasTable(global, "cursorDiskKV") {
		return out
	}
	db, err := openReadOnly(global)
	if err != nil {
		return out
	}
	defer db.Close()
	rows, err := db.Query("SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if rows.Scan(&key, &raw) != nil {
			continue
		}
		id := strings.TrimPrefix(key, "composerData:")
		var doc struct {
			Name          string `json:"name"`
			LastUpdatedAt string `json:"lastUpdatedAt"`
		}
		if json.Unmarshal(raw, &doc) != nil {
			continue
		}
		m := composerMeta{title: doc.Name}
		if t, ok := parseTime(doc.LastUpdatedAt); ok {
			m.last = t
		}
		out[id] = m
	}
	return out
}

// cursorTranscripts lists the per-session transcript directories under the
// agent home: projects/<slug>/agent-transcripts/<id>/.
func cursorTranscripts(root string) []string {
	return glob(filepath.Join(root, "projects", "*", "agent-transcripts", "*"))
}

// cursorTranscriptSession builds a session from a transcript directory.
func cursorTranscriptSession(dir string) model.Session {
	id := filepath.Base(dir)
	// The transcript file shares the id and sits inside the dir.
	file := filepath.Join(dir, id+".jsonl")
	last := time.Time{}
	if fi, err := os.Stat(file); err == nil {
		last = fi.ModTime()
	}
	size := dirBytes(dir)
	cwd := cursorTranscriptCWD(file)
	return model.Session{
		ID:           id,
		CWD:          cwd,
		Label:        cwd,
		Repo:         deriveRepo(cwd),
		LastActivity: last,
		SizeBytes:    size,
	}
}

// cursorTranscriptCWD reads the cwd field from the transcript's first line.
func cursorTranscriptCWD(file string) string {
	if v, ok := firstLineJSON(file)["cwd"].(string); ok {
		return v
	}
	return ""
}

// cursorPlan deletes the transcript dir, the global-DB KV rows, strips the id
// from the workspace sidebar index, then the optional ai-tracking summary row.
// Content-addressed blob rows are never touched.
func cursorPlan(root string, s *model.Session) *engine.SessionPlan {
	var actions []engine.Action
	transcripts := glob(filepath.Join(root, "projects", "*", "agent-transcripts", s.ID))
	actions = append(actions, removeActions(engine.RemoveTree, transcripts, exists, dirBytes)...)

	global := paths.CursorGlobalState()
	if exists(global) && storeHasTable(global, "cursorDiskKV") {
		for _, kv := range cursorKVDeletes(s.ID) {
			actions = append(actions, engine.Action{Kind: engine.SQLDelete, Store: global, SQL: kv.sql, Args: kv.args})
		}
	}
	for _, ws := range workspaceStores() {
		if storeContainsComposer(ws, s.ID) {
			actions = append(actions, engine.Action{
				Kind: engine.StripKV, Store: ws, Table: "ItemTable", Column: "value",
				Key: "composer.composerData", IDs: []string{s.ID},
			})
		}
	}
	if tracking := filepath.Join(root, "ai-tracking", "ai-code-tracking.db"); exists(tracking) && storeHasTable(tracking, "conversation_summaries") {
		actions = append(actions, engine.Action{
			Kind: engine.SQLDelete, Store: tracking,
			SQL: "DELETE FROM conversation_summaries WHERE conversationId = ?", Args: []any{s.ID},
		})
	}
	return &engine.SessionPlan{ID: s.ID, Agent: "Cursor", Title: s.Title, Actions: actions}
}

// kvDelete pairs one cursorDiskKV delete statement with its args.
type kvDelete struct {
	sql  string
	args []any
}

// cursorKVDeletes lists the global-DB rows a composer owns. Only the
// per-composer key families are deleted; composer.content.* stays.
func cursorKVDeletes(id string) []kvDelete {
	exact := []string{"composerData:" + id, "agentKv:checkpoint:" + id}
	prefix := []string{"bubbleId", "checkpointId", "messageRequestContext", "codeBlockDiff"}
	del := make([]kvDelete, 0, len(exact)+len(prefix))
	for _, key := range exact {
		del = append(del, kvDelete{"DELETE FROM cursorDiskKV WHERE key = ?", []any{key}})
	}
	for _, p := range prefix {
		del = append(del, kvDelete{"DELETE FROM cursorDiskKV WHERE key LIKE ?", []any{p + ":" + id + ":%"}})
	}
	return del
}

// workspaceStores lists the per-workspace state.vscdb files.
func workspaceStores() []string {
	return glob(filepath.Join(paths.CursorAppData(), "User", "workspaceStorage", "*", "state.vscdb"))
}
