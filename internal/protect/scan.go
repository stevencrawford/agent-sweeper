package protect

import (
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// Protector marks the active sessions of one agent at a moment in time and
// returns why each was protected. The TUI calls it once to seed the pickers and
// again at the delete step to re-validate the survivors.
type Protector func(a model.Agent, now time.Time) Report

// ScanOne is the real Protector: it gathers the live signals for one agent
// (process list, durable markers, tentative resumes) and detects protection.
// All reads are read-only; a protected set collapses gracefully to the grace
// window when no durable signal exists.
func ScanOne(a model.Agent, now time.Time) Report {
	procs := snapshot()
	exact, resumes, running := argvFor(a.Name, procs)
	markers := gatherMarkers(&a, running)
	return Detect(a.Sessions, exact, markers, resumes, now)
}

// Scan marks in place every session across the agents that is currently open
// and returns the combined report keyed by session id.
func Scan(agents []model.Agent, now time.Time) Report {
	procs := snapshot()
	rep := Report{}
	for i := range agents {
		exact, resumes, running := argvFor(agents[i].Name, procs)
		markers := gatherMarkers(&agents[i], running)
		r := Detect(agents[i].Sessions, exact, markers, resumes, now)
		for j := range agents[i].Sessions {
			agents[i].Sessions[j].Active = r[agents[i].Sessions[j].ID] != ""
		}
		for id, reason := range r {
			if _, ok := rep[id]; !ok {
				rep[id] = reason
			}
		}
	}
	return rep
}
