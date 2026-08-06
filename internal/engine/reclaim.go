package engine

import "github.com/stevencrawford/agent-sweeper/internal/model"

// SessionReclaim is the single footprint computation shared by the `stats`
// command and the `sweep` dry-run, so the two can never disagree. It returns
// the filesystem bytes a sweep of s reclaims immediately: the sum of the
// session deletion plan's Remove* action bytes. Row and KV actions contribute
// zero because SQLite file size is unchanged until VACUUM.
func SessionReclaim(s model.Session) int64 {
	return s.ReclaimBytes
}

// SessionStoreBytes returns the estimated SQLite row bytes a sweep of s frees
// after a VACUUM, kept separate from the immediate reclaim so the two are not
// conflated.
func SessionStoreBytes(s model.Session) int64 {
	return s.StoreBytes
}

// SessionFootprint returns the total a sweep of s reclaims given a VACUUM:
// the immediate filesystem bytes plus the SQLite row bytes. It is the merged
// number stats reports (the SQLite rows are genuinely reclaimable, just not
// until a VACUUM runs).
func SessionFootprint(s model.Session) int64 {
	return s.ReclaimBytes + s.StoreBytes
}

// SessionTouchesStore reports whether deleting s removes SQLite rows or KV
// entries, the actions whose bytes Reclaim does not cover. Stats surfaces
// this as the store-row count so agents with inflated DB files are visible.
func SessionTouchesStore(s model.Session) bool {
	return s.TouchesStore
}
