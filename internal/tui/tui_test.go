package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stevencrawford/agent-sweeper/internal/mock"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/protect"
)

func press(t *testing.T, m *Model, keys ...string) *Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(tea.KeyMsg{Type: keyType(k), Runes: []rune(k)})
		var ok bool
		m, ok = next.(*Model)
		if !ok {
			t.Fatalf("update returned unexpected model type: %T", next)
		}
	}
	return m
}

func keyType(s string) tea.KeyType {
	switch s {
	case "up", "k":
		return tea.KeyUp
	case "down", "j":
		return tea.KeyDown
	case " ":
		return tea.KeySpace
	case "enter":
		return tea.KeyEnter
	case "esc":
		return tea.KeyEsc
	}
	return tea.KeyRunes
}

func TestFlowReachesAfterReport(t *testing.T) {
	m := New(mock.Agents())
	m = press(t, m, "enter") // agent picker -> mode picker (OpenCode selected)
	if m.screen != screenMode {
		t.Fatalf("after agent enter, want mode picker, got screen %d", m.screen)
	}

	m = press(t, m, "enter") // directory mode -> dir picker
	if m.screen != screenDir {
		t.Fatalf("after mode enter, want dir picker, got screen %d", m.screen)
	}

	m = press(t, m, " ", "enter") // select first directory, continue -> age picker
	if m.screen != screenAge {
		t.Fatalf("after dir enter, want age picker, got screen %d", m.screen)
	}

	m = press(t, m, "enter") // accept default age (1d) -> dry run
	if m.screen != screenDryRun {
		t.Fatalf("after age enter, want dry run, got screen %d", m.screen)
	}

	m = press(t, m, "enter") // dry run -> confirm
	if m.screen != screenConfirm {
		t.Fatalf("after dry-run enter, want confirm, got screen %d", m.screen)
	}

	m = press(t, m, "y") // confirm -> progress
	if m.screen != screenProgress {
		t.Fatalf("after confirm, want progress, got screen %d", m.screen)
	}

	// The progress bar advances on progressMsg ticks, which only fire inside
	// a running program; drive them directly here.
	for i := 0; i < 20 && !m.done; i++ {
		next, _ := m.Update(progressMsg(1))
		var ok bool
		m, ok = next.(*Model)
		if !ok {
			t.Fatalf("update returned unexpected model type: %T", next)
		}
	}
	if !m.done {
		t.Fatal("progress never finished")
	}
	m = press(t, m, "enter")
	if m.screen != screenAfter {
		t.Fatalf("after progress completes, want after report, got screen %d", m.screen)
	}
}

func TestGitModeReachesDryRunWithBranches(t *testing.T) {
	m := New(mock.Agents())
	m = press(t, m, "enter") // -> mode picker
	m = press(t, m, "down")  // git repository
	m = press(t, m, "enter") // -> branch picker
	if m.screen != screenBranch {
		t.Fatalf("after mode enter, want branch picker, got screen %d", m.screen)
	}
	if len(m.branches) == 0 {
		t.Fatal("expected branch groups to be computed")
	}

	m = press(t, m, " ", "enter") // select first branch -> age picker
	if m.screen != screenAge {
		t.Fatalf("after branch enter, want age picker, got screen %d", m.screen)
	}
	m = press(t, m, "enter") // -> dry run
	if m.screen != screenDryRun {
		t.Fatalf("after age enter, want dry run, got screen %d", m.screen)
	}
	if !strings.Contains(m.View(), "·") {
		t.Fatalf("dry run in git mode should show repo · branch headers, got:\n%s", m.View())
	}
}

func TestDirPickerRequiresSelection(t *testing.T) {
	m := New(mock.Agents())
	m = press(t, m, "enter") // -> mode picker
	m = press(t, m, "enter") // -> dir picker
	m = press(t, m, "enter") // enter with nothing selected
	if m.screen != screenDir {
		t.Fatalf("empty selection must stay on dir picker, got screen %d", m.screen)
	}
	if !strings.Contains(m.View(), "select at least one directory") {
		t.Fatalf("expected selection error in view, got:\n%s", m.View())
	}
}

func TestEscNavigatesBack(t *testing.T) {
	m := New(mock.Agents())
	m = press(t, m, "enter") // -> mode picker
	m = press(t, m, "esc")   // back to agent
	if m.screen != screenAgent {
		t.Fatalf("after esc want agent picker, got screen %d", m.screen)
	}
}

// toDryRun drives the flow for the default agent (OpenCode) into directory
// mode, selects the first directory, and accepts the default age, landing on
// the dry run.
func toDryRun(t *testing.T, m *Model) *Model {
	t.Helper()
	m = press(t, m, "enter")          // agent -> mode
	m = press(t, m, "enter")          // directory mode -> dir picker
	m = press(t, m, " ", "enter")     // select first directory -> age picker
	m = press(t, m, "enter")          // age 1d -> dry run
	return m
}

func TestProtectedSessionExcludedAndListed(t *testing.T) {
	stub := func(a model.Agent, now time.Time) protect.Report {
		if a.Name == "OpenCode" {
			return protect.Report{"oc-0001": protect.ReasonRecent}
		}
		return protect.Report{}
	}
	m := NewWithProtection(mock.Agents(), stub)
	m = toDryRun(t, m)

	if m.screen != screenDryRun {
		t.Fatalf("want dry run, got screen %d", m.screen)
	}
	if m.protected != 1 {
		t.Fatalf("want 1 protected session, got %d", m.protected)
	}
	for _, s := range m.matches {
		if s.ID == "oc-0001" {
			t.Fatal("protected session must never appear in matches")
		}
	}
	view := m.View()
	if !strings.Contains(view, "protected") {
		t.Fatalf("dry run should list the protected session, got:\n%s", view)
	}
	if !strings.Contains(view, protect.ReasonText(protect.ReasonRecent)) {
		t.Fatalf("dry run should explain why the session is protected, got:\n%s", view)
	}
}

func TestNothingToDeleteBlocksConfirm(t *testing.T) {
	// Protect every OpenCode session so the selection has nothing to sweep.
	stub := func(a model.Agent, now time.Time) protect.Report {
		rep := protect.Report{}
		for _, s := range a.Sessions {
			if a.Name == "OpenCode" {
				rep[s.ID] = protect.ReasonRunning
			}
		}
		return rep
	}
	m := NewWithProtection(mock.Agents(), stub)
	m = press(t, m, "enter")      // agent -> mode
	m = press(t, m, "enter")      // directory mode -> dir picker
	m = press(t, m, " ", "enter") // select first directory -> age picker
	m = press(t, m, "enter")      // age 1d -> nothing to delete

	if m.screen != screenAge {
		t.Fatalf("0 matches must block the confirm, want age picker, got screen %d", m.screen)
	}
	if !strings.Contains(m.View(), "try a longer age") {
		t.Fatalf("age picker should explain nothing matches, got:\n%s", m.View())
	}
}

func TestConfirmRevalidatesProtection(t *testing.T) {
	calls := 0
	stub := func(a model.Agent, now time.Time) protect.Report {
		calls++
		// The second scan (at confirm) finds oc-0002 now open.
		if a.Name == "OpenCode" && calls >= 2 {
			return protect.Report{"oc-0002": protect.ReasonRunning}
		}
		return protect.Report{}
	}
	m := NewWithProtection(mock.Agents(), stub)
	m = toDryRun(t, m)
	if len(m.matches) != 2 {
		t.Fatalf("before confirm both oc-0001 and oc-0002 should match, got %d", len(m.matches))
	}
	m = press(t, m, "enter", "y")
	if m.screen != screenProgress {
		t.Fatalf("after confirm want progress, got screen %d", m.screen)
	}
	for _, s := range m.matches {
		if s.ID == "oc-0002" {
			t.Fatal("oc-0002 became active at confirm and must be dropped")
		}
	}
}

func TestConfirmBlocksWhenEverythingRevalidatedActive(t *testing.T) {
	calls := 0
	stub := func(a model.Agent, now time.Time) protect.Report {
		calls++
		if a.Name == "OpenCode" && calls >= 2 {
			rep := protect.Report{}
			for _, s := range a.Sessions {
				rep[s.ID] = protect.ReasonRunning
			}
			return rep
		}
		return protect.Report{}
	}
	m := NewWithProtection(mock.Agents(), stub)
	m = toDryRun(t, m)
	m = press(t, m, "enter", "y")
	if m.screen != screenDryRun {
		t.Fatalf("when every session revalidates active, confirm must block, got screen %d", m.screen)
	}
	if !strings.Contains(m.View(), "now active") {
		t.Fatalf("dry run should explain the block, got:\n%s", m.View())
	}
}
