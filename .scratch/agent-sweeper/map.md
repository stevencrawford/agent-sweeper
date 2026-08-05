# Map: agent-sweeper

## Destination

A working, distributed Go CLI — `agent-sweeper` — that interactively detects the six major agents' local session stores (OpenCode, Copilot, Claude Code, Codex, Pi, Cursor), reports their filesystem footprint, and cleans stale sessions: pick an agent, filter its sessions by directory/repo, pick an age (24h/1d/3d/7d/1mo…), review the before-footprint, confirm once, delete, then see the after-footprint. Actively-open sessions are never touched, and nothing is deleted without review. Ships as a GitHub Release + Homebrew binary for macOS, Linux, and Windows.

## Notes

- **Effort carries execution in-place**: tickets resolve with working code for their slice, and the map is done when the CLI works — not when a spec is written. (Override of wayfinder's plan-don't-do default.)
- Domain: Go 1.26; Cobra command surface (`sweep`, `stats`); bubbletea TUI; SQLite row deletion in agent DBs (opencode.db, session-store.db, state.vscdb); JSONL session files; side-artifact cleanup; XDG / %APPDATA% path resolution.
- Reference: `../omnivue` — AGENTS.md, `docs/ADAPTERS.md`, `internal/ingest/*` hold the detection paths and session formats. **Reimplement from reference; do not import omnivue packages.**
- Follow omnivue's Go conventions (golangci-lint + gostyle) and its pre-commit checks (`go build`, `go vet -vettool=$(which gostyle)`, `go test ./...`).
- Skills every session should consult: `grill-me` (grilling), `grill-with-docs` when docs should be produced.
- Tracker: local-markdown. Map is this file; tickets live in `issues/NN-<slug>.md`; research findings live in `research/`.
- Work tickets in number order; the frontier (open, unblocked, unclaimed) naturally sequences the work.

## Decisions so far

<!-- the index — one line per closed ticket: enough to judge relevance, then zoom the link for the detail the ticket holds -->

- [Session-artifact inventory per agent](issues/01-session-artifact-inventory.md) — per-agent map of per-session vs shared artifacts; OpenCode snapshots and Cursor content-addressed stores are shared and must be treated specially. Full doc: `research/artifact-inventory.md`.
- [Safe DB-row deletion mechanics per agent](issues/02-safe-db-deletion-mechanics.md) — delete child-then-parent in one txn with foreign_keys=ON (opencode); Copilot rows + session-state dir; Cursor KV rows + workspace JSON strip; Codex is file-based (`codex delete`). Full doc: `research/db-deletion.md`.
- [Active-session detection per agent](issues/03-active-session-detection.md) — Copilot `inuse.<pid>.lock`, Claude daemon roster, Cursor focus ids are the durable markers; argv parsing gives exact ids; Pi has no marker (mtime only); fail-safe = protect most-recent. Full doc: `research/active-session-detection.md`.
- [Cross-platform session-store paths](issues/04-cross-platform-paths.md) — OpenCode is pure XDG on every OS; Copilot/Claude/Codex/Pi are home-dotdirs with env overrides; Cursor splits app-data (DBs) from `~/.cursor/projects` (transcripts). Full doc: `research/platform-paths.md`.
- [Session grouping key — directory vs git-repo](issues/05-session-grouping-key.md) — group by cwd only (no git): canonical path is the grouping key (symlink-resolved, Abs+Clean fallback), original spelling is the label; monorepo subdirs split into rows (try-and-see); no-cwd sessions land in an anonymous `(no directory)` bucket.
- [Age enum and matching semantics](issues/06-age-enum-and-semantics.md) — enum `1d/3d/7d/30d/90d/1y/all`, no custom value; strictly older-than-X match; per-store last-activity reads (opencode MAX time_updated/time_created, copilot sessions.updated_at bumped by events mtime, mtime for claude/codex/pi, cursor lastUpdatedAt); unreadable → fall back to created, else treat as oldest.
- [TUI flow and screens](issues/07-tui-flow-and-screens.md) — 7-screen flow (agent → grouping-mode → dir/branch picker → age → dry-run → confirm → progress → after) as a runnable bubbletea stub (`go run .`); dry-run lists only deletable rows; human review added a git-repo grouping mode with branch multi-select (feeds 13). Prototype: `main.go`, `internal/{model,mock,tui}/`.
- [Deletion semantics design](issues/08-deletion-semantics-design.md) — engine executes per-session plans: files-first-record-last (stop on first error → interrupted sweeps leave self-healing ghosts), continue-and-report, dry-run=plan (reclaim invariant), no auto-VACUUM (NeedsVacuum hint), shared stores (snapshot/, composer.content.*) excluded. One BEGIN IMMEDIATE txn per session-store with FK=ON + busy retry; all actions idempotent. Prototype: `internal/engine/` (modernc.org/sqlite), per-agent ordered plan table in the ticket.
- [stats command data model](issues/09-stats-data-model.md) — `agent-sweeper stats` is a one-shot Cobra table `AGENT | SESSIONS | RECLAIMABLE | STORE-ROW` + TOTAL; footprint is plan-based (`engine.SessionReclaim` = Remove\* bytes, DB bulk counts 0) shared with the sweep dry-run via the same seam; `model.Session.ReclaimBytes`/`TouchesStore` distinct from raw `SizeBytes`; `cmd/` package with `sweep`/`stats` subcommands (Cobra, mirrors omnivue). Code: `internal/stats/`, `internal/units/`, `internal/engine/reclaim.go`.
- [Active-session protection wiring](issues/10-active-session-protection-wiring.md) — real detector (`internal/protect/`): `ps` scan + per-agent resume-argv parsing + durable markers (Copilot `inuse.*.lock` w/ PID validation, Claude roster/jobs, Cursor focus gated on a running process); 24h grace window is a hard protection rule; computed at load, re-validated at confirm with a fresh scan; dry-run lists protected rows dimmed with reason; 0 matches blocks the confirm. Code: `internal/protect/`, `internal/tui/`, `cmd/sweep.go`.
- [Release and CI pipeline](issues/11-release-and-ci-pipeline.md) — shipped v0.1.0; tagpr + goreleaser mirrors omnivue; repo `stevencrawford/agent-sweeper` (public) + tap `stevencrawford/homebrew-tap` (public); CI on ubuntu/macos; `brew install stevencrawford/tap/agent-sweeper` verified end-to-end. Facts: repo/tap/release URLs in the ticket.
- [Test and fixture strategy for destructive operations](issues/12-test-and-fixture-strategy.md) — shared helpers, inline fixtures, no testdata/; scheme-minimal store shapes; never-use-real-store guard = single-root seam (`testutil.StoreRoot` + `UnderRoot`, paths escaping the fixture root fail loudly); no coverage threshold (CI = lint + gostyle + test); dry-run==confirm invariant stays on the existing engine test. Code: `internal/testutil/`.
- [Git-repo grouping semantics](issues/13-git-repo-grouping-semantics.md) — repo identity = native field else cwd→.git (best-effort); branch = session-time recorded label only (Copilot `sessions.branch`; merged-branch label IS the cleanup signal, no read-time HEAD); flat repo·branch picker, **no-repo sessions excluded from git mode** (directory mode only); git mode is per-sweep, stats unaffected. Also fixed esc-back pivot: dir/branch pickers restore the mode cursor so git↔directory flips cleanly. Code: `internal/tui/`.
- [Live field-test against real agent sessions](issues/14-live-field-test.md) — **the destination is not reached**: every command feeds `mock.Agents()`; no inventory adapter reads a real store; `engine.Execute` is never invoked outside tests; protect scans mock IDs. Real-vs-mock on this host: OpenCode 1,241 db rows vs mock 5; Codex 2,026 sessions vs 3; Pi 4 vs 2. The missing layer is the real session-store inventory adapter (ticket 15, superseded by 16).
- [Real session-store inventory + scriptable sweep](issues/16-real-session-inventory-and-scriptable-sweep.md) — **open.** The spec that reaches the destination: a real `inventory` seam (per-agent read-only store readers → `[]model.Session`, registry-of-adapters mirroring omnivue without importing it), plan-based footprint replacing mock ratios, wiring `sweep`/`stats` to real data (mock → fixture/`--demo`), plus a non-interactive scriptable path (`--agent/--mode/--repo/--dir/--age/--yes`, `--json`/`--quiet`) that keeps every protection invariant. Acceptance = the ticket-14 field hand-audit. Rolls up ticket 15 and the scriptable-mode want.

## Not yet specified

- Scriptable/non-interactive mode (`--agent`, `--repo`, `--age`, `--yes`) — a likely later want; the flow is interactive-only for now.
- Config file to pin/exclude specific sessions from ever being swept (declined as a safety mechanism, may return as a power-user feature).
- Windows installer / winget / scoop.
- Shared-store reclaim: OpenCode project-wide `git gc --prune=7.days` on `snapshot/<project-id>/` after a project's last session is deleted, and Cursor content-addressed `composer.content.*` reference-counting — both excluded from per-session plans by 08; gated deep-reclaim decision deferred until the sweep is wired to real detection.

## Out of scope

- Session viewing / resume / search / UI server — omnivue's domain; agent-sweeper is a cleanup CLI. Closed ticket: none yet.
- Trash / recycle-bin restore — deletion is permanent by design; dry-run + confirm is the safety model.
- Cloud sync or any network upload of session data.
