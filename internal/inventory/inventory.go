// Package inventory is the real session-store detection seam: the one place
// that enumerates each supported coding agent's actual on-disk session store
// into []model.Session and the per-session deletion plans that back them. It
// is strictly read-only — detection and stats never open a store for writing.
// Mock data is a test fixture / --demo fallback only; every command surfaces
// real detection through this package.
package inventory

import (
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/mock"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// Inventory is the real-detection result: the agents (for stats, sweep, and
// protection) plus the per-session deletion plans (for execution).
type Inventory struct {
	Agents []model.Agent
	// Plans maps PlanKey(agent, session-id) to the session's ordered deletion
	// plan. Every plan is built once at detection time, so the dry-run
	// footprint (the sum of each session's ReclaimBytes, itself the plan's
	// remove-artifact bytes) and the executed sweep can never disagree
	// (dry-run==confirm).
	Plans map[string]*engine.SessionPlan
}

// PlanKey is the map key binding a plan to its session. Agent names share
// session-id formats across agents (everyone uses UUIDs), so the key is
// qualified by agent.
func PlanKey(agent, id string) string { return agent + "\x00" + id }

// Plan returns the deletion plan for a session of an agent, or nil.
func (in *Inventory) Plan(agent model.Agent, s model.Session) *engine.SessionPlan {
	return in.Plans[PlanKey(agent.Name, s.ID)]
}

// Find enumerates every known agent's real session store. Missing or unreadable
// stores yield an agent with Found=false and no sessions. Detection never
// mutates a store: the SQLite readers open read-only and the file walks only
// stat and read first lines.
func Find() Inventory {
	inv := Inventory{Plans: map[string]*engine.SessionPlan{}}
	for _, e := range enumerators {
		inv.add(e)
	}
	return inv
}

// FindAgent is Find scoped to one agent name (scriptable mode's --agent). The
// agent is still returned (with Found=false) when the store is missing so
// callers can report "no store rather than no such agent.
func FindAgent(name string) Inventory {
	inv := Inventory{Plans: map[string]*engine.SessionPlan{}}
	for _, e := range enumerators {
		if e.name == name {
			inv.add(e)
		}
	}
	return inv
}

// Demo returns the mock dataset as a fallback toggle for --demo and tests. It
// carries no plans (nothing is executable against synthetic sessions).
func Demo() []model.Agent {
	agents := mock.Agents()
	for i := range agents {
		agents[i].Found = true
	}
	return agents
}

// add runs one enumerator against its root and appends the resulting agent and
// its plans to inv.
func (in *Inventory) add(e enumerator) {
	root := e.root()
	a := model.Agent{Name: e.name, DataRoot: root}
	if !exists(root) {
		in.Agents = append(in.Agents, a)
		return
	}
	a.Found = true
	sessions, err := e.scan(root, time.Now())
	if err != nil {
		// An unreadable store degrades to zero sessions rather than failing the
		// whole sweep: detection is best-effort and every command still runs.
		sessions = nil
	}
	for i := range sessions {
		plan := planBuilderFor(a.Name, root, &sessions[i])
		sessions[i].ReclaimBytes = plan.Reclaim()
		sessions[i].TouchesStore = plan.HasStoreActions()
		in.Plans[PlanKey(a.Name, sessions[i].ID)] = plan
	}
	a.Sessions = sessions
	in.Agents = append(in.Agents, a)
}
