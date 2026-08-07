// Package stats implements the `stats` command: a one-shot table of each
// agent's session count, reclaimable footprint, and store-row sessions, plus
// a grand-total row. Footprint is the merged reclaim (filesystem bytes plus
// estimated SQLite row bytes a VACUUM would free), computed with the same
// shared function the sweep dry-run uses, so the two surfaces report
// identical numbers.
package stats

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/units"
)

// Row is one agent's line in the stats table. Missing reports that the
// agent's store could not be resolved, so a zero-session row is "not
// installed/missing" rather than "no sessions".
type Row struct {
	Agent     string
	Missing   bool
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
		row := Row{Agent: a.Name, Missing: !a.Found}
		for _, sess := range a.Sessions {
			row.Sessions++
			row.Reclaim += engine.SessionFootprint(sess)
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

// Render writes the table, including a grand-total row, to w. Missing stores
// are marked (missing) so the reader can tell "not installed" from "no
// sessions".
func Render(w io.Writer, s Summary) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "AGENT\tSESSIONS\tRECLAIMABLE\tSTORE-ROW"); err != nil {
		return err
	}
	for _, r := range s.Rows {
		name := r.Agent
		if r.Missing {
			name += " (missing)"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%d\t%s\t%d\n",
			name, r.Sessions, units.Bytes(r.Reclaim), r.StoreRows); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(tw, "TOTAL\t%d\t%s\t%d\n",
		s.Sessions, units.Bytes(s.Reclaim), s.StoreRows); err != nil {
		return err
	}
	return tw.Flush()
}
