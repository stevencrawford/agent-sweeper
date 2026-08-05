# Ticket: Active-session detection per agent

Type: research
Status: resolved
Blocked by:

## Question

How can agent-sweeper know which sessions each agent **currently has open**, so they can be protected from deletion? Per agent, identify a concrete detection recipe:

- Process detection (running binary + argv/pid)
- Agent state files (e.g. OpenCode's active-session record, Claude Code's running-project file)
- DB or lock markers written while a session is live
- What to do when detection is ambiguous (fail-safe = protect, or skip protection?)

Agents: OpenCode, Copilot, Claude Code, Codex, Pi, Cursor. This feeds the active-session protection wiring ticket.

**Research** — resolve with a `/research` subagent. Write the findings as a facts doc under `research/active-session-detection.md` and link it here.

## Answer

Resolved. Full facts doc: [`research/active-session-detection.md`](../research/active-session-detection.md). Highlights:

- **OpenCode**: process `opencode`; exact id from argv `-s/--session` (High); `-c/--continue` resolves to newest session for cwd (Medium); no durable marker.
- **Claude Code**: process `claude`; exact id from argv `-r/--resume` (High); background sessions robustly via `~/.claude/daemon/roster.json` + `jobs/*/state.json` or `claude agents --json` (High); interactive sessions only a `<5min` JSONL mtime heuristic (Medium).
- **Codex**: process `codex`; exact id from argv `resume <uuid>` (High); `resume --last` → newest thread in state sqlite (Medium).
- **Copilot**: process `copilot`/`github-copilot`; exact id from argv `--resume=<uuid>` (High); **best lock marker of all six** — `session-state/<uuid>/inuse.<pid>.lock` (validate PID alive, stale locks accumulate).
- **Pi**: process `pi`; exact id from argv `--session` (High); **no marker at all** — liveness only via file mtime (Medium).
- **Cursor**: GUI process; weak argv; use state DBs — workspace `composer.composerData` `selectedComposerIds`/`lastFocusedComposerIds` and global `composerData:<id>` status (Medium).
- Fail-safe policy (protect rather than delete): argv-declared ids always protect; durable markers protect; `-c`/`--continue`/`--last`/bare launches protect most-recent per agent+dir; recency grace window; dry-run + confirm before any delete.
