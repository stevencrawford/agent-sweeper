package cmd

import (
	"testing"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/inventory"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/protect"
)

// stubAgent returns a two-session agent fixture: one recent (2d old) and one
// stale (60d old), both in the same working directory, the stale one carrying
// an empty-action plan.
func stubAgent() (model.Agent, inventory.Inventory) {
	now := time.Now()
	recent := model.Session{ID: "r-1", Title: "recent", CWD: "/w/agent-sweeper", LastActivity: now.Add(-2 * 24 * time.Hour)}
	stale := model.Session{ID: "s-1", Title: "stale", CWD: "/w/agent-sweeper", LastActivity: now.Add(-60 * 24 * time.Hour)}
	a := model.Agent{Name: "TestAgent", Found: true, Sessions: []model.Session{recent, stale}}
	inv := inventory.Inventory{
		Agents: []model.Agent{a},
		Plans: map[string]*engine.SessionPlan{
			inventory.PlanKey(a.Name, recent.ID): {ID: recent.ID, Agent: a.Name, Title: recent.Title},
			inventory.PlanKey(a.Name, stale.ID):  {ID: stale.ID, Agent: a.Name, Title: stale.Title},
		},
	}
	return a, inv
}

// noneProtects is a stub protector that protects nothing.
func noneProtects(model.Agent, time.Time) protect.Report { return protect.Report{} }

// protectStale protects the stale session id only.
func protectStale(a model.Agent, _ time.Time) protect.Report {
	return protect.Report{"s-1": protect.ReasonRunning}
}

func TestBuildScriptedPlanRespectsAge(t *testing.T) {
	a, inv := stubAgent()
	age, _ := ageForLabel("30d")

	plan, err := buildScriptedPlan(inv, a, age, noneProtects)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SessionCount() != 1 {
		t.Fatalf("30d age should match only the stale session, got %d", plan.SessionCount())
	}
	if plan.Sessions[0].ID != "s-1" {
		t.Fatalf("expected s-1, got %s", plan.Sessions[0].ID)
	}
}

func TestBuildScriptedPlanExcludesProtected(t *testing.T) {
	a, inv := stubAgent()
	age, _ := ageForLabel("all")

	plan, err := buildScriptedPlan(inv, a, age, protectStale)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SessionCount() != 1 || plan.Sessions[0].ID != "r-1" {
		t.Fatalf("protected s-1 must never be planned; got %d sessions", plan.SessionCount())
	}
}

func TestBuildScriptedPlanNothingToDelete(t *testing.T) {
	a, inv := stubAgent()
	age, _ := ageForLabel("all")
	allProtected := func(a model.Agent, _ time.Time) protect.Report {
		return protect.Report{"r-1": protect.ReasonRunning, "s-1": protect.ReasonRunning}
	}

	if _, err := buildScriptedPlan(inv, a, age, allProtected); err == nil {
		t.Fatal("all sessions protected must error as nothing to delete")
	}
}

func TestBuildScriptedPlanDirFilter(t *testing.T) {
	a, inv := stubAgent()
	age, _ := ageForLabel("all")
	old := sweepFlags.dir
	sweepFlags.dir = "/nowhere"
	defer func() { sweepFlags.dir = old }()

	if _, err := buildScriptedPlan(inv, a, age, noneProtects); err == nil {
		t.Fatal("dir filter with no matches must error")
	}
}

func TestBuildScriptedPlanNoPlanAvailable(t *testing.T) {
	a := model.Agent{Name: "TestAgent", Found: true, Sessions: []model.Session{
		{ID: "s-1", LastActivity: time.Now().Add(-60 * 24 * time.Hour)},
	}}
	inv := inventory.Inventory{Agents: []model.Agent{a}, Plans: map[string]*engine.SessionPlan{}}
	age, _ := ageForLabel("all")

	if _, err := buildScriptedPlan(inv, a, age, noneProtects); err == nil {
		t.Fatal("a session with no plan must error")
	}
}

func TestPickAgentKnownAndUnknown(t *testing.T) {
	_, inv := stubAgent()
	if _, err := pickAgent(inv, "TestAgent"); err != nil {
		t.Fatalf("known agent should resolve, got %v", err)
	}
	if _, err := pickAgent(inv, "Nope"); err == nil {
		t.Fatal("unknown agent must error")
	}
}
