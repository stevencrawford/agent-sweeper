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

## Not yet specified

- Scriptable/non-interactive mode (`--agent`, `--repo`, `--age`, `--yes`) — a likely later want; the flow is interactive-only for now.
- Config file to pin/exclude specific sessions from ever being swept (declined as a safety mechanism, may return as a power-user feature).
- Homebrew formula and tap details (tangled up in the release ticket).
- Windows installer / winget / scoop.
- Behavior when every matching session is protected or active.

## Out of scope

- Session viewing / resume / search / UI server — omnivue's domain; agent-sweeper is a cleanup CLI. Closed ticket: none yet.
- Trash / recycle-bin restore — deletion is permanent by design; dry-run + confirm is the safety model.
- Cloud sync or any network upload of session data.
