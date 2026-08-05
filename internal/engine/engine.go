package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"modernc.org/sqlite"
	lib "modernc.org/sqlite/lib"
)

// busyTimeout is the SQLite busy timeout (milliseconds are sent as a pragma)
// and the per-attempt ceiling the retry loop sleeps toward.
var busyTimeout = 5 * time.Second

// maxBusyRetries is how many times a lock-contended store transaction is
// retried before the session is reported as failed.
const maxBusyRetries = 3

// Result reports what one sweep execution did. Sessions whose Err is nil were
// fully deleted; the rest failed part-way and are safe to re-run.
type Result struct {
	Sessions []*SessionResult
}

// SessionResult is the outcome of one session's plan.
type SessionResult struct {
	Session *SessionPlan
	Err     error // nil when the session was fully deleted
}

// Deleted returns how many sessions were fully deleted.
func (r *Result) Deleted() int {
	var n int
	for _, s := range r.Sessions {
		if s.Err == nil {
			n++
		}
	}
	return n
}

// Failed returns how many sessions errored part-way and are left for a re-run.
func (r *Result) Failed() int {
	var n int
	for _, s := range r.Sessions {
		if s.Err != nil {
			n++
		}
	}
	return n
}

// BytesReclaimed returns the filesystem bytes removed by fully deleted
// sessions. It equals Plan.Reclaim when every session succeeded, which is the
// dry-run invariant.
func (r *Result) BytesReclaimed() int64 {
	var total int64
	for _, s := range r.Sessions {
		if s.Err == nil {
			total += s.Session.Reclaim()
		}
	}
	return total
}

// NeedsVacuum reports whether the sweep deleted rows from an SQLite store,
// whose file size is unchanged until a VACUUM runs.
func (r *Result) NeedsVacuum() bool {
	for _, s := range r.Sessions {
		if s.Err != nil {
			continue
		}
		if s.Session.HasStoreActions() {
			return true
		}
	}
	return false
}

// Execute runs every session plan in order. A failed session does not stop
// the sweep: it is reported on SessionResult.Err and execution continues with
// the next session. Within a session the actions run in listed order and stop
// at the first failure, so a session's record is never removed while its
// artifacts remain.
func Execute(ctx context.Context, plan *Plan) *Result {
	res := &Result{}
	for _, sp := range plan.Sessions {
		sr := &SessionResult{Session: sp}
		if err := executeSession(ctx, sp); err != nil {
			sr.Err = fmt.Errorf("session %s (%s): %w", sp.ID, sp.Agent, err)
		}
		res.Sessions = append(res.Sessions, sr)
	}
	return res
}

// executeSession runs one session's actions in listed order. Consecutive
// same-store SQLite actions (SQLDelete and StripKV) share one immediate
// transaction so a session's rows are committed or rolled back together.
func executeSession(ctx context.Context, sp *SessionPlan) error {
	for i := 0; i < len(sp.Actions); i++ {
		a := &sp.Actions[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.Kind != SQLDelete && a.Kind != StripKV {
			if err := execAction(ctx, a); err != nil {
				return fmt.Errorf("%s: %w", describe(a), err)
			}
			continue
		}
		group := sp.Actions[i:]
		end := 1
		for end < len(group) && group[end].Store == a.Store &&
			(group[end].Kind == SQLDelete || group[end].Kind == StripKV) {
			end++
		}
		if err := executeStoreGroup(ctx, sp.Actions[i:i+end]); err != nil {
			return err
		}
		i += end - 1
	}
	return nil
}

// executeStoreGroup deletes rows from one SQLite store in a single immediate
// transaction, retrying on lock contention up to maxBusyRetries times.
func executeStoreGroup(ctx context.Context, group []Action) error {
	var lastErr error
	for attempt := 0; attempt <= maxBusyRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
		lastErr = runStoreGroup(ctx, group)
		if lastErr == nil || !isBusy(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

// runStoreGroup executes one group of actions against its store inside a
// single BEGIN IMMEDIATE transaction.
func runStoreGroup(ctx context.Context, group []Action) error {
	store := group[0].Store
	if _, err := os.Stat(store); errors.Is(err, os.ErrNotExist) {
		return nil // store gone; nothing left to delete
	}
	db, err := openStore(store)
	if err != nil {
		return fmt.Errorf("open %s: %w", store, err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("%s: begin: %w", store, err)
	}
	for i := range group {
		if err := execStoreAction(ctx, db, &group[i]); err != nil {
			return fmt.Errorf("%s: %w", store, rollbackOr(db, err))
		}
	}
	if _, err := db.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("%s: commit: %w", store, rollbackOr(db, err))
	}
	return nil
}

// rollbackOr attempts a rollback of db and returns err, joined with any
// rollback failure so a clean-up error is never silently dropped.
func rollbackOr(db *sql.DB, err error) error {
	if _, rbErr := db.Exec("ROLLBACK"); rbErr != nil {
		return errors.Join(err, rbErr)
	}
	return err
}

// execStoreAction runs one SQLite-backed action inside the open transaction.
func execStoreAction(ctx context.Context, db *sql.DB, a *Action) error {
	switch a.Kind {
	case SQLDelete:
		_, err := db.ExecContext(ctx, a.SQL, a.Args...)
		return err
	case StripKV:
		return stripKV(ctx, db, a)
	}
	return nil
}

// stripKV removes the session ids from a JSON value stored in an SQLite KV
// row. Cursor's composer.composerData carries allComposers,
// selectedComposerIds, and lastFocusedComposerIds; dropping the id from all
// three is what removes the conversation from the sidebar.
func stripKV(ctx context.Context, db *sql.DB, a *Action) error {
	// Table and column come from the planner; validate them so the
	// interpolated identifiers can never smuggle in SQL.
	if !identOK(a.Table) || !identOK(a.Column) {
		return errors.New("kv strip: invalid table or column identifier")
	}
	var raw []byte
	// #nosec G202 -- table and column are validated fixed identifiers above.
	err := db.QueryRowContext(ctx,
		"SELECT "+a.Column+" FROM "+a.Table+" WHERE key = ?", a.Key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // nothing to strip
	}
	if err != nil {
		return err
	}
	stripped, err := removeComposerIDs(raw, a.IDs)
	if err != nil {
		return err
	}
	if string(stripped) == string(raw) {
		return nil
	}
	// #nosec G202 -- table and column are validated fixed identifiers above.
	_, err = db.ExecContext(ctx,
		"UPDATE "+a.Table+" SET "+a.Column+" = ? WHERE key = ?", stripped, a.Key)
	return err
}

// identOK reports whether s is a plain SQLite identifier ([A-Za-z0-9_]).
func identOK(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

// removeComposerIDs returns raw with every composer whose id is in ids removed
// from allComposers, selectedComposerIds, and lastFocusedComposerIds.
func removeComposerIDs(raw []byte, ids []string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	for _, field := range []string{"allComposers", "selectedComposerIds", "lastFocusedComposerIds"} {
		list, ok := doc[field].([]any)
		if !ok {
			continue
		}
		doc[field] = filterComposerIDs(list, ids)
	}
	return json.Marshal(doc)
}

// filterComposerIDs returns list without the items whose id is in ids. An
// item is matched by its id field when it is an object or by its value when
// it is a plain string.
func filterComposerIDs(list []any, ids []string) []any {
	out := list[:0]
	for _, item := range list {
		var id string
		switch v := item.(type) {
		case string:
			id = v
		case map[string]any:
			id, _ = v["id"].(string)
		}
		if slices.Contains(ids, id) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// execAction runs one filesystem or index action. All of them are idempotent:
// a missing file, directory, glob, or index is not an error.
func execAction(ctx context.Context, a *Action) error {
	switch a.Kind {
	case RemoveFile:
		return ignoreNotExist(os.Remove(a.Path))
	case RemoveTree:
		return ignoreNotExist(os.RemoveAll(a.Path))
	case RemoveGlob:
		matches, err := filepath.Glob(a.Path)
		if err != nil {
			return err
		}
		for _, m := range matches {
			if err := ignoreNotExist(os.RemoveAll(m)); err != nil {
				return err
			}
		}
		return nil
	case DropJSONLLines:
		return dropJSONLLines(a)
	case DropJSONKeys:
		return dropJSONKeys(a)
	}
	return nil
}

// dropJSONLLines rewrites a JSONL index without the lines whose id matches a
// session id. A malformed line fails the action rather than silently dropping
// index data.
func dropJSONLLines(a *Action) error {
	data, err := os.ReadFile(a.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var out []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return err
		}
		id, _ := rec["id"].(string)
		if slices.Contains(a.IDs, id) {
			continue
		}
		out = append(out, line)
	}
	// #nosec G703 -- the index path comes from the local session scan, not user input.
	return os.WriteFile(a.Path, []byte(strings.Join(out, "\n")+"\n"), 0o600)
}

// dropJSONKeys rewrites a JSON index without the session ids. Object-form
// indexes drop the id keys; array-form indexes drop the elements whose id
// field matches.
func dropJSONKeys(a *Action) error {
	data, err := os.ReadFile(a.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		for _, id := range a.IDs {
			delete(obj, id)
		}
		return writeJSON(a.Path, obj)
	}
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	out := arr[:0]
	for _, item := range arr {
		id, _ := item["id"].(string)
		if slices.Contains(a.IDs, id) {
			continue
		}
		out = append(out, item)
	}
	return writeJSON(a.Path, out)
}

// writeJSON writes v as indented JSON with a trailing newline.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	// #nosec G703 -- the index path comes from the local session scan, not user input.
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// openStore opens one SQLite store for writing with foreign keys enforced and
// a busy timeout, as a single connection so transaction control is exact.
func openStore(path string) (*sql.DB, error) {
	dsn := "file:" + filepath.ToSlash(path) +
		"?mode=rw&_pragma=foreign_keys(1)&_pragma=busy_timeout(" +
		fmt.Sprint(busyTimeout.Milliseconds()) + ")"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// isBusy reports whether err is an SQLite lock-contention error.
func isBusy(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code() == lib.SQLITE_BUSY || se.Code() == lib.SQLITE_LOCKED
	}
	return strings.Contains(err.Error(), "database is locked") ||
		strings.Contains(err.Error(), "database table is locked")
}

// ignoreNotExist returns nil when err is a missing-file error.
func ignoreNotExist(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// describe returns a human-readable label for an action, used in error
// wrapping so a failure names what it failed to do.
func describe(a *Action) string {
	switch a.Kind {
	case RemoveFile:
		return "remove file " + a.Path
	case RemoveTree:
		return "remove tree " + a.Path
	case RemoveGlob:
		return "remove glob " + a.Path
	case DropJSONLLines, DropJSONKeys:
		return "strip index " + a.Path
	}
	return a.Path
}
