// Package protect detects which coding-agent sessions are currently open so a
// sweep never touches them. Protection is a reservation of session ids drawn
// from four signals observed at one moment in time: a live process's resume
// argv, durable "in use" markers (Copilot locks, Claude's background roster,
// Cursor's focused composers), a tentative resume (a process that may adopt the
// most recent session for its directory next), and a recency grace window that
// protects anything recently active — the fail-safe for agents like Pi that
// write no durable marker at all.
//
// The core (Detect) takes the observed signals as arguments so the rules are
// pure and testable; the Scan* entrypoints gather those signals from the real
// system (process list, lock files, rosters, focus store) and are read-only.
package protect

import (
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// DefaultGrace is how recent a session's last activity must be to be presumed
// live absent a durable marker (research 3's recency fail-safe): anything
// active within the window is kept.
const DefaultGrace = 24 * time.Hour

// Reason names the signal that protected a session. Each value is the display
// string; Label renders a full sentence.
type Reason string

const (
	ReasonRunning Reason = "running"
	ReasonLock    Reason = "in use"
	ReasonRoster  Reason = "live background session"
	ReasonRecent  Reason = "recently active"
	ReasonResume  Reason = "most recent for directory"
	ReasonFocused Reason = "focused in the GUI"
)

// ReasonOrder is the precedence order for deciding which reason a session shows
// when several signals protect it: an explicit marker always outweighs the
// resume fallback, which outweighs the recency window.
var ReasonOrder = []Reason{
	ReasonRunning,
	ReasonLock,
	ReasonRoster,
	ReasonFocused,
	ReasonResume,
	ReasonRecent,
}

// ReasonText is a short human sentence explaining why a session is protected.
func ReasonText(r Reason) string {
	switch r {
	case ReasonRunning:
		return "a live process is running it"
	case ReasonLock:
		return "it is held in use by a live process"
	case ReasonRoster:
		return "it is a live background session"
	case ReasonRecent:
		return "it was active within the grace window"
	case ReasonResume:
		return "a process may resume it next"
	case ReasonFocused:
		return "it is the focused session in the GUI"
	}
	return "it is in use"
}

// Mark pairs a durable-marker session id with the kind of marker that named it.
type Mark struct {
	ID     string
	Reason Reason
}

// Report maps a protected session id to the reason it is protected.
type Report map[string]Reason

// Detect is the pure protection core. Given a session list and the live signals
// observed at one instant, it reports which sessions are active and why. A live
// resume argv (exactSession args) and durable markers protect their session
// outright; a tentative resume protects the most-recent session per directory it
// could adopt; the grace window protects anything touched within DefaultGrace.
// Sessions are matched by exact id.
func Detect(sessions []model.Session, argvIDs, markerIDs []Mark, resumeDirs []string, now time.Time) Report {
	byID := make(map[string]model.Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}
	rep := Report{}

	priority := make(map[Reason]int, len(ReasonOrder))
	for i, r := range ReasonOrder {
		priority[r] = len(ReasonOrder) - i
	}
	add := func(id string, reason Reason) {
		if _, ok := byID[id]; !ok {
			return
		}
		if cur, seen := rep[id]; seen && priority[cur] >= priority[reason] {
			return
		}
		rep[id] = reason
	}

	// 1. Exact, high-confidence ids: a live resume argv and durable markers.
	for _, m := range argvIDs {
		add(m.ID, m.Reason)
	}
	for _, m := range markerIDs {
		add(m.ID, m.Reason)
	}

	// 2. Tentative resumes protect the most recent session per resume dir.
	for _, dir := range resumeDirs {
		if s, ok := newestInDir(sessions, dir); ok {
			add(s.ID, ReasonResume)
		}
	}

	// 3. Grace window: any session touched within DefaultGrace is presumed live.
	for _, s := range sessions {
		if now.Sub(s.LastActivity) < DefaultGrace {
			add(s.ID, ReasonRecent)
		}
	}

	return rep
}

// newestInDir returns the session whose LastActivity is newest among the
// sessions whose CWD exactly equals dir. A zero dir matches every session and
// returns the newest overall.
func newestInDir(sessions []model.Session, dir string) (model.Session, bool) {
	var best model.Session
	var found bool
	for _, s := range sessions {
		if dir != "" && s.CWD != dir {
			continue
		}
		if !found || s.LastActivity.After(best.LastActivity) {
			best = s
			found = true
		}
	}
	return best, found
}
