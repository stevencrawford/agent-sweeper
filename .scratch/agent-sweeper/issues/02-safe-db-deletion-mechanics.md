# Ticket: Safe DB-row deletion mechanics per agent

Type: research
Status: resolved
Blocked by:

## Question

For the SQLite-backed stores — OpenCode's `opencode.db`, Copilot's `session-store.db`, Cursor's `state.vscdb` — what is the exact schema/table relationship needed to delete **one session and all of its rows safely**?

- Which tables hold session data, and how are they keyed/linked (foreign keys, session_id columns)?
- What breaks if a row is orphaned?
- WAL journaling, database locks, and behavior **while the agent is running** (a live agent holds the DB open; deletions may conflict).
- Any cross-store linkage (e.g. OpenCode `snapshot/` git repos referencing DB sessions; Copilot `session-state/` dirs referencing DB rows).

This feeds the deletion-semantics design and the active-session protection wiring.

**Research** — resolve with a `/research` subagent. Write the findings as a facts doc under `research/db-deletion.md` and link it here.

## Answer

Resolved. Full facts doc: [`research/db-deletion.md`](../research/db-deletion.md). Highlights:

- **OpenCode opencode.db**: delete `session` rows with `PRAGMA foreign_keys=ON` in one `BEGIN IMMEDIATE` txn; children cascade (`message`, `part`, `todo`, `session_message`, `session_input`, `event`); recurse `parent_id` for child sessions; never touch `project`/`permission`; snapshot git repos are project-scoped — orphaned objects cleaned via `git gc --prune=7.days`, not deleted.
- **Copilot session-store.db**: delete child rows (turns/checkpoints/session_files/...) then `sessions`, then `rm -rf session-state/<uuid>/`; skip live sessions (inuse.<pid>.lock); orphaned row/dir mismatch produces ghosts.
- **Cursor state.vscdb**: KV-only, no FKs. Delete global keys `composerData:<id>`, `bubbleId:<id>:*`, `checkpointId:<id>:*`, etc., **and strip the id from the workspace `composer.composerData` JSON** or a sidebar ghost remains; never touch shared `composer.content.*`.
- **Cursor ai-code-tracking.db**: `conversation_summaries`/`ai_code_hashes`/`tracked_file_content` keyed by conversationId — safe per-id delete (attribution loss only).
- **Codex**: file-based, not SQLite-for-sessions. Best: `codex delete <SESSION>`; manual = delete rollout(s) + index lines + shell snapshot.
- Cross-cutting: all DBs are WAL; single-writer → `busy_timeout` + one txn per session + retry on `SQLITE_BUSY`; skip locked/active stores; back up `-wal`/`-shm` before writing.
