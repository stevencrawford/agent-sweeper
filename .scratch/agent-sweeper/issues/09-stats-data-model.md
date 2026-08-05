# Ticket: stats command data model

Type: grilling
Status: resolved
Blocked by: 01
Claimed by: wayfinder session (working 09)

## Question

Given the artifact inventory (01), what does `stats` report?

- Per-agent totals (session count, bytes) before any interaction
- Drill-down into a selected agent's per-directory / per-session footprint
- Whether `stats` and the `sweep` dry-run share one footprint computation (they should)

Decide the data model and the output shape (table, size formatting). Grill with a human.

## Answer

Resolved by a grilling with the human (seven decisions) plus a working, wired `stats` command. All build checks clean (go build, gostyle vet, golangci-lint, go test).

**Decisions (all accepted by the human):**

1. **Plan-based footprint everywhere.** A session's reported size is the sum of its deletion plan's Remove\* action bytes (`engine.SessionReclaim`), never the detection-time DB size. DB rows and shared stores count 0. `stats` and the sweep dry-run use the *same* function, so they can never disagree.
2. **`stats` is a one-shot static table** that prints and exits — no TUI drill-down. Shell-pipeable.
3. **4 columns:** `AGENT | SESSIONS | RECLAIMABLE | STORE-ROW`. STORE-ROW counts sessions whose deletion removes SQLite/KV rows — the DB-file bulk that reclaim doesn't cover until VACUUM (the `NeedsVacuum` hint from 08).
4. **`units.Bytes` human formatting** (shared with the TUI — TUI's local `humanBytes` removed) **+ a grand-total row** (`TOTAL`).
5. **One shared plan-builder seam:** `engine.SessionReclaim(s)` / `engine.SessionTouchesStore(s)` on a new `model.Session.ReclaimBytes` / `TouchesStore`. Real detection (feeds 10/12) builds the per-agent plan and sums its remove actions; `model.Session.SizeBytes` is kept only as the raw detection-time number and is never displayed.
6. **Working on mock now:** `agent-sweeper stats` runs against `mock.Agents()`; mock now carries `ReclaimBytes` (a per-agent ratio of `SizeBytes` — DB-backed agents reclaim less than they measure) and `TouchesStore` per agent. Real detection swaps in later without touching the command.
7. **Cobra surface:** new `cmd/` package — root command with `sweep` and `stats` subcommands (`main.go` delegates to `cmd.Execute()`). Mirrors omnivue; version var ready for 11's ldflags.

**Implementations:** `internal/stats/` (Row, Summary, Compute, Render — tabwriter table), `internal/units/` (Bytes), `internal/engine/reclaim.go`, `cmd/{root,sweep,stats}.go`. The sweep TUI dry-run, group views, and agent picker all route through `engine.SessionReclaim` / `units.Bytes` now. Tests: stats_test (totals + render), units/bytes_test, engine seam test. Sample output:

```
AGENT        SESSIONS  RECLAIMABLE  STORE-ROW
OpenCode     5         138.0MiB     5
Copilot      4         103.5MiB     4
Claude Code  4         19.0MiB      0
Codex        3         8.8MiB       3
Pi           2         2.0MiB       0
Cursor       3         9.5MiB       3
TOTAL        21        280.8MiB     15
```
