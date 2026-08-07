package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/stevencrawford/agent-sweeper/internal/inventory"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/stats"
)

var statsFlags struct {
	demo bool
	json bool
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show each agent's session count and reclaimable footprint",
	Args:  cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		var agents = withSpinner("indexing session stores", func() []model.Agent {
			inv := inventory.Find()
			return inv.Discovered()
		})
		if statsFlags.demo {
			agents = inventory.Demo()
		}
		summary := stats.Compute(agents)
		if statsFlags.json {
			return writeStatsJSON(summary)
		}
		return stats.Render(os.Stdout, summary)
	},
}

func init() {
	statsCmd.Flags().BoolVar(&statsFlags.demo, "demo", false, "use the mock dataset instead of real session stores")
	statsCmd.Flags().BoolVar(&statsFlags.json, "json", false, "emit machine-readable JSON")
	rootCmd.AddCommand(statsCmd)
}

func writeStatsJSON(s stats.Summary) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
