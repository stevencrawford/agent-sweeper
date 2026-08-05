package inventory

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// scanCopilot enumerates sessions from session-store.db and pairs each with
// its session-state/<id>/ artifact directory. The DB is a derived index over
// the session dirs, so a session may be present in one and not the other.
func scanCopilot(root string, _ time.Time) ([]model.Session, error) {
	store := filepath.Join(root, "session-store.db")
	if !exists(store) {
		return nil, nil
	}
	db, err := openReadOnly(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, COALESCE(cwd, ''), COALESCE(repository, ''), COALESCE(branch, ''),
		       COALESCE(summary, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var id, cwd, repo, branch, summary, created, updated string
		if err := rows.Scan(&id, &cwd, &repo, &branch, &summary, &created, &updated); err != nil {
			return nil, err
		}
		if id == "" {
			continue
		}
		last := time.Time{}
		if t, ok := parseTime(updated); ok {
			last = t
		} else if t, ok := parseTime(created); ok {
			last = t
		}
		s := model.Session{
			ID:           id,
			Title:        summary,
			CWD:          cwd,
			Label:        cwd,
			Repo:         repo,
			Branch:       branch,
			LastActivity: last,
			SizeBytes:    dirBytes(filepath.Join(root, "session-state", id)),
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// copilotPlan deletes the session dir, then the store rows for the session.
// Each table delete is gated on the table existing so a newer/older store
// never fails a sweep.
func copilotPlan(root string, s *model.Session) *engine.SessionPlan {
	var actions []engine.Action
	actions = append(actions, removeActions(engine.RemoveTree,
		[]string{filepath.Join(root, "session-state", s.ID)}, exists, dirBytes)...)

	store := filepath.Join(root, "session-store.db")
	if exists(store) {
		for _, sql := range []string{
			"DELETE FROM search_index WHERE session_id = ?",
			"DELETE FROM turns WHERE session_id = ?",
			"DELETE FROM checkpoints WHERE session_id = ?",
			"DELETE FROM session_files WHERE session_id = ?",
			"DELETE FROM session_refs WHERE session_id = ?",
			"DELETE FROM assistant_usage_events WHERE session_id = ?",
			"DELETE FROM sessions WHERE id = ?",
		} {
			table := sqlTable(sql)
			if table != "" && !storeHasTable(store, table) {
				continue
			}
			actions = append(actions, engine.Action{
				Kind: engine.SQLDelete, Store: store, SQL: sql, Args: []any{s.ID},
			})
		}
	}
	if data := filepath.Join(root, "data.db"); exists(data) && storeHasTable(data, "sessions") {
		actions = append(actions, engine.Action{
			Kind: engine.SQLDelete, Store: data,
			SQL: "DELETE FROM sessions WHERE id = ?", Args: []any{s.ID},
		})
	}
	return &engine.SessionPlan{ID: s.ID, Agent: "Copilot", Title: s.Title, Actions: actions}
}

// sqlTable extracts the first table named in a "DELETE FROM <table> ..."
// statement, to gate table deletes against the real schema.
func sqlTable(sql string) string {
	_, after, ok := strings.Cut(sql, "FROM ")
	if !ok {
		return ""
	}
	for f := range strings.FieldsSeq(after) {
		return f
	}
	return ""
}
