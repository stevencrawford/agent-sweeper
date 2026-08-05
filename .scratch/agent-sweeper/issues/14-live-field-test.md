# Ticket: Live field-test against real agent sessions

Type: task
Status: resolved
Blocked by:
Claimed by: wayfinder session (working 14)

## Question

Map-notes ended with the map done and the code shipped at v0.1.0, but nobody has shown the CLI actually working against **live** agent session stores on a real machine — the fog has always been factory fixtures and mocks. Does `agent-sweeper` really work when pointed at the sessions an agent actually left on this host?

Drive the real binary against the real stores without deleting anything, the safest exercise first seen correct:

- **Detection against reality**: do OpenCode / Copilot / Claude / Codex / Pi / Cursor stores resolve to real, populated paths on this macOS host, and does the inventory + grouping produce the sessions you'd expect (vs. a hand audit of the stores themselves)?
- **`stats` sanity**: does the per-agent footprint table agree with a manual measure of the store sizes under the shared/`snapshot/` + DB-row architecture decisions come to (manage::plan-based footprint, unshared stores excluded)?
- **Active-session protection honesty**, and confirm that `sweep` dry-run lists the genuinely-protected sessions and dims them with a reason (fully scanned include a real currently-open agent).
- **Report** the field findings: which agents were actually present, which resolved empty, any crashes, any discrepancies (detection says N, reality says N+1), and anything that needs a follow-up ticket.

Command only the SAFE surface: `agent-sweeper stats` and the sweep **dry-run** (lists before-footprint, deletes nothing). Do **not** execute a real confirm+delete against the user's live sessions in this session — that needs a human behind at the final `confirm`.

## Answer

**No. The application does not work against live agent sessions — it is mock-driven end to end.** This session's field-test failed at the gate: there is no real-store inventory adapter anywhere, so there is nothing to field-test yet. Findings:

**1. Every command reads `mock.Agents()`, never a real store.** `cmd/sweep.go` feeds the TUI `mock.Agents()`; `cmd/stats.go` computes `stats.Compute(mock.Agents())`. `mock.go` is an explicitly synthetic hardcoded dataset (5 OpenCode / 4 Copilot / 4 Claude / 3 Codex / 2 Pi / 3 Cursor sessions, all fabricated paths and IDs).

**2. No inventory adapter exists.** Grep of the entire Go tree finds no code that reads opencode.db, Copilot session-store.db, Cursor state.vscdb, or the claude/codex/pi JSONL transcripts to produce `[]model.Session`. The only real store-touching code is: the deletion engine (`internal/engine`, `modernc.org/sqlite`) — invoked **only from tests**, never from a command — and the process/marker scanners in `internal/protect` (argv parsing, `ps` snapshot, Copilot lock, Claude roster, Cursor focus) which run against **mock session IDs** and thus cannot match the user's real sessions.

**3. Live-vs-mock numbers (read-only audit, this host):**

| agent | mock claims | real store |
|---|---|---|
| OpenCode | 5 sessions | 1,241 rows in `~/.local/share/opencode/opencode.db` |
| Claude Code | 4 | 1 project dir under `~/.claude/projects` (sessions = jsonl files inside) |
| Codex | 3 | 2,026 entries under `~/.codex/sessions` |
| Pi | 2 | 4 entries under `~/.pi/agent/sessions` |
| Cursor | 3 | store present but not enumerated |

All six real stores are present on this host; `go run . stats` prints exactly the mock table (21 sessions, 280.8MiB).

**What this means for the map.** The destination ("detects the six agents' local session stores") is **not reached**. Tickets 01–06 researched the real shapes and paths; 07–13 built genuine, tested machinery — but the seam between research and machinery was never bridged. The single missing layer is a **real session-store inventory adapter** that enumerates each agent's actual store into `[]model.Session`. Until it exists, the TUI, protection, and engine all operate on fabrications, and the dry-run==confirm invariant is only proven against fixtures.

No destructive sweep was run; `go run . stats` is the only binary surface exercised. The safe conclusion above required zero deletion.

Follow-up ticket: the real inventory adapter (created as 15).