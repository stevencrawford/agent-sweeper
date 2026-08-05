package inventory

import (
	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/paths"
)

// enumerators is the fixed six-agent registry, in display order. Each entry
// binds the agent name to its cross-platform data root and its read-only
// enumerator.
var enumerators = []enumerator{
	{name: "OpenCode", root: func() string { return paths.DataRoot("OpenCode") }, scan: scanOpenCode},
	{name: "Copilot", root: func() string { return paths.DataRoot("Copilot") }, scan: scanCopilot},
	{name: "Claude Code", root: func() string { return paths.DataRoot("Claude Code") }, scan: scanClaude},
	{name: "Codex", root: func() string { return paths.DataRoot("Codex") }, scan: scanCodex},
	{name: "Pi", root: func() string { return paths.DataRoot("Pi") }, scan: scanPi},
	{name: "Cursor", root: func() string { return paths.DataRoot("Cursor") }, scan: scanCursor},
}

// planBuilderFor returns the ordered deletion plan for one session of an agent
// (files first, record last, per decision 08). It is the single function used
// both to derive a session's reclaim accounting and to execute the sweep, so
// the dry-run footprint is exactly what a confirmed sweep deletes.
func planBuilderFor(agent, root string, s *model.Session) *engine.SessionPlan {
	switch agent {
	case "OpenCode":
		return opencodePlan(root, s)
	case "Copilot":
		return copilotPlan(root, s)
	case "Claude Code":
		return claudePlan(root, s)
	case "Codex":
		return codexPlan(root, s)
	case "Pi":
		return piPlan(root, s)
	case "Cursor":
		return cursorPlan(root, s)
	}
	return &engine.SessionPlan{ID: s.ID, Agent: agent, Title: s.Title}
}
