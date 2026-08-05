// Package mock provides the six-agent dataset the TUI prototype renders.
// The shape — agent names, session-store roots, per-session cwds, sizes, and
// last-activity — mirrors the research findings in .scratch/agent-sweeper/
// (platform-paths.md, artifact-inventory.md), but is synthetic: no real
// session stores are read.
package mock

import (
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// Agents returns the six known agents with a small, plausible session set.
func Agents() []model.Agent {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour

	return []model.Agent{
		{
			Name:     "OpenCode",
			DataRoot: "~/.local/share/opencode",
			Sessions: []model.Session{
				{ID: "oc-0001", Title: "Add wayfinder skill", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", Repo: "github.com/stevencrawford/agent-sweeper", Branch: "main", LastActivity: ago(2 * day), SizeBytes: 128 << 20},
				{ID: "oc-0002", Title: "bubbletea prototype", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", Repo: "github.com/stevencrawford/agent-sweeper", Branch: "feature/tui", LastActivity: ago(6 * day), SizeBytes: 96 << 20},
				{ID: "oc-0003", Title: "Fix adapter registry", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", Repo: "github.com/stevencrawford/omnivue", Branch: "main", LastActivity: ago(20 * day), SizeBytes: 12 << 20},
				{ID: "oc-0004", Title: "Cursor transcript scan", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", Repo: "github.com/stevencrawford/omnivue", Branch: "fix/cursor", LastActivity: ago(45 * day), SizeBytes: 220 << 20},
				{ID: "oc-0005", Title: "session-store schema notes", CWD: "", Label: "", LastActivity: ago(90 * day), SizeBytes: 4 << 20},
			},
		},
		{
			Name:     "Copilot",
			DataRoot: "~/.copilot",
			Sessions: []model.Session{
				{ID: "cp-001", Title: "migrate deploy pipeline", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(3 * day), SizeBytes: 9 << 20},
				{ID: "cp-002", Title: "rewind snapshot test", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(35 * day), SizeBytes: 61 << 20, Active: true},
				{ID: "cp-003", Title: "session-state dir walk", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(60 * day), SizeBytes: 33 << 20},
				{ID: "cp-004", Title: "plan.md cleanup", CWD: "/Users/stcrawfo/Development/javascript/side-project", Label: "/Users/stcrawfo/Development/javascript/side-project", LastActivity: ago(200 * day), SizeBytes: 12 << 20},
			},
		},
		{
			Name:     "Claude Code",
			DataRoot: "~/.claude",
			Sessions: []model.Session{
				{ID: "cc-aaa1", Title: "grill ticket 06", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(1 * day), SizeBytes: 5 << 20},
				{ID: "cc-aaa2", Title: "age enum semantics", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(10 * day), SizeBytes: 11 << 20},
				{ID: "cc-bbb1", Title: "omnivue frontend polish", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(80 * day), SizeBytes: 2 << 20},
				{ID: "cc-bbb2", Title: "todos tidy", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(120 * day), SizeBytes: 1 << 20},
			},
		},
		{
			Name:     "Codex",
			DataRoot: "~/.codex",
			Sessions: []model.Session{
				{ID: "cx-77", Title: "goreleaser config", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(4 * day), SizeBytes: 3 << 20},
				{ID: "cx-78", Title: "release pipeline", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(7 * day), SizeBytes: 1 << 20},
				{ID: "cx-99", Title: "codex tui log", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(150 * day), SizeBytes: 7 << 20, Active: true},
			},
		},
		{
			Name:     "Pi",
			DataRoot: "~/.pi/agent/sessions",
			Sessions: []model.Session{
				{ID: "pi-sess-1", Title: "pi adapter smoke test", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(12 * day), SizeBytes: 1 << 20},
				{ID: "pi-sess-2", Title: "jsonl header parse", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(300 * day), SizeBytes: 1 << 20},
			},
		},
		{
			Name:     "Cursor",
			DataRoot: "~/Library/Application Support/Cursor",
			Sessions: []model.Session{
				{ID: "cur-1", Title: "state.vscdb kv probe", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(5 * day), SizeBytes: 2 << 20},
				{ID: "cur-2", Title: "composerData keys", CWD: "/Users/stcrawfo/Development/javascript/agent-sweeper", Label: "/Users/stcrawfo/Development/javascript/agent-sweeper", LastActivity: ago(8 * day), SizeBytes: 14 << 20},
				{ID: "cur-3", Title: "transcripts walk", CWD: "/Users/stcrawfo/Development/javascript/omnivue", Label: "/Users/stcrawfo/Development/javascript/omnivue", LastActivity: ago(70 * day), SizeBytes: 3 << 20, Active: true},
			},
		},
	}
}
