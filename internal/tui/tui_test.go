package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stevencrawford/agent-sweeper/internal/mock"
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
