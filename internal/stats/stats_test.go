package stats

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

func testAgents() []model.Agent {
	return []model.Agent{
		{
			Name: "Pi",
			Sessions: []model.Session{
				{SizeBytes: 2048, ReclaimBytes: 2048},
				{SizeBytes: 1024, ReclaimBytes: 1024},
			},
		},
		{
			Name: "Cursor",
			Sessions: []model.Session{
				{SizeBytes: 1 << 20, ReclaimBytes: 200 << 10, TouchesStore: true},
				{SizeBytes: 1 << 20, ReclaimBytes: 100 << 10, TouchesStore: true},
			},
		},
	}
}

func TestComputeTotalsPerAgentAndGrand(t *testing.T) {
	s := Compute(testAgents())
	if len(s.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(s.Rows))
	}
	pi, cur := s.Rows[0], s.Rows[1]
	if pi.Agent != "Pi" || pi.Sessions != 2 || pi.Reclaim != 3072 || pi.StoreRows != 0 {
		t.Fatalf("Pi row = %+v, want 2 sessions / 3072 B / 0 store rows", pi)
	}
	if cur.Agent != "Cursor" || cur.Sessions != 2 || cur.Reclaim != 300<<10 || cur.StoreRows != 2 {
		t.Fatalf("Cursor row = %+v, want 2 sessions / 300 KiB / 2 store rows", cur)
	}
	if s.Sessions != 4 || s.Reclaim != 3072+300<<10 || s.StoreRows != 2 {
		t.Fatalf("totals = %+v, want 4 sessions / 3072+300KiB / 2 store rows", s)
	}
}

func TestRenderIncludesHeaderRowsAndTotal(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, Compute(testAgents())); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"AGENT", "SESSIONS", "RECLAIMABLE", "STORE-ROW", "TOTAL", "Pi", "Cursor"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "300.0KiB") {
		t.Fatalf("render should format reclaim bytes via units.Bytes:\n%s", out)
	}
}
