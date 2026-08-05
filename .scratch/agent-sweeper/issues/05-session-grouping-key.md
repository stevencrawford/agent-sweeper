# Ticket: Session grouping key — directory vs git-repo

Type: grilling
Status: resolved
Blocked by:

## Question

Sessions under an agent group by the directory they ran in. Since an agent can run **without a backing git repo**, is the grouping key the raw cwd path, a git-remote identity, or both (cwd by default, git identity when present)? What happens to sessions with no usable cwd (missing/empty)?

Decide the grouping model that both the `sweep` directory picker and `stats` drill-down use. Grill with a human who owns real session data; a prototype of the group-tree shape is welcome.

## Answer

Decided by grilling with the human. **Group by the session's cwd path** — canonicalized for grouping, original spelling for display.

- **Grouping key**: canonical filesystem path derived from the stored cwd — symlinks resolved when the path exists, `Abs`+`Clean` fallback when it doesn't. The literal stored string is the row label.
- **Git identity is out**: no repo derivation, no git lookups. cwd is the sole grouping dimension; monorepo subdirectory runs are separate groups.
- **Picker shape** (feeds 07): multi-select list of distinct groups, each row showing its session count; after selecting groups the user moves to the age filter (which includes "all").
- **Monorepo fragmentation accepted for now**: two runs in subdirectories of the same repo → two rows. Try-and-see — revisit if the TUI prototype (07) shows the fragmented list is annoying.
- **No-cwd sessions** (empty or missing cwd): single catch-all row `(no directory)` — anonymous (count only, no origin shown), selectable and sweepable like any other row.
- Row ordering and path rendering are TUI-prototype scope (07); only the count rides on the row here.

## Amendment (resolved 07)

Ticket 07's human review added a **git-repo grouping mode**: an extra mode picker after the agent picker offering directory (this decision) OR git-repo grouping with branch multi-select. That reopens the "Git identity is out" line above for git mode only. Directory mode keeps this decision intact; git-repo mode's semantics (repo identity derivation, branch capture, dry-run/flag behavior) are decided in **13 git-repo grouping semantics**.
