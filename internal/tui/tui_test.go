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

func TestGitModeExcludesNoRepoSessions(t *testing.T) {
	// OpenCode's oc-0005 has no cwd/repo (see mock); it must never appear in
	// git-repo mode, which only offers sessions that resolved to a repo (13).
	agents := mock.Agents()
	var oc []model.Session
	for _, a := range agents {
		if a.Name == "OpenCode" {
			for _, s := range a.Sessions {
				if s.Repo == "" {
					oc = append(oc, s)
				}
			}
		}
	}
	if len(oc) == 0 {
		t.Fatal("fixture needs at least one no-repo session to exercise the filter")
	}
	m := New(agents)
	for i := range m.agents {
		if m.agents[i].Name == "OpenCode" {
			m.agentIdx = i
		}
	}
	m.branches = model.GroupByRepoBranch(onlyWithRepo(m.agents[m.agentIdx].Sessions))
	for _, bg := range m.branches {
		for _, s := range bg.Sessions {
			if s.Repo == "" {
				t.Fatalf("git mode must not offer a no-repo session, got %q in branch group %q", s.ID, bg.Repo)
			}
		}
	}
}

// pivotFromGitMode drives into git mode, then escs back to the mode picker to
// check the pivot target for a following directory choice.
func pivotFromGitMode(t *testing.T) *Model {
	m := New(mock.Agents())
	m = press(t, m, "enter")        // -> mode picker
	m = press(t, m, "down")         // git
	m = press(t, m, "enter")        // -> branch picker
	m = press(t, m, "down", "down", "down", "esc") // walk branches, then esc back
	return m
}

func TestGitModeEscBackLandsOnModePicker(t *testing.T) {
	m := pivotFromGitMode(t)
	if m.screen != screenMode {
		t.Fatalf("after esc from branch picker, want mode picker, got screen %d", m.screen)
	}
}

func TestGitModeEscBackAllowsDirectoryPivot(t *testing.T) {
	m := pivotFromGitMode(t)
	// cursor must be back within the mode picker's range after esc, even when
	// the branch list cursor had moved well past it.
	if m.cursor != int(m.mode) {
		t.Fatalf("after esc, mode cursor = %d, want %d (restored to current mode)", m.cursor, int(m.mode))
	}
	// "up" climbs to directory mode; enter lands on the directory picker.
	m = press(t, m, "up")
	if m.cursor != int(modeDir) {
		t.Fatalf("up on the mode picker should select directory, got cursor %d", m.cursor)
	}
	m = press(t, m, "enter")
	if m.screen != screenDir {
		t.Fatalf("pivoting from git mode to directory mode should reach the dir picker, got screen %d", m.screen)
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
