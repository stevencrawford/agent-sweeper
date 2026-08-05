package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/stevencrawford/agent-sweeper/internal/mock"
	"github.com/stevencrawford/agent-sweeper/internal/stats"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show each agent's session count and reclaimable footprint",
	Args:  cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		summary := stats.Compute(mock.Agents())
		return stats.Render(os.Stdout, summary)
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
