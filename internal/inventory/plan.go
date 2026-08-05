package inventory

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
)

// storeTables caches the table set of every SQLite store we've inspected so
// plan building (which runs once per session) does not re-open a store it has
// already seen.
var (
	storeTablesCache = map[string]map[string]bool{}
	storeTablesMu    sync.Mutex
)

// openReadOnly opens a SQLite store for reading. mode=ro lets the driver see
// the WAL sidecar, so recent un-checkpointed rows are not silently missed.
func openReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
}

// storeTables returns the set of table names present in a store, cached.
func storeTables(store string) map[string]bool {
	storeTablesMu.Lock()
	defer storeTablesMu.Unlock()
	if t, ok := storeTablesCache[store]; ok {
		return t
	}
	t := map[string]bool{}
	if db, err := openReadOnly(store); err == nil {
		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table'")
		if err == nil {
			for rows.Next() {
				var name string
				if rows.Scan(&name) == nil {
					t[name] = true
				}
			}
			rows.Close()
		}
		db.Close()
	}
	storeTablesCache[store] = t
	return t
}

// storeHasTable reports whether the store has the named table. Plan builders
// gate SQL actions on this so a DELETE against an absent table (e.g. Copilot's
// newer assistant_usage_events) can never fail a session.
func storeHasTable(store, table string) bool {
	return storeTables(store)[table]
}

// storeContainsComposer reports whether a Cursor workspace store's
// composer.composerData value references the composer id, which is what marks
// the workspace DB as needing a StripKV action.
func storeContainsComposer(store, id string) bool {
	if !storeHasTable(store, "ItemTable") {
		return false
	}
	db, err := openReadOnly(store)
	if err != nil {
		return false
	}
	defer db.Close()
	var raw []byte
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "composer.composerData").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	return composerIDReferenced(raw, id)
}

// composerIDReferenced reports whether the composer id appears in any of the
// composerData index fields (allComposers / selectedComposerIds /
// lastFocusedComposerIds), matching how the engine strips the id.
func composerIDReferenced(raw []byte, id string) bool {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return false
	}
	for _, field := range []string{"allComposers", "selectedComposerIds", "lastFocusedComposerIds"} {
		list, ok := doc[field].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			var got string
			switch v := item.(type) {
			case string:
				got = v
			case map[string]any:
				got, _ = v["id"].(string)
			}
			if got == id {
				return true
			}
		}
	}
	return false
}

// removeActions returns the ordered filesystem-remove actions for a set of
// existing paths (files or trees), summing their bytes for the plan's reclaim.
func removeActions(kind engine.ActionKind, paths []string, exists func(string) bool, size func(string) int64) []engine.Action {
	var out []engine.Action
	for _, p := range paths {
		if !exists(p) {
			continue
		}
		out = append(out, engine.Action{Kind: kind, Path: p, Bytes: size(p)})
	}
	return out
}
