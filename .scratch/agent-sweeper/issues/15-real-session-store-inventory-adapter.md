# Ticket: Real session-store inventory adapter

Type: prototype
Status: superseded
Blocked by:
Superseded by: 16 (this scope is folded into the real-inventory + scriptable-sweep spec)
Claimed by:

## Question

Ticket 14 proved the CLI is mock-driven end to end: `sweep` and `stats` consume `mock.Agents()`, and no code reads a real session store. The researched shape (tickets 01–06, `research/*.md`) was never bridged into code. Design and build the **real inventory adapter** — the layer that enumerates each of the six agents' actual session stores into `[]model.Session` — and swap it in as the source for `sweep` and `stats` (mock becomes a fixture/fallback only).

Decide and prove:

- **Adapter seam**: one `inventory.Agent(name)`-style interface? Per-agent sub-adapters behind a registry (mirrors omnivue's adapter layout — do NOT import omnivue)? What does `model.Agent` + `model.Session` need that mock currently fabricates (real store path, cwd, repo, branch, last-activity, size)?
- **Per-store readers** (read-only): opencode.db session rows + `~/.local/share/opencode` snapshot dirs; Copilot session-store.db + session-state dir; Claude `~/.claude/projects/**/*.jsonl`; Codex `~/.codex/sessions` + shell_snapshots + index; Pi `~/.pi/agent/sessions`; Cursor state.vscdb + `~/.cursor/projects`. Reuse the safe read-only patterns from 01–06; never-mutate.
- **Size and reclaim semantics**: `SizeBytes` from the store scan; `ReclaimBytes` from engine.SessionReclaim / plan-shaped reads — the mock's per-agent ratios must be replaced by real plan accounting (a session plan's actual Remove* byte sum), matching 08/09's plan-based footprint.
- **Wiring**: replace `mock.Agents()` in both commands; keep the dry-run==confirm invariant (12); protect (10) must then match real IDs, not mock ones.
- **Verification**: this is the ticket 14 field-test payoff — after wiring, `agent-sweeper stats` must roughly agree with a hand audit of the six real stores on this host (1,241 OpenCode rows etc.), still without deleting anything.

Build it with working code (effort override: tickets resolve with code). Prototype the seam on ONE agent (OpenCode, the richest store) first, human-review the model seam, then fan out to the other five.
