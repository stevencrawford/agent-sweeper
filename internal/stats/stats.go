// Package stats implements the `stats` command: a one-shot table of each
// agent's session count, reclaimable footprint, and store-row sessions, plus
// a grand-total row. Footprint is computed with the same shared function the
// sweep dry-run uses, so the two surfaces report identical numbers.
package stats

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/units"
)

// Row is one agent's line in the stats table.
type Row struct {
	Agent     string
	Sessions  int
	Reclaim   int64
	StoreRows int
}

// Summary is the full stats output: one Row per agent plus grand totals.
type Summary struct {
	Rows      []Row
	Sessions  int
	Reclaim   int64
	StoreRows int
}

// Compute builds the summary over agents using the shared footprint function.
func Compute(agents []model.Agent) Summary {
	var s Summary
	for _, a := range agents {
		row := Row{Agent: a.Name}
		for _, sess := range a.Sessions {
			row.Sessions++
			row.Reclaim += engine.SessionReclaim(sess)
			if engine.SessionTouchesStore(sess) {
				row.StoreRows++
			}
		}
		s.Rows = append(s.Rows, row)
		s.Sessions += row.Sessions
		s.Reclaim += row.Reclaim
		s.StoreRows += row.StoreRows
	}
	return s
}

// Render writes the table, including a grand-total row, to w.
func Render(w io.Writer, s Summary) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "AGENT\tSESSIONS\tRECLAIMABLE\tSTORE-ROW"); err != nil {
		return err
	}
	for _, r := range s.Rows {
		if _, err := fmt.Fprintf(tw, "%s\t%d\t%s\t%d\n",
			r.Agent, r.Sessions, units.Bytes(r.Reclaim), r.StoreRows); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(tw, "TOTAL\t%d\t%s\t%d\n",
		s.Sessions, units.Bytes(s.Reclaim), s.StoreRows); err != nil {
		return err
	}
	return tw.Flush()
}
