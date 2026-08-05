// Package engine executes per-session deletion plans safely against real
// agent stores. A plan lists, for each session, the exact actions that delete
// it: filesystem artifacts first, the record (DB rows, index entries) last,
// so an interrupted sweep leaves the session visible and a re-run finishes
// it. The plan is built once at dry-run time and executed unchanged at
// confirm, which is what keeps the dry-run's reported reclaim equal to what
// the sweep actually deletes.
package engine

// ActionKind enumerates the operations a deletion plan can perform on a
// session's artifacts.
type ActionKind int

const (
	// RemoveFile deletes one file; a missing file is not an error.
	RemoveFile ActionKind = iota
	// RemoveTree recursively deletes a directory; a missing directory is not
	// an error.
	RemoveTree
	// RemoveGlob deletes every path matching a glob pattern; no matches are
	// not an error.
	RemoveGlob
	// SQLDelete runs one DELETE statement against an SQLite store, grouped
	// with the session's consecutive same-store SQL actions into a single
	// immediate transaction.
	SQLDelete
	// DropJSONLLines rewrites a JSONL index file without the lines whose id
	// field matches a session id; a missing file is not an error.
	DropJSONLLines
	// DropJSONKeys rewrites a JSON index file (object or array) without the
	// session ids; a missing file is not an error.
	DropJSONKeys
	// StripKV rewrites one JSON value held in an SQLite KV row (Cursor's
	// composer.composerData) without the session ids; a missing key is not an
	// error.
	StripKV
)

// Action is one step of a session plan. Which fields are used depends on the
// kind: Path for the remove and index kinds; Store, SQL, and Args for
// SQLDelete; Store, Table, Column, Key, and IDs for StripKV.
type Action struct {
	Kind   ActionKind
	Path   string
	Store  string
	Table  string
	Column string
	Key    string
	IDs    []string
	SQL    string
	Args   []any
	Bytes  int64 // filesystem bytes reclaimable by a remove action
}

// SessionPlan is the ordered set of actions that delete one session and all
// of its per-session artifacts. Actions run in listed order; the record comes
// last so a session is never left invisible while its artifacts remain.
type SessionPlan struct {
	ID      string
	Agent   string
	Title   string
	Actions []Action
}

// Reclaim returns the filesystem bytes this plan removes: the sum of the
// remove-action bytes. SQL and index actions contribute zero because row
// deletion does not shrink an SQLite file until VACUUM.
func (p *SessionPlan) Reclaim() int64 {
	var total int64
	for _, a := range p.Actions {
		if a.Kind == RemoveFile || a.Kind == RemoveTree || a.Kind == RemoveGlob {
			total += a.Bytes
		}
	}
	return total
}

// HasStoreActions reports whether this plan deletes SQLite rows or KV
// entries, the actions whose bytes Reclaim does not cover.
func (p *SessionPlan) HasStoreActions() bool {
	for _, a := range p.Actions {
		if a.Kind == SQLDelete || a.Kind == StripKV {
			return true
		}
	}
	return false
}

// Plan is the fixed unit the dry-run renders and the confirmed sweep
// executes. It is built once, before any interaction, so the before-footprint
// it reports is exactly what the engine will reclaim.
type Plan struct {
	Sessions []*SessionPlan
}

// Reclaim returns the total reclaimable filesystem bytes across all sessions.
func (p *Plan) Reclaim() int64 {
	var total int64
	for _, s := range p.Sessions {
		total += s.Reclaim()
	}
	return total
}

// SessionCount returns how many sessions the plan deletes.
func (p *Plan) SessionCount() int {
	return len(p.Sessions)
}

// HasStoreActions reports whether any session deletes SQLite rows or KV
// entries, the actions whose bytes the plan's reclaim does not cover.
func (p *Plan) HasStoreActions() bool {
	for _, s := range p.Sessions {
		for _, a := range s.Actions {
			if a.Kind == SQLDelete || a.Kind == StripKV {
				return true
			}
		}
	}
	return false
}
