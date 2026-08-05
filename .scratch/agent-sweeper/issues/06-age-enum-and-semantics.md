# Ticket: Age enum and matching semantics

Type: grilling
Status: resolved
Blocked by:
Claimed by: wayfinder session (working 06)

## Question

The age picker offers an enum (24h / 1d / 3d / 7d / 1mo / …). Which values ship? Does the enum allow a custom value, and what exactly is compared — the session's **last-activity timestamp** (read per store: last JSONL line, DB `updated_at`, etc.) with which boundary (older than X = match)?

Decide the enum, the last-activity read per store, and the boundary semantics. Grill with a human.

## Answer

Decided by grilling with the human.

- **Enum**: `1d / 3d / 7d / 30d / 90d / 1y / all`. No `24h` bucket (redundant with the 24h active-session grace window, which protects anything younger than that anyway — the 1d floor is the first bucket that is actually sweepable). No custom/free-form value — enum only, so no duration parser or validation.
- **Boundary semantics**: strictly **older than X** — a session matches when `lastActivity < now - X`; a session exactly X old does NOT match. `all` bypasses the age comparison entirely (still subject to active-session protection).
- **Last-activity read per store**:
  - **OpenCode**: `MAX(session.time_updated, MAX(message.time_created))` from `opencode.db` (mirrors omnivue's `internal/ingest/opencode/sessions.go:73`).
  - **Copilot**: `sessions.updated_at` from `session-store.db`, bumped to `events.jsonl` mtime when newer (mirrors omnivue `internal/ingest/copilot/sessions.go:66`).
  - **Claude Code**: transcript JSONL mtime (`~/.claude/projects/<path>/<id>.jsonl`).
  - **Codex**: `state_5.sqlite` `threads.updated_at`, else rollout JSONL mtime.
  - **Pi**: JSONL file mtime (header carries only session *start*, so mtime is the read).
  - **Cursor**: `state.vscdb` `composerData.lastUpdatedAt`, else transcript-dir mtime.
- **Unreadable timestamp**: fall back to the session's **created** timestamp; if neither is readable, treat as oldest (matches every bucket, shown with a `?` age in the dry-run).

Feeds: the age picker screen in **TUI flow and screens** (07), and the sweep match predicate in **Deletion semantics design** (08).
