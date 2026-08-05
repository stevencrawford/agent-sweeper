// Command agent-sweeper is a prototype stub of the interactive sweep flow.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stevencrawford/agent-sweeper/internal/mock"
	"github.com/stevencrawford/agent-sweeper/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.New(mock.Agents()), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "agent-sweeper:", err)
		os.Exit(1)
	}
}
