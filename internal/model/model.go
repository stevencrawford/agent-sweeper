// Package model holds the shared domain types for the sweep flow: agents,
// their sessions, and the directory-group view the pickers operate on.
package model

import (
	"sort"
	"time"
)

// Agent is one of the six detected coding agents, with the sessions found
// under its data root.
type Agent struct {
	Name     string
	DataRoot string
	Sessions []Session
	// Found is false when the agent's data root could not be resolved to an
	// existing store, so a caller can distinguish "not installed or missing"
	// from "installed but no sessions".
	Found bool
}

// Session is a single sweepable session: a working directory it ran in, its
// last activity, the bytes its deletion reclaims, and whether it is currently
// open (and therefore protected).
type Session struct {
	ID           string
	Title        string
	CWD          string // canonical path; grouping key for directory mode
	Label        string // original spelling as stored; display label
	Repo         string // git repository identity when known (git-repo mode)
	Branch       string // branch at session time (git-repo mode)
	LastActivity time.Time
	// SizeBytes is the detection-time raw size across the session's stores.
	// It is never displayed directly.
	SizeBytes int64
	// ReclaimBytes is the plan-based footprint: Remove* artifact bytes this
	// session's sweep reclaims immediately. It is the filesystem portion of
	// the footprint reported by stats and the sweep dry-run.
	ReclaimBytes int64
	// StoreBytes is the estimated SQLite row bytes a sweep of this session
	// frees after a VACUUM: the data bytes of the message/part/event/etc rows
	// its plan deletes. SQLite only returns this to disk after a VACUUM, so it
	// is surfaced alongside ReclaimBytes, never lumped into an "immediate"
	// reclaim number.
	StoreBytes int64
	// Children lists the descendant session ids this session owns (opencode
	// compacted/abstract children reachable via parent_id). Sweeping this
	// session deletes its whole subtree; children are not listed standalone
	// (decision: recurse children).
	Children []string
	// TouchesStore reports whether deleting this session removes SQLite rows
	// or KV entries, whose surface is unchanged until VACUUM. Surfaced by
	// stats as the store-row count.
	TouchesStore bool
	// Active is true when the session is currently open; such sessions are
	// protected and never swept.
	Active bool
}

// Group buckets sessions by their canonical cwd. A zero Path is the
// anonymous "(no directory)" bucket for sessions without a usable cwd.
type Group struct {
	Path     string
	Label    string
	Sessions []Session
}

// BranchGroup buckets sessions by git repo and branch, for git-repo mode.
type BranchGroup struct {
	Repo     string
	Branch   string
	Sessions []Session
}

// SessionCount returns how many sessions the agent holds.
func (a *Agent) SessionCount() int {
	return len(a.Sessions)
}

// Footprint returns the total reclaimable bytes across all of the agent's
// sessions — the merged number (filesystem bytes plus SQLite row bytes that a
// VACUUM frees) that stats and the sweep dry-run report.
func (a *Agent) Footprint() int64 {
	var total int64
	for _, s := range a.Sessions {
		total += s.ReclaimBytes + s.StoreBytes
	}
	return total
}

// GroupCount returns how many sessions the group holds.
func (g *Group) GroupCount() int {
	return len(g.Sessions)
}

// GroupCount returns how many sessions the branch group holds.
func (b *BranchGroup) GroupCount() int {
	return len(b.Sessions)
}

// GroupByRepoBranch buckets sessions by repo then branch, descending
// session-count with path tie-breaks, so the noisiest branches sort first.
func GroupByRepoBranch(sessions []Session) []BranchGroup {
	groups := map[string]*BranchGroup{}
	for _, s := range sessions {
		key := s.Repo + "\x00" + s.Branch
		g, ok := groups[key]
		if !ok {
			g = &BranchGroup{Repo: s.Repo, Branch: s.Branch}
			groups[key] = g
		}
		g.Sessions = append(g.Sessions, s)
	}
	all := make([]BranchGroup, 0, len(groups))
	for _, g := range groups {
		all = append(all, *g)
	}
	sort.Slice(all, func(i, j int) bool {
		if len(all[i].Sessions) != len(all[j].Sessions) {
			return len(all[i].Sessions) > len(all[j].Sessions)
		}
		if all[i].Repo != all[j].Repo {
			return all[i].Repo < all[j].Repo
		}
		return all[i].Branch < all[j].Branch
	})
	return all
}

// GroupByCWD buckets sessions by canonical cwd, in descending session-count
// order (ties broken by path, anonymous bucket last) so the noisiest
// directories sit at the top of the picker.
func GroupByCWD(sessions []Session) []Group {
	groups := map[string]*Group{}
	for _, s := range sessions {
		g, ok := groups[s.CWD]
		if !ok {
			g = &Group{Path: s.CWD, Label: s.Label}
			groups[s.CWD] = g
		}
		g.Sessions = append(g.Sessions, s)
	}
	all := make([]Group, 0, len(groups))
	for _, g := range groups {
		all = append(all, *g)
	}
	sort.Slice(all, func(i, j int) bool {
		if len(all[i].Sessions) != len(all[j].Sessions) {
			return len(all[i].Sessions) > len(all[j].Sessions)
		}
		if (all[i].Path == "") != (all[j].Path == "") {
			return all[j].Path == ""
		}
		return all[i].Path < all[j].Path
	})
	return all
}

// Age is one entry in the age-picker enum.
type Age struct {
	Label    string
	Duration time.Duration
	All      bool
}

// Ages is the enum offered by the age picker, in sweep order. "all" bypasses
// the age comparison entirely; every other bucket matches strictly older
// sessions.
var Ages = []Age{
	{Label: "1d", Duration: 24 * time.Hour},
	{Label: "3d", Duration: 3 * 24 * time.Hour},
	{Label: "7d", Duration: 7 * 24 * time.Hour},
	{Label: "30d", Duration: 30 * 24 * time.Hour},
	{Label: "90d", Duration: 90 * 24 * time.Hour},
	{Label: "1y", Duration: 365 * 24 * time.Hour},
	{Label: "all", All: true},
}

// Matches reports whether a session's last activity falls inside this bucket.
func (a *Age) Matches(activity time.Time) bool {
	if a.All {
		return true
	}
	return time.Since(activity) > a.Duration
}
