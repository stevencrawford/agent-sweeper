package inventory

import (
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// opencodeRow is one session row from opencode.db, with the parent linkage
// needed to assemble the parent/child (compaction) tree.
type opencodeRow struct {
	id    string
	pID   string // empty when the session has no parent
	title string
	dir   string
	last  int64
}

// scanOpenCode enumerates sessions from opencode.db plus their per-session
// storage/ artifact trees. Sessions are folded into their subtree root: a
// session whose parent_id resolves to another session in the store is a child
// and is never listed alone; it is absorbed into the nearest ancestor whose
// parent is absent (the root), and sweeping the root deletes the whole
// subtree. Snapshot git repos are shared per project and are never enumerated
// or planned (decision 08).
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
		SELECT id, COALESCE(parent_id, ''), COALESCE(title, ''),
		       COALESCE(directory, ''), COALESCE(time_created, 0),
		       COALESCE(time_updated, 0)
		FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []opencodeRow
	for rows.Next() {
		var r opencodeRow
		if err := rows.Scan(&r.id, &r.pID, &r.title, &r.dir, new(int64), &r.last); err != nil {
			return nil, err
		}
		if r.id == "" {
			continue
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	inStore := make(map[string]bool, len(all))
	for _, r := range all {
		inStore[r.id] = true
	}
	// childrenOf maps a parent id to its direct children (parent_id present in
	// this store).
	childrenOf := make(map[string][]string)
	for _, r := range all {
		if r.pID != "" && inStore[r.pID] {
			childrenOf[r.pID] = append(childrenOf[r.pID], r.id)
		}
	}

	// Roots are the sessions whose parent is absent or not in this store;
	// every other session is swept only as part of its subtree.
	var roots []opencodeRow
	for _, r := range all {
		if r.pID == "" || !inStore[r.pID] {
			roots = append(roots, r)
		}
	}

	sessions := make([]model.Session, 0, len(roots))
	for _, r := range roots {
		subtree := collectSubtree(r.id, childrenOf)
		sessions = append(sessions, opencodeSession(root, r, subtree, db, dbPath))
	}
	return sessions, nil
}

// collectSubtree returns id itself followed by every transitive descendant,
// breadth-first: the session ids a subtree sweep deletes.
func collectSubtree(id string, childrenOf map[string][]string) []string {
	ids := []string{id}
	queue := []string{id}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range childrenOf[cur] {
			ids = append(ids, c)
			queue = append(queue, c)
		}
	}
	return ids
}

// opencodeSession builds the model.Session for one subtree root, measuring the
// storage trees and SQLite row bytes of every member and taking the newest
// activity across the subtree so a live child protects the whole tree.
func opencodeSession(root string, r opencodeRow, subtree []string, db *sql.DB, _ string) model.Session {
	var fsBytes, storeBytes int64
	last := r.last
	for _, id := range subtree {
		fsBytes += opencodeSessionSize(root, id)
		if t := sessionTimestamp(db, id); t > last {
			last = t
		}
	}
	storeBytes = opencodeStoreBytes(db, subtree)
	return model.Session{
		ID:           r.id,
		Title:        r.title,
		CWD:          r.dir,
		Label:        r.dir,
		Repo:         deriveRepo(r.dir),
		LastActivity: time.UnixMilli(last),
		SizeBytes:    fsBytes + storeBytes,
		StoreBytes:   storeBytes,
		Children:     subtree[1:],
	}
}

// sessionTimestamp returns the max of time_created and time_updated for a
// session row, or 0 when the row is gone.
func sessionTimestamp(db *sql.DB, id string) int64 {
	var created, updated int64
	if err := db.QueryRow(
		"SELECT COALESCE(time_created,0), COALESCE(time_updated,0) FROM session WHERE id = ?",
		id).Scan(&created, &updated); err != nil {
		return 0
	}
	if created > updated {
		return created
	}
	return updated
}

// opencodeStoreBytes estimates the SQLite row bytes a subtree sweep frees
// after VACUUM: the LENGTH of every per-session data column across the subtree
// plus the event log (keyed by aggregate_id). A table that does not exist
// contributes zero.
func opencodeStoreBytes(db *sql.DB, subtree []string) int64 {
	ph := inPlaceholders(len(subtree))
	args := make([]any, len(subtree))
	for i, id := range subtree {
		args[i] = id
	}
	var total int64
	for _, t := range []string{"part", "message", "session_message", "session_input", "todo"} {
		if !storeHasDBTable(db, t) {
			continue
		}
		total += sumColumn(db, "SELECT COALESCE(SUM(LENGTH(data)),0) FROM "+t+
			" WHERE session_id IN ("+ph+")", args)
	}
	if storeHasDBTable(db, "event") {
		total += sumColumn(db, "SELECT COALESCE(SUM(LENGTH(data)),0) FROM event"+
			" WHERE aggregate_id IN ("+ph+")", args)
	}
	return total
}

// storeHasDBTable reports whether db has the named table.
func storeHasDBTable(db *sql.DB, table string) bool {
	var one string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&one)
	return err == nil
}

// sumColumn runs a scalar aggregation query returning the first int64 column.
func sumColumn(db *sql.DB, query string, args []any) int64 {
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		return 0
	}
	return n
}

// inPlaceholders returns "?,?,?" with n placeholders.
func inPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// opencodeSessionSize is the raw detection-time size of the session's
// per-session artifact trees (DB row bytes are measured separately).
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

// opencodePlan deletes a session's whole subtree: the storage artifacts for
// each member, then the DB rows. The explicit event/event_sequence deletes
// cover the v2 event log, which has no FK path from session; the session
// DELETE cascades message/part/todo and the other per-session rows via
// foreign_keys=ON on the engine's connection.
func opencodePlan(root string, s *model.Session) *engine.SessionPlan {
	subtree := append([]string{s.ID}, s.Children...)

	var actions []engine.Action
	for _, id := range subtree {
		for _, sub := range []string{"session_diff", "message", "part"} {
			actions = append(actions, removeActions(engine.RemoveTree,
				[]string{filepath.Join(root, "storage", sub, id)},
				exists, dirBytes)...)
		}
		legacy := filepath.Join(root, "storage", "session", id+"*")
		if matches := glob(legacy); len(matches) > 0 {
			var b int64
			for _, m := range matches {
				b += dirBytes(m)
			}
			actions = append(actions, engine.Action{Kind: engine.RemoveGlob, Path: legacy, Bytes: b})
		}
	}

	db := filepath.Join(root, "opencode.db")
	if exists(db) {
		if storeHasTable(db, "event") {
			actions = append(actions, sqlDeleteIn(db, "event", "aggregate_id", subtree))
		}
		if storeHasTable(db, "event_sequence") {
			actions = append(actions, sqlDeleteIn(db, "event_sequence", "aggregate_id", subtree))
		}
		actions = append(actions, engine.Action{
			Kind: engine.SQLDelete, Store: db, StoreBytes: s.StoreBytes,
			SQL: "DELETE FROM session WHERE id IN (" + inPlaceholders(len(subtree)) + ")",
			Args: anyList(subtree),
		})
	}
	return &engine.SessionPlan{ID: s.ID, Agent: "OpenCode", Title: s.Title, Actions: actions}
}

// sqlDeleteIn builds a SQLDelete action removing every row of table whose key
// column is in ids. Carries no store bytes; the session delete attributes them
// once.
func sqlDeleteIn(db, table, key string, ids []string) engine.Action {
	return engine.Action{
		Kind:  engine.SQLDelete,
		Store: db,
		SQL:   "DELETE FROM " + table + " WHERE " + key + " IN (" + inPlaceholders(len(ids)) + ")",
		Args:  anyList(ids),
	}
}

func anyList(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
