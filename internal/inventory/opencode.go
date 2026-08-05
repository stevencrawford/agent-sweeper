package inventory

import (
	"path/filepath"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// scanOpenCode enumerates sessions from opencode.db plus their per-session
// storage/ artifact trees. Snapshot git repos are shared per project and are
// never enumerated or planned (decision 08).
func scanOpenCode(root string, _ time.Time) ([]model.Session, error) {
	dbPath := filepath.Join(root, "opencode.db")
	if !exists(dbPath) {
		return nil, nil
	}
	db, err := openReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, COALESCE(title, ''), COALESCE(directory, ''),
		       COALESCE(time_created, 0), COALESCE(time_updated, 0)
		FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var id, title, dir string
		var created, updated int64
		if err := rows.Scan(&id, &title, &dir, &created, &updated); err != nil {
			return nil, err
		}
		if id == "" {
			continue
		}
		last := max(created, updated)
		s := model.Session{
			ID:           id,
			Title:        title,
			CWD:          dir,
			Label:        dir,
			Repo:         deriveRepo(dir),
			LastActivity: time.UnixMilli(last),
			SizeBytes:    opencodeSessionSize(root, id),
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// opencodeSessionSize is the raw detection-time size of the session's
// per-session artifact trees (DB row bytes are shared and not measured).
func opencodeSessionSize(root, id string) int64 {
	var total int64
	for _, sub := range []string{"session_diff", "message", "part"} {
		total += dirBytes(filepath.Join(root, "storage", sub, id))
	}
	legacy := filepath.Join(root, "storage", "session", id+"*")
	for _, p := range glob(legacy) {
		total += dirBytes(p)
	}
	return total
}

// opencodePlan deletes the session's storage artifacts then its DB row. The
// FK cascade (foreign_keys=ON on the engine's connection) removes message,
// part, todo, and the v2 event rows.
func opencodePlan(root string, s *model.Session) *engine.SessionPlan {
	var actions []engine.Action
	for _, sub := range []string{"session_diff", "message", "part"} {
		actions = append(actions, removeActions(engine.RemoveTree,
			[]string{filepath.Join(root, "storage", sub, s.ID)},
			exists, dirBytes)...)
	}
	legacy := filepath.Join(root, "storage", "session", s.ID+"*")
	if matches := glob(legacy); len(matches) > 0 {
		var b int64
		for _, m := range matches {
			b += dirBytes(m)
		}
		actions = append(actions, engine.Action{Kind: engine.RemoveGlob, Path: legacy, Bytes: b})
	}
	db := filepath.Join(root, "opencode.db")
	if exists(db) {
		actions = append(actions, engine.Action{
			Kind: engine.SQLDelete, Store: db,
			SQL: "DELETE FROM session WHERE id = ?", Args: []any{s.ID},
		})
	}
	return &engine.SessionPlan{ID: s.ID, Agent: "OpenCode", Title: s.Title, Actions: actions}
}
