package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/inventory"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/protect"
	"github.com/stevencrawford/agent-sweeper/internal/tui"
	"github.com/stevencrawford/agent-sweeper/internal/units"
)

var sweepFlags struct {
	demo   bool
	agent  string
	mode   string
	dir    string
	repo   string
	branch string
	age    string
	yes    bool
	json   bool
	quiet  bool
	dryRun bool
	vacuum bool
}

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Sweep stale sessions for one agent",
	Long: `Sweep cleans stale sessions interactively (the TUI) or, with --agent and
--yes, non-interactively for automation. Active sessions are always protected;
--yes is a required, explicit confirmation and never bypasses protection.
--dry-run prints the to-be-deleted plan and exits without deleting anything.`,
	RunE: runSweep,
}

func init() {
	f := sweepCmd.Flags()
	f.BoolVar(&sweepFlags.demo, "demo", false, "use the mock dataset instead of real session stores")
	f.StringVar(&sweepFlags.agent, "agent", "", "agent to sweep (non-interactive)")
	f.StringVar(&sweepFlags.mode, "mode", "dir", "grouping for selection: dir|git (with --agent)")
	f.StringVar(&sweepFlags.dir, "dir", "", "only sessions whose working directory equals DIR (with --agent)")
	f.StringVar(&sweepFlags.repo, "repo", "", "only sessions in REPO (with --agent, git mode)")
	f.StringVar(&sweepFlags.branch, "branch", "", "only sessions on BRANCH (with --agent, git mode)")
	f.StringVar(&sweepFlags.age, "age", "", "only sessions older than AGE: 1d|3d|7d|30d|90d|1y|all")
	f.BoolVar(&sweepFlags.yes, "yes", false, "non-interactive: confirm deletions (required, never implied)")
	f.BoolVar(&sweepFlags.dryRun, "dry-run", false, "print the plan and exit without deleting (works with or without --yes)")
	f.BoolVar(&sweepFlags.json, "json", false, "emit machine-readable JSON")
	f.BoolVar(&sweepFlags.quiet, "quiet", false, "print a one-line summary only")
	f.BoolVar(&sweepFlags.vacuum, "vacuum", false, "run VACUUM on swept stores after deleting rows (with --yes)")
	rootCmd.AddCommand(sweepCmd)
}

func runSweep(*cobra.Command, []string) error {
	if sweepFlags.demo {
		return runSweepDemo()
	}
	inv := withSpinner("indexing session stores", inventory.Find)
	if sweepFlags.agent != "" || sweepFlags.yes || sweepFlags.dryRun {
		return runSweepScripted(inv)
	}
	return runSweepInteractive(inv)
}

// runSweepInteractive starts the bubbletea TUI over the real agents and their
// plans, protecting active sessions against a live scan. It renders inline
// (no alternate screen) so the terminal scrollback and surrounding context stay
// intact. Only agents discovered on this machine are offered.
func runSweepInteractive(inv inventory.Inventory) error {
	agents := inv.Discovered()
	if len(agents) == 0 {
		return fmt.Errorf("no agent session stores found on this machine")
	}
	p := tea.NewProgram(
		tui.NewWithProtection(agents, protect.ScanOne, inv.Plans),
		tea.WithInputTTY())
	_, err := p.Run()
	return err
}

// runSweepDemo runs the interactive flow over the mock dataset, which carries
// no deletion plans, so the confirm step is execution-disabled.
func runSweepDemo() error {
	p := tea.NewProgram(
		tui.NewWithProtection(inventory.Demo(), protect.ScanOne, nil),
		tea.WithInputTTY())
	_, err := p.Run()
	return err
}

// runSweepScripted runs the sweep against a real store with flags only. It
// reuses the same protection and age rules as the TUI and never sweeps an open
// session, even with --yes.
func runSweepScripted(inv inventory.Inventory) error {
	agent, err := pickAgent(inv, sweepFlags.agent)
	if err != nil {
		return err
	}
	if !agent.Found {
		return fmt.Errorf("no %s session store found at %s", agent.Name, agent.DataRoot)
	}
	if !sweepFlags.yes && !sweepFlags.dryRun {
		return fmt.Errorf("refusing to run non-interactively without --yes (or pass --dry-run to preview)")
	}
	age, ok := ageForLabel(sweepFlags.age)
	if !ok {
		return fmt.Errorf("unknown age %q (want 1d|3d|7d|30d|90d|1y|all)", sweepFlags.age)
	}

	plan, err := buildScriptedPlan(inv, *agent, age, protect.ScanOne)
	if err != nil {
		return err
	}

	printDryRun(plan, age, agent.Name)
	if sweepFlags.dryRun {
		return writeDryRunResult(plan, age, agent.Name)
	}
	res := engine.Execute(context.Background(), plan)
	if err := writeSweepResult(res, plan, agent.Name); err != nil {
		return err
	}
	return maybeVacuum(context.Background(), plan)
}

// maybeVacuum offers the post-sweep VACUUM that turns deleted SQLite rows back
// into free disk. In --yes automation it runs only when --vacuum is passed; a
// --vacuum without --yes (interactive) still asks the user to confirm because
// VACUUM rewrites the whole store file and needs an exclusive lock. A store
// held by a live agent process is skipped.
func maybeVacuum(ctx context.Context, plan *engine.Plan) error {
	stores := plan.Stores()
	if len(stores) == 0 {
		return nil
	}
	if !sweepFlags.vacuum {
		if !sweepFlags.quiet && !sweepFlags.json {
			fmt.Fprintln(os.Stderr, "store rows were deleted — pass --vacuum to VACUUM the SQLite file and return the space to disk")
		}
		return nil
	}
	sizes, err := engine.VacuumSizes(ctx, stores)
	if err != nil {
		return err
	}
	anyRows := false
	for _, info := range sizes {
		if info.FreelistBytes > 0 {
			anyRows = true
		}
	}
	if !sweepFlags.yes && !sweepFlags.quiet && !sweepFlags.json {
		fmt.Printf("VACUUM would free %s across %d SQLite file(s) — confirm? [y/N] ",
			units.Bytes(totalFree(sizes)), len(sizes))
		var reply string
		if _, err := fmt.Scanln(&reply); err != nil {
			reply = ""
		}
		if reply != "y" && reply != "Y" && reply != "yes" {
			return nil
		}
	}
	if !anyRows {
		return nil
	}
	var total int64
	for _, info := range sizes {
		freed, err := engine.Vacuum(ctx, info.Store)
		if err != nil {
			if !sweepFlags.quiet {
				fmt.Fprintf(os.Stderr, "vacuum %s: %v\n", info.Store, err)
			}
			continue
		}
		total += freed
		if !sweepFlags.quiet && !sweepFlags.json {
			fmt.Printf("vacuumed %s, freed %s\n", info.Store, units.Bytes(freed))
		}
	}
	if sweepFlags.json {
		out := map[string]any{"vacuumed": true, "vacuumedBytes": total}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	return nil
}

func totalFree(sizes []engine.VacuumInfo) int64 {
	var total int64
	for _, info := range sizes {
		total += info.FreelistBytes
	}
	return total
}

// buildScriptedPlan applies the mode/dir/repo/branch filters, the age bucket,
// and live protection to one agent's sessions, returning the ordered plan of
// what a confirmed sweep would delete. It never executes anything.
func buildScriptedPlan(inv inventory.Inventory, agent model.Agent, age model.Age, protectFn func(model.Agent, time.Time) protect.Report) (*engine.Plan, error) {
	selected := selectSessions(agent, sweepFlags.mode, sweepFlags.dir, sweepFlags.repo, sweepFlags.branch)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no sessions match the given filters")
	}

	protected := protectFn(agent, time.Now())
	var matches []model.Session
	for _, s := range selected {
		if _, open := protected[s.ID]; open {
			continue
		}
		if !age.Matches(s.LastActivity) {
			continue
		}
		matches = append(matches, s)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("nothing to delete — every matching session is active or newer than %s", age.Label)
	}

	plan := &engine.Plan{}
	for _, s := range matches {
		if p := inv.Plan(agent, s); p != nil {
			plan.Sessions = append(plan.Sessions, p)
		}
	}
	if len(plan.Sessions) == 0 {
		return nil, fmt.Errorf("no deletion plans are available for %s", agent.Name)
	}
	return plan, nil
}

// pickAgent resolves the --agent flag to an inventory agent, listing the known
// agents when the name is unknown.
func pickAgent(inv inventory.Inventory, name string) (*model.Agent, error) {
	if name == "" {
		return nil, fmt.Errorf("--agent is required in non-interactive mode")
	}
	for i := range inv.Agents {
		if inv.Agents[i].Name == name {
			return &inv.Agents[i], nil
		}
	}
	known := make([]string, 0, len(inv.Agents))
	for _, a := range inv.Agents {
		known = append(known, a.Name)
	}
	return nil, fmt.Errorf("unknown agent %q (known: %s)", name, joinList(known))
}

// selectSessions narrows an agent's sessions by mode and filters.
func selectSessions(a model.Agent, mode, dir, repo, branch string) []model.Session {
	out := make([]model.Session, 0, len(a.Sessions))
	for _, s := range a.Sessions {
		if dir != "" {
			if s.CWD == dir {
				out = append(out, s)
			}
			continue
		}
		if repo != "" {
			if s.Repo != repo {
				continue
			}
			if branch != "" && s.Branch != branch {
				continue
			}
			out = append(out, s)
			continue
		}
		if mode == "git" || mode == "repo" {
			if s.Repo != "" {
				out = append(out, s)
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// ageForLabel resolves an age flag to a model.Age bucket.
func ageForLabel(label string) (model.Age, bool) {
	if label == "" {
		label = "all"
	}
	for _, a := range model.Ages {
		if a.All && label == "all" {
			return a, true
		}
		if !a.All && a.Label == label {
			return a, true
		}
	}
	return model.Age{}, false
}

// printDryRun shows the before-footprint the sweep will delete, matching the
// TUI's dry-run so --yes automation sees the same contract.
func printDryRun(plan *engine.Plan, age model.Age, agent string) {
	if sweepFlags.quiet || sweepFlags.json {
		return
	}
	fmt.Printf("target %s · older than %s\n", agent, age.Label)
	for _, sp := range plan.Sessions {
		fmt.Printf("  delete %s (%s, %s)\n", sp.ID, sp.Title, units.Bytes(sp.Reclaim()))
	}
	total := plan.Reclaim()
	fmt.Printf("would delete %d sessions, reclaiming %s\n", plan.SessionCount(), units.Bytes(total))
	if st := plan.StoreBytes(); st > 0 {
		fmt.Printf("  …plus %s in SQLite rows freed by a post-sweep VACUUM\n", units.Bytes(st))
	}
}

// sessionOutcome is one session's result in the JSON report.
type sessionOutcome struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ReclaimBytes int64  `json:"reclaimBytes"`
	StoreBytes   int64  `json:"storeBytes"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
}

// sweepReport is the machine-readable sweep outcome.
type sweepReport struct {
	Agent          string           `json:"agent"`
	DryRun         bool             `json:"dryRun"`
	Requested      int              `json:"requested"`
	Deleted        int              `json:"deleted"`
	Failed         int              `json:"failed"`
	ReclaimedBytes int64            `json:"reclaimedBytes"`
	StoreBytes     int64            `json:"storeBytes"`
	NeedsVacuum    bool             `json:"needsVacuum"`
	Sessions       []sessionOutcome `json:"sessions"`
}

// writeDryRunResult reports the would-be footprint without touching the store.
// In JSON mode it emits the plan as a sweepReport with DryRun=true; otherwise
// printDryRun has already shown the human listing, so nothing more is printed.
func writeDryRunResult(plan *engine.Plan, age model.Age, agent string) error {
	if !sweepFlags.json {
		if !sweepFlags.quiet {
			fmt.Println("dry run — nothing was deleted")
		}
		return nil
	}
	rep := sweepReport{Agent: agent, DryRun: true}
	for _, sp := range plan.Sessions {
		rep.Requested++
		rep.ReclaimedBytes += sp.Reclaim()
		rep.StoreBytes += sp.StoreBytes()
		rep.NeedsVacuum = rep.NeedsVacuum || sp.HasStoreActions()
		rep.Sessions = append(rep.Sessions, sessionOutcome{
			ID: sp.ID, Title: sp.Title, ReclaimBytes: sp.Reclaim(), StoreBytes: sp.StoreBytes(), OK: true,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func writeSweepResult(res *engine.Result, plan *engine.Plan, agent string) error {
	if sweepFlags.json {
		rep := sweepReport{
			Agent:          agent,
			Deleted:        res.Deleted(),
			Failed:         res.Failed(),
			ReclaimedBytes: res.BytesReclaimed(),
			NeedsVacuum:    res.NeedsVacuum(),
			Sessions:       make([]sessionOutcome, 0, len(res.Sessions)),
		}
	for _, sr := range res.Sessions {
			o := sessionOutcome{
				ID:           sr.Session.ID,
				Title:        sr.Session.Title,
				ReclaimBytes: sr.Session.Reclaim(),
				StoreBytes:   sr.Session.StoreBytes(),
				OK:           sr.Err == nil,
			}
			if sr.Err != nil {
				o.Error = sr.Err.Error()
			}
			rep.StoreBytes += o.StoreBytes
			rep.Sessions = append(rep.Sessions, o)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	if sweepFlags.quiet {
		fmt.Printf("%s: deleted %d, failed %d, reclaimed %s\n",
			agent, res.Deleted(), res.Failed(), units.Bytes(res.BytesReclaimed()))
		return nil
	}
	fmt.Printf("deleted %d sessions from %s, reclaiming %s\n", res.Deleted(), agent, units.Bytes(res.BytesReclaimed()))
	if st := plan.StoreBytes(); st > 0 {
		fmt.Fprintf(os.Stderr, "…%s more in SQLite rows will be freed by a VACUUM\n", units.Bytes(st))
	}
	if n := res.Failed(); n > 0 {
		fmt.Fprintf(os.Stderr, "%d sessions failed and were left for a re-run\n", n)
	}
	if res.NeedsVacuum() && !sweepFlags.vacuum {
		fmt.Fprintln(os.Stderr, "store rows were deleted — pass --vacuum to return the space to disk")
	}
	return nil
}

func joinList(items []string) string {
	return strings.Join(items, ", ")
}
