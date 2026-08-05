package engine

import "github.com/stevencrawford/agent-sweeper/internal/model"

// SessionReclaim is the single footprint computation shared by the `stats`
// command and the `sweep` dry-run, so the two can never disagree. It returns
// the filesystem bytes a sweep of s reclaims: the sum of the session deletion
// plan's Remove* action bytes. Row and KV actions contribute zero because
// SQLite file size is unchanged until VACUUM. Real detection (feeds 10/12)
// will build the per-agent plan and sum its remove actions; until then it
// reports the session's carried reclaim footprint.
func SessionReclaim(s model.Session) int64 {
	return s.ReclaimBytes
}

// SessionTouchesStore reports whether deleting s removes SQLite rows or KV
// entries, the actions whose bytes Reclaim does not cover. Stats surfaces
// this as the store-row count so agents with inflated DB files are visible.
func SessionTouchesStore(s model.Session) bool {
	return s.TouchesStore
}
