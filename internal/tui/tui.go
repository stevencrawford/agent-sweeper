// Package tui implements the sweep flow as a bubbletea program: agent picker,
// directory picker, age picker, dry-run report, single confirm, delete
// progress, and after-footprint report. This is a prototype stub — the data
// comes from mock, no real session stores are touched.
package tui

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/inventory"
	"github.com/stevencrawford/agent-sweeper/internal/model"
	"github.com/stevencrawford/agent-sweeper/internal/protect"
	"github.com/stevencrawford/agent-sweeper/internal/units"
)

type screen int

const (
	screenAgent screen = iota
	screenMode
	screenDir
	screenBranch
	screenAge
	screenDryRun
	screenConfirm
	screenProgress
	screenAfter
)

type groupMode int

const (
	modeDir groupMode = iota
	modeGit
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).MarginBottom(1)
	rowStyle   = lipgloss.NewStyle().PaddingLeft(1)
	selStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).PaddingLeft(1)
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

type Model struct {
	width  int
	height int
	screen screen

	agents   []model.Agent
	agentIdx int
	cursor   int

	groups   []model.Group
	branches []model.BranchGroup
	mode     groupMode
	selected map[int]bool
	err      string

	ageIdx int

	matches      []model.Session
	reclaimBytes int64
	protected    int

	// offset is the first row of the current windowing view: the visible slice
	// of a long picker list or dry-run report. It advances with the cursor on
	// selection screens and directly with PgUp/PgDn on the paged report.
	offset int

	// protector marks which sessions are currently open. It is run once at
	// load to seed the pickers (via NewWithProtection) and again at confirm,
	// so deletion never touches a session that became active after the
	// dry-run. nil disables protection (tests, prototype data).
	protector protect.Protector
	// active maps a protected session id to why it is protected, for the
	// current sweep's dry-run display.
	active map[string]protect.Reason

	progress float64
	done     bool

	// plans binds each detected session to its ordered deletion plan, the same
	// plan whose remove bytes produced the dry-run footprint. At confirm the
	// matched sessions' plans are executed so dry-run==confirm holds on real
	// data. Nil disables real execution (tests, --demo).
	plans map[string]*engine.SessionPlan
	// result is the engine outcome of the confirmed sweep, rendered on the
	// after screen.
	result *engine.Result
}

// New builds the sweep TUI over the given agents.
func New(agents []model.Agent) *Model {
	return &Model{
		agents:   agents,
		selected: map[int]bool{},
		active:   map[string]protect.Reason{},
	}
}

// NewWithProtection builds the TUI with a live protector, marking every
// currently-open session Active before the first render. plans binds the
// real detection plans (may be nil for a demo/test run).
func NewWithProtection(agents []model.Agent, fn protect.Protector, plans map[string]*engine.SessionPlan) *Model {
	m := New(agents)
	m.plans = plans
	m.ApplyActive(fn, time.Now())
	return m
}

// fallbackHeight is the rendered row budget when no window size is known. In
// inline mode a terminal height may not be delivered before the first paint,
// so lists must not render unbounded.
const fallbackHeight = 20

// viewportRows returns the number of list rows that fit in the current (or
// fallback) viewport, reserving a few rows for the header and hint lines.
func viewportRows(m *Model) int {
	h := m.height
	if h <= 0 {
		h = fallbackHeight
	}
	rows := max(h-5, 3) // title + data-root + hints + summary
	return rows
}

// windowFor returns the inclusive rows [lo, hi) of a list of count items whose
// cursor is kept visible but static when count fits the viewport. This is what
// makes long pickers scroll rather than overflow.
func windowFor(cursor, count, rows int) (lo, hi int) {
	if count <= rows {
		return 0, count
	}
	lo = max(cursor-(rows/3), 0) // keep the cursor a third from the top
	if lo+rows > count {
		lo = count - rows
	}
	return lo, lo + rows
}

// ApplyActive marks in place every session the protector reports as open and
// records the reasons for the dry-run. The protector is retained so the
// confirm step can re-validate against a fresh scan.
func (m *Model) ApplyActive(fn protect.Protector, now time.Time) {
	if fn == nil {
		return
	}
	m.protector = fn
	active := protect.Report{}
	for i := range m.agents {
		r := fn(m.agents[i], now)
		for j := range m.agents[i].Sessions {
			if _, ok := r[m.agents[i].Sessions[j].ID]; ok {
				m.agents[i].Sessions[j].Active = true
			}
		}
		maps.Copy(active, r)
	}
	m.active = active
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

type progressMsg float64

// sweepResult carries the engine outcome of a confirmed sweep.
type sweepResult struct {
	plan *engine.Plan
	res  *engine.Result
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case progressMsg:
		m.progress = float64(msg)
		if m.progress >= 1 {
			m.done = true
			return m, nil
		}
		return m, m.advanceProgress()
	case sweepResult:
		m.result = msg.res
		m.done = true
		m.screen = screenAfter
		return m, nil
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() string {
	switch m.screen {
	case screenAgent:
		return m.viewAgent()
	case screenMode:
		return m.viewMode()
	case screenDir:
		return m.viewDir()
	case screenBranch:
		return m.viewBranch()
	case screenAge:
		return m.viewAge()
	case screenDryRun:
		return m.viewDryRun()
	case screenConfirm:
		return m.viewConfirm()
	case screenProgress:
		return m.viewProgress()
	case screenAfter:
		return m.viewAfter()
	}
	return ""
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	switch m.screen {
	case screenAgent:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "enter":
			m.agentIdx = m.cursor
			m.cursor = 0
			m.screen = screenMode
		}
	case screenMode:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < 1 {
				m.cursor++
			}
		case "enter":
			m.mode = groupMode(m.cursor)
			m.selected = map[int]bool{}
			m.err = ""
			if m.mode == modeGit {
				m.branches = model.GroupByRepoBranch(onlyWithRepo(m.agents[m.agentIdx].Sessions))
				m.screen = screenBranch
			} else {
				m.groups = model.GroupByCWD(m.agents[m.agentIdx].Sessions)
				m.screen = screenDir
			}
		case "esc":
			m.cursor = 0
			m.screen = screenAgent
		}
	case screenDir:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.groups)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			if len(m.selected) == 0 {
				m.err = errStyle.Render("select at least one directory")
			} else {
				m.err = ""
				m.screen = screenAge
			}
		case "esc":
			// Return to the mode picker with the cursor restored to the
			// currently-selected mode, so backing out of a directory list
			// and choosing git mode is a clean pivot rather than a stuck
			// cursor (the dir cursor can exceed the mode picker's range).
			m.cursor = int(m.mode)
			m.screen = screenMode
		}
	case screenBranch:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			if len(m.selected) == 0 {
				m.err = errStyle.Render("select at least one branch")
			} else {
				m.err = ""
				m.screen = screenAge
			}
		case "esc":
			// Same pivot: restore the cursor to the current mode so the user
			// can flip from git mode to directory mode (or back) cleanly.
			m.cursor = int(m.mode)
			m.screen = screenMode
		}
	case screenAge:
		switch msg.String() {
		case "up", "k":
			if m.ageIdx > 0 {
				m.ageIdx--
			}
		case "down", "j":
			if m.ageIdx < len(model.Ages)-1 {
				m.ageIdx++
			}
		case "enter":
			m.computeMatches()
			if len(m.matches) == 0 {
				// Decision 10: with nothing to delete the confirm step is
				// nonsensical — surface why and stay on the picker.
				age := model.Ages[m.ageIdx]
				reason := "nothing matches this selection"
				if m.protected > 0 {
					reason = "every matching session is active or protected"
				}
				m.err = warnStyle.Render(reason + fmt.Sprintf(" — try a longer age than %s", age.Label))
				return m, nil
			}
			m.screen = screenDryRun
			m.offset = 0
		case "esc":
			if m.mode == modeGit {
				m.screen = screenBranch
			} else {
				m.screen = screenDir
			}
		}
	case screenDryRun:
		switch msg.String() {
		case "enter":
			m.screen = screenConfirm
		case "esc":
			m.screen = screenAge
		case "pgup", "up", "k":
			if m.offset > 0 {
				m.offset -= max(1, viewportRows(m)-4)
				if m.offset < 0 {
					m.offset = 0
				}
			}
		case "pgdown", "down", "j":
			// The window clamps itself on render; just nudge forward so the
			// hint reflects that more content is reachable.
			m.offset += max(1, viewportRows(m)-4)
		}
	case screenConfirm:
		switch msg.String() {
		case "y", "Y":
			agent := m.agents[m.agentIdx]
			if m.protector != nil {
				// Decision 10: re-validate protection against a fresh scan
				// immediately before deleting — a session opened since the
				// dry-run must never be swept.
				fresh := m.protector(agent, time.Now())
				kept := m.matches[:0]
				var reclaim int64
				for _, s := range m.matches {
					if _, active := fresh[s.ID]; active {
						continue
					}
					kept = append(kept, s)
					reclaim += engine.SessionReclaim(s)
				}
				m.matches = kept
				m.reclaimBytes = reclaim
				if len(m.matches) == 0 {
					m.err = warnStyle.Render("nothing to delete — every remaining session is now active")
					m.screen = screenDryRun
					return m, nil
				}
			}
			plan := m.buildPlan(agent)
			if plan == nil {
				m.err = errStyle.Render("no deletion plans are available for this run (demo/test data has none)")
				m.screen = screenDryRun
				return m, nil
			}
			m.screen = screenProgress
			m.progress = 0
			m.done = false
			return m, m.runSweep(plan)
		case "n", "N", "esc":
			m.screen = screenDryRun
		}
	case screenProgress:
		if m.done {
			m.screen = screenAfter
		}
	case screenAfter:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) advanceProgress() tea.Cmd {
	steps := 12
	next := m.progress + 1/float64(steps)
	if next > 1 {
		next = 1
	}
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return progressMsg(next)
	})
}

// buildPlan assembles the ordered deletion plan from the confirmed-still-safe
// matches, using the same plans that produced the dry-run footprint, so a
// confirmed sweep reclaims exactly what the dry-run reported. It returns nil
// when no plans are available (demo/test) or nothing matched.
func (m *Model) buildPlan(agent model.Agent) *engine.Plan {
	if len(m.plans) == 0 || len(m.matches) == 0 {
		return nil
	}
	plan := &engine.Plan{}
	for _, s := range m.matches {
		if p := m.plans[inventory.PlanKey(agent.Name, s.ID)]; p != nil {
			plan.Sessions = append(plan.Sessions, p)
		}
	}
	if len(plan.Sessions) == 0 {
		return nil
	}
	return plan
}

// runSweep executes the plan off the UI thread and reports the outcome.
func (m *Model) runSweep(plan *engine.Plan) tea.Cmd {
	return func() tea.Msg {
		res := engine.Execute(context.Background(), plan)
		return sweepResult{plan: plan, res: res}
	}
}

// computeMatches applies the age bucket and protection to the selected
// groups (directories or branches, per mode), producing the sessions a
// confirmed sweep would delete and counting the active ones spared.
func (m *Model) computeMatches() {
	age := model.Ages[m.ageIdx]
	var matches []model.Session
	var reclaim int64
	var protected int
	for idx := range m.selected {
		var sessions []model.Session
		if m.mode == modeGit {
			sessions = m.branches[idx].Sessions
		} else {
			sessions = m.groups[idx].Sessions
		}
		for _, s := range sessions {
			if _, active := m.active[s.ID]; active {
				protected++
				continue
			}
			if !age.Matches(s.LastActivity) {
				continue
			}
			matches = append(matches, s)
			reclaim += engine.SessionReclaim(s)
		}
	}
	m.matches = matches
	m.reclaimBytes = reclaim
	m.protected = protected
}

func (m *Model) viewAgent() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("agent-sweeper · pick an agent to sweep"))
	lo, hi := windowFor(m.cursor, len(m.agents), viewportRows(m))
	for i := lo; i < hi; i++ {
		a := m.agents[i]
		line := fmt.Sprintf("%-12s  %3d sessions  %10s", a.Name, a.SessionCount(), units.Bytes(a.Footprint()))
		if i == m.cursor {
			b.WriteString("\n" + selStyle.Render("▸ "+line))
		} else {
			b.WriteString("\n" + rowStyle.Render("  "+line))
		}
	}
	b.WriteString(scrollHint(lo, hi, len(m.agents)))
	b.WriteString("\n\n" + hintStyle.Render("↑/↓ move · enter select · q quit"))
	return b.String()
}

func (m *Model) viewMode() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("how to group sessions?"))
	for i, mode := range []struct {
		name string
		desc string
	}{
		{"directory", "group by working directory"},
		{"git repository", "group by repo and branch (multi-select branches)"},
	} {
		line := fmt.Sprintf("%-14s %s", mode.name, mode.desc)
		if i == m.cursor {
			b.WriteString("\n" + selStyle.Render("▸ "+line))
		} else {
			b.WriteString("\n" + rowStyle.Render("  "+line))
		}
	}
	b.WriteString("\n\n" + hintStyle.Render("↑/↓ move · enter select · esc back"))
	return b.String()
}

func (m *Model) viewDir() string {
	agent := m.agents[m.agentIdx]
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("sweep %s · choose directories", agent.Name)))
	b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("data root: %s", agent.DataRoot)))
	lo, hi := windowFor(m.cursor, len(m.groups), viewportRows(m))
	for i := lo; i < hi; i++ {
		g := m.groups[i]
		mark := " "
		if m.selected[i] {
			mark = "●"
		}
		label := g.Path
		if label == "" {
			label = "(no directory)"
		}
		line := fmt.Sprintf("[%s] %-58s %3d sessions %10s", mark, label, g.GroupCount(), units.Bytes(groupBytes(g)))
		if i == m.cursor {
			b.WriteString("\n" + selStyle.Render("▸ "+line))
		} else {
			b.WriteString("\n" + rowStyle.Render("  "+line))
		}
	}
	b.WriteString(scrollHint(lo, hi, len(m.groups)))
	if m.err != "" {
		b.WriteString("\n\n" + m.err)
	}
	b.WriteString("\n\n" + hintStyle.Render("↑/↓ move · space toggle · enter continue · esc back"))
	return b.String()
}

// scrollHint emits a trim indicator when the list is scrolled at the top,
// bottom, or mid-window so long pickers read as paged.
func scrollHint(lo, hi, count int) string {
	if count <= hi-lo {
		return ""
	}
	var parts []string
	if lo > 0 {
		parts = append(parts, "▲")
	}
	if hi < count {
		parts = append(parts, "▼")
	}
	return "\n" + dimStyle.Render("‥ "+strings.Join(parts, " ")+" scroll")
}

func (m *Model) viewBranch() string {
	agent := m.agents[m.agentIdx]
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("sweep %s · choose branches", agent.Name)))
	lo, hi := windowFor(m.cursor, len(m.branches), viewportRows(m))
	for i := lo; i < hi; i++ {
		bg := m.branches[i]
		mark := " "
		if m.selected[i] {
			mark = "●"
		}
		line := fmt.Sprintf("[%s] %-38s %-24s %3d sessions %10s", mark, shortRepo(bg.Repo), bg.Branch, bg.GroupCount(), units.Bytes(branchGroupBytes(bg)))
		if i == m.cursor {
			b.WriteString("\n" + selStyle.Render("▸ "+line))
		} else {
			b.WriteString("\n" + rowStyle.Render("  "+line))
		}
	}
	b.WriteString(scrollHint(lo, hi, len(m.branches)))
	if m.err != "" {
		b.WriteString("\n\n" + m.err)
	}
	b.WriteString("\n\n" + hintStyle.Render("↑/↓ move · space toggle · enter continue · esc back"))
	return b.String()
}

func (m *Model) viewAge() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sweep sessions older than"))
	for i, a := range model.Ages {
		desc := "everything (skip age filter)"
		if !a.All {
			desc = fmt.Sprintf("strictly older than %s", a.Label)
		}
		line := fmt.Sprintf("%-6s %s", a.Label, desc)
		if i == m.ageIdx {
			b.WriteString("\n" + selStyle.Render("▸ "+line))
		} else {
			b.WriteString("\n" + rowStyle.Render("  "+line))
		}
	}
	if m.err != "" {
		b.WriteString("\n\n" + m.err)
	}
	b.WriteString("\n\n" + hintStyle.Render("↑/↓ move · enter select · esc back"))
	return b.String()
}

func (m *Model) viewDryRun() string {
	agent := m.agents[m.agentIdx]
	age := model.Ages[m.ageIdx]
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("dry run · %s · older than %s", agent.Name, age.Label)))
	b.WriteString("\n" + dimStyle.Render("nothing is deleted until you confirm"))

	// Build the report into rows so a long session list pages instead of
	// overflowing the inline viewport.
	var rows []string
	for idx := range m.selected {
		label, sessions := groupLabel(m, idx)
		rows = append(rows, okStyle.Render("▣ "+label)+dimStyle.Render(fmt.Sprintf("  %d sessions", len(sessions))))
		for _, s := range sessions {
			reason, protected := m.active[s.ID]
			if protected {
				rows = append(rows, fmt.Sprintf("   %-42s %-10s %s", dimStyle.Render(s.Title), dimStyle.Render("protected"), dimStyle.Render(protect.ReasonText(reason))))
				continue
			}
			if !age.Matches(s.LastActivity) {
				continue
			}
			extra := ""
			if m.mode == modeGit && s.Branch != "" {
				extra = " " + dimStyle.Render(s.Branch)
			}
			rows = append(rows, fmt.Sprintf("   %-42s %-10s %9s%s", s.Title, s.ID, units.Bytes(engine.SessionReclaim(s)), extra))
		}
	}
	lo, hi := windowFor(m.offset, len(rows), viewportRows(m))
	for i := lo; i < hi; i++ {
		b.WriteString("\n" + rows[i])
	}
	b.WriteString(scrollHint(lo, hi, len(rows)))
	b.WriteString("\n\n" + warnStyle.Render(fmt.Sprintf(
		"would delete %d sessions, reclaiming %s", len(m.matches), units.Bytes(m.reclaimBytes))))
	if m.protected > 0 {
		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("%d active sessions are protected and never swept", m.protected)))
	}
	if m.err != "" {
		b.WriteString("\n\n" + m.err)
	} else {
		b.WriteString("\n\n" + hintStyle.Render("enter continue · esc back · ↑/↓ pgup/pgdn scroll"))
	}
	return b.String()
}

// groupLabel returns the header label and session set for the selected group
// index, honoring the active grouping mode.
func groupLabel(m *Model, idx int) (string, []model.Session) {
	if m.mode == modeGit {
		bg := m.branches[idx]
		return bg.Repo + " · " + bg.Branch, bg.Sessions
	}
	g := m.groups[idx]
	if g.Path == "" {
		return "(no directory)", g.Sessions
	}
	return g.Path, g.Sessions
}

func (m *Model) viewConfirm() string {
	agent := m.agents[m.agentIdx]
	var b strings.Builder
	b.WriteString(titleStyle.Render("confirm"))
	fmt.Fprintf(&b, "\nDelete %d sessions from %s, reclaiming %s?", len(m.matches), agent.Name, units.Bytes(m.reclaimBytes))
	b.WriteString("\n\n" + errStyle.Render("This cannot be undone. Active sessions are never touched."))
	b.WriteString("\n\n" + hintStyle.Render("y confirm · n cancel"))
	return b.String()
}

func (m *Model) viewProgress() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("deleting…"))
	bar := progressBar(m.progress, 40)
	b.WriteString("\n" + bar)
	if m.done {
		b.WriteString("\n\n" + okStyle.Render("done"))
	}
	return b.String()
}

func (m *Model) viewAfter() string {
	agent := m.agents[m.agentIdx]
	var b strings.Builder
	b.WriteString(titleStyle.Render("sweep complete"))
	if m.result != nil {
		fmt.Fprintf(&b, "\n%s", okStyle.Render(fmt.Sprintf(
			"deleted %d sessions, reclaimed %s", m.result.Deleted(), units.Bytes(m.result.BytesReclaimed()))))
		if n := m.result.Failed(); n > 0 {
			fmt.Fprintf(&b, "\n%s", warnStyle.Render(fmt.Sprintf("%d sessions failed and were left for a re-run", n)))
		}
		if m.result.NeedsVacuum() {
			fmt.Fprintf(&b, "\n%s", warnStyle.Render("store rows were deleted — the SQLite file only shrinks after a VACUUM"))
		}
	} else {
		fmt.Fprintf(&b, "\nReclaimed %s from %s across %d sessions.", units.Bytes(m.reclaimBytes), agent.Name, len(m.matches))
	}
	b.WriteString("\n\n" + hintStyle.Render("enter quit"))
	return b.String()
}

// err stores the transient picker error, surfaced in the dir view.

// onlyWithRepo keeps sessions whose repo identity resolved; git-repo mode
// (13) only offers sessions that landed in a repo, everything else is swept
// via directory mode.
func onlyWithRepo(sessions []model.Session) []model.Session {
	kept := sessions[:0]
	for _, s := range sessions {
		if s.Repo != "" {
			kept = append(kept, s)
		}
	}
	return kept
}

// groupBytes returns the reclaimable footprint of a directory group.
func groupBytes(g model.Group) int64 {
	var total int64
	for _, s := range g.Sessions {
		total += engine.SessionReclaim(s)
	}
	return total
}

func branchGroupBytes(bg model.BranchGroup) int64 {
	var total int64
	for _, s := range bg.Sessions {
		total += engine.SessionReclaim(s)
	}
	return total
}

func shortRepo(repo string) string {
	if repo == "" {
		return "(no repo)"
	}
	// Trim a scheme prefix (e.g. https://github.com/x/y) down to owner/name.
	for _, scheme := range []string{"https://", "http://", "ssh://"} {
		if rest, ok := strings.CutPrefix(repo, scheme); ok {
			return rest
		}
	}
	return repo
}

func progressBar(p float64, width int) string {
	filled := min(int(p*float64(width)), width)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("  %s %3.0f%%", bar, p*100)
}
