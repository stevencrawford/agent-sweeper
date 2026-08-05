# Ticket: Session-artifact inventory per agent

Type: research
Status: resolved
Blocked by:

## Question

For each of the six agents (OpenCode, Copilot, Claude Code, Codex, Pi, Cursor), enumerate **every filesystem artifact that belongs to a single session** and must be deleted along with it — the primary record plus all side files:

- Claude Code: main project JSONL, subagent transcripts
- Copilot: `session-store.db` rows + `session-state/<uuid>/` events.jsonl, checkpoints, rewind-snapshots
- OpenCode: `opencode.db` rows (session/parts/todos/etc.) + `snapshot/` git repos
- Cursor: `state.vscdb` rows + `projects/<uuid>/` transcripts + `ai-code-tracking.db` rows
- Codex: session JSONL + any logs/history
- Pi: session JSONL

Include typical byte sizes per artifact so a session's true footprint is measurable, and note anything that is **not** per-session (shared caches, config) that must be left alone. This feeds the footprint reporting and the deletion-semantics design.

**Research** — resolve with a `/research` subagent. Write the findings as a facts doc under `research/artifact-inventory.md` and link it here.

## Answer

Resolved. Full facts doc: [`research/artifact-inventory.md`](../research/artifact-inventory.md). Highlights:

- **OpenCode**: single `opencode.db` holds all sessions (cascaded rows in session/message/part/todo/task/...); `snapshot/<project-id>/` git repos are **per-project, shared across a project's sessions** — not deletable per-session (only `git gc`); `storage/` legacy dirs are per-session.
- **Copilot**: `session-store.db` rows + per-session `session-state/<uuid>/` dir (events.jsonl 2.2–2.4 MB, checkpoints/, rewind-snapshots/ raw file backups).
- **Claude Code**: `projects/<dir>/<uuid>.jsonl` + subagents/, plans/, todos/, shell-snapshots/ per session; shared `sessions-index.json` and `history.jsonl` must be *edited* (entry removed), never deleted.
- **Codex**: `sessions/YYYY/MM/DD/rollout-*.jsonl` + `session_index.jsonl` line + `shell_snapshots/`; per-session rows in shared sqlite index DBs.
- **Pi**: one JSONL per session under `sessions/<cwd-slug>/`, no side artifacts.
- **Cursor**: `state.vscdb` KV rows (composerData:, bubbleId:, checkpointId:, codeBlockDiff:) + `projects/.../agent-transcripts/<uuid>/` dirs; **trap** — `composer.content.*` and `ai-code-tracking.db` content tables are content-addressed/shared, need reference-counting before deletion.
- `## Not per-session (leave alone)` section lists shared caches/config/indexes per agent.
