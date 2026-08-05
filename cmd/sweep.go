package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/stevencrawford/agent-sweeper/internal/mock"
	"github.com/stevencrawford/agent-sweeper/internal/tui"
)

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Interactively sweep stale sessions for one agent",
	RunE: func(*cobra.Command, []string) error {
		p := tea.NewProgram(tui.New(mock.Agents()), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(sweepCmd)
}
