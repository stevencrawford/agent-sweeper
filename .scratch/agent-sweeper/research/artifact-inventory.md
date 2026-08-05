# Agent Session Artifact Inventory

Research facts for building **agent-sweeper**, a cleanup tool for AI coding-agent session files.
Goal: enumerate EVERY filesystem artifact that belongs to a SINGLE session and must be deleted
along with it, plus the shared/global stores that MUST be left alone.

Primary sources: the omnivue repo (`internal/ingest/` adapters, `AGENTS.md`, `docs/ADAPTERS.md`)
and direct inspection of the author's live agent data dirs. Web sources cited inline.

> **All byte sizes are empirical estimates** from one developer machine (macOS) unless a web
> source is cited. Treat them as order-of-magnitude guidance, not guarantees.

---

## OpenCode

Data root: `$XDG_DATA_HOME/opencode/` → `~/.local/share/opencode/` (macOS/Linux).

### Primary session record

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.local/share/opencode/opencode.db` (SQLite) | Partially — rows per session | **Huge.** 2.8 GB on author machine (2,798,661,632 B); grows without bound with usage | One DB holds ALL sessions. Delete per-session rows from `session`, `message`, `part`, `todo`, `task`, `session_input`, `session_message`, `session_context_epoch` (FKs on `session.id` / `message.id` are `ON DELETE CASCADE` from `session`, so deleting the `session` row cascades). `project` rows are shared per repo — keep unless last session for the project. DB never shrinks until `VACUUM`. |

### Per-session side artifacts

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.local/share/opencode/storage/message/<session-id>/` (legacy JSON message dirs) | Per-session dirs | ~327 MB total under `storage/` on author machine; `part/` 173 MB, `message/` 53 MB, `session_diff/` 101 MB | Legacy/parallel file store keyed by session/message id. Delete the per-session dirs (`storage/message/<session-id>/`, `storage/part/<msg-id>/`, `storage/session_diff/<session-id>/`). `storage/project/` is shared. |
| `~/.local/share/opencode/snapshot/<project-id>/<sha1(worktree)>/` (bare git repos) | **Per-project, NOT per-session** — one repo per project; all sessions of that project write into it | 167 MB total author machine: `global/` 91 MB, largest project 51 MB, most 1–9 MB. Web: a misconfigured `snapshot/global/` reached **424 GB** (bradystroud.dev) | Shared across sessions of the same project. Deleting ONE session cannot safely delete the whole repo (undo/redo history of other sessions lives there). `git gc --prune=now` inside the repo can reclaim space after the DB rows are gone. `snapshot/global/` is the fallback repo for non-git dirs. |
| `~/.local/share/opencode/tool-output/tool_<id>` | Per tool call (~per session) | 1.4 MB total (author) | Tool-output cache keyed by tool-call id. Safe to remove with the session; deleting is cosmetic. |

### NOT per-session (leave alone)

- `opencode.db` global/shared tables: `project`, `workspace`, `project_directory`, `account`, `credential`, `control_account`, `permission`, `event` / `event_sequence` (aggregate event log, not session-scoped), `__drizzle_migrations`.
- `~/.local/share/opencode/auth.json` (credentials), `bin/` (installed runtime), `log/` (64 MB, global logs), `storage/project/`, `repos/`, `PEan`.
- `~/.cache/opencode/` plugin cache (opencode upkeep skill; separate from data root).
- Snapshot repos for OTHER projects; `snapshot/<project-id>/` for a project that still has live sessions.

---

## GitHub Copilot

Data root: `~/.copilot/` (macOS/Linux).

### Primary session record

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.copilot/session-store.db` (SQLite) | Partially — rows per session | 0 B on author machine (schema: `sessions`, `turns`, `session_files`, `checkpoints`, FTS index) | Shared DB for all sessions. Delete rows keyed by session id: `sessions`, `turns`, `session_files`, `checkpoints` (+ FTS). |

### Per-session side artifacts

Everything below lives under `~/.copilot/session-state/<session-uuid>/`:

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `session-state/<uuid>/events.jsonl` | Yes | **2.2–2.4 MB** each on author machine; whole-session conversation + tool calls, single largest Copilot artifact | Full transcript. Delete with session. (Omnivue adapter reads it as the message source.) |
| `session-state/<uuid>/session.db` (SQLite: `todos`, `todo_deps`, `inbox_entries`) | Yes | ~28 KB | Per-session todo DB. Delete with session. |
| `session-state/<uuid>/plan.md` | Yes | ~8 KB | Implementation plan markdown. Delete with session. |
| `session-state/<uuid>/checkpoints/` (`001-*.md`, `index.md`) | Yes | KBs | Plan/checkpoint markdown. Delete with session. |
| `session-state/<uuid>/rewind-snapshots/` (`index.json` + `backups/<hash>/` raw files) | Yes | Grows with session; raw file backups | **Can be large** (full file backups per snapshot). Delete with session to reclaim real space. |
| `session-state/<uuid>/workspace.yaml` | Yes | <1 KB | Per-session workspace config. Delete with session. |
| `session-state/<uuid>/files/`, `research/` | Yes (often empty) | 0 B in author data | Reserved dirs; part of session. |
| `session-state/<uuid>/inuse.<pid>.lock` | Yes | ~6 B | Stale lock after session ends; remove. |

### NOT per-session (leave alone)

- `~/.copilot/session-store.db` itself (shared; only rows are per-session).
- `~/.copilot/logs/`, `~/.copilot/config.json` (auth config).
- `session-state/<other-uuid>/` dirs of other sessions.

---

## Claude Code

Data root: `~/.claude/`.

### Primary session record

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.claude/projects/<project-dir>/<session-uuid>.jsonl` | Yes | **10–200 KB typical; up to ~5.6 MB** on author machine (a long session); one JSONL per session (user/assistant/system/tool lines) | Primary transcript. Delete per session. `<project-dir>` is a slug of the workdir path (e.g. `-Users-stcrawfo-Development-javascript-sess`). |
| `~/.claude/projects/<project-dir>/sessions-index.json` | **Per-project index, has per-session entries** | ~1 KB | Shared index of all sessions (id, summary, mtime). When deleting a session, REMOVE its entry, do not delete the file. |

### Per-session side artifacts

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.claude/projects/<project-dir>/<session-uuid>/subagents/agent-*.jsonl` | Yes | **100 KB–800 KB each** on author machine; one per subagent invocation | Subagent transcripts. Delete with session. |
| `~/.claude/projects/<project-dir>/<session-uuid>/subagents/agent-*.meta.json` | Yes | KBs | Subagent metadata. Delete with session. |
| `~/.claude/projects/<project-dir>/<session-uuid>/` (the per-session dir itself) | Yes | — | Exists only when the session spawned subagents; delete whole dir. |
| `~/.claude/plans/<slug>.md` | Yes (keyed by session slug) | KBs–tens of KB | Plan file referenced by session slug. Delete when the session is deleted and no other session shares the slug. |
| `~/.claude/todos/<session-uuid>-agent-<session-uuid>.json` | Yes | KBs | Per-session todo file. Delete with session. |
| `~/.claude/shell-snapshots/<session-uuid>.*.sh` | Yes | 0 B–KBs (author machine empty) | Per-session shell-state restore files; can be large on heavy sessions. Delete with session. |

### NOT per-session (leave alone)

- `~/.claude/history.jsonl` — global history of every user message across all sessions (27 lines author machine); update/remove entries for the session, do not delete file.
- `~/.claude/settings.json`, `~/.claude/auth` / `credentials`, `daemon/`, `daemon.lock`, `daemon.log`, `daemon.status.json`, `plugins/`, `cache/`, `debug/`, `backups/`, `file-history/` (120 KB, file-edit history shared across sessions), `paste-cache/`, `stats-cache.json`, `telemetry/`, `downloads/`, `jobs/`, `tasks/`, `session-env/`, `sessions/` (native sqlite: e.g. `sessions/58885.json`), `.last-cleanup`, `.last-update-result.json`.
- `~/.claude/projects/<other-project-dir>/` and the `projects/` root.

---

## OpenAI Codex

Data root: `~/.codex/` (honors `$CODEX_HOME`).

### Primary session record

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<session-uuid>.jsonl` | Yes | **200 KB–1 MB each** on author machine (max 958 KB); date-partitioned, self-contained transcript | The session. Delete the file; prune the now-empty date dirs. |
| `~/.codex/session_index.jsonl` | **Global index, one line per session** | ~1 KB / session | Delete the session's line (`id`, `thread_name`, `updated_at`); never the file. |

### Per-session side artifacts

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.codex/state_5.sqlite` → `threads` table (+ `thread_spawn_edges`) | Rows per session | 180 KB file (author); ~1 row/session | Shared DB. Delete `threads` row by `id` (points at `rollout_path`) + spawn edges. Also holds `thread_dynamic_tools`. |
| `~/.codex/shell_snapshots/<session-uuid>.<ns>.sh` | Yes | 784 KB total (author, 2 files) | Per-session shell snapshot used by resume. Delete with session. |
| `~/.codex/goals_1.sqlite` → `thread_goals` | Rows per session | 24 KB | Delete rows for the thread id. |
| `~/.codex/logs_2.sqlite` → `logs` (26 MB author) | Rows tagged `thread_id` | file 26 MB | Shared log DB; rows carry `thread_id`. Delete (or optionally prune) rows for the thread. |
| Legacy/older-version dirs `~/.codex/edits/*.json` and `~/.codex/plans/*.json` | Yes | — | Not present on author machine; cited in omnivue `docs/ADAPTERS.md` as edit/plan event files. Delete per session if present. |

### NOT per-session (leave alone)

- `~/.codex/auth.json`, `config.toml`, `.codex-global-state.json` (+ `.bak`), `installation_id`, `.personality_migration`, `models_cache.json`.
- `~/.codex/cache/` (remote plugin/apps catalogs), `plugins/`, `rules/`, `skills/`, `tmp/`, `.tmp/`, `process_manager/`, `vendor_imports/`, `computer-use/` (global config), `memories_1.sqlite` (global memory pipeline), `sqlite/`.
- `session_index.jsonl` for OTHER sessions' lines; `sessions/` date dirs containing other sessions.

---

## Pi

Data root: `~/.pi/agent/sessions/`.

### Primary session record

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.pi/agent/sessions/<workdir-slug>/<timestamp>_<session-uuid>.jsonl` | Yes — **one file per session** | **500 KB–1 MB each** on author machine (max 1.0 MB); header line `{"type":"session",...}` + `message`/`toolResult`/`model_change` event lines | The entire session. Delete the single file. Workdir-slug dirs are per-working-directory (e.g. `--Users-stcrawfo-Development-javascript-sess--/`). |

### Per-session side artifacts

None. Pi stores no per-session side files (no plans/diffs/snapshots; omnivue Pi adapter returns `(nil, nil)` for Plan/Diffs).

### NOT per-session (leave alone)

- `~/.pi/agent/auth.json`, `~/.pi/agent/settings.json`, `~/.pi/agent/models-store.json` (180 KB), `~/.pi/agent/bin/` (6.7 MB runtime).
- `sessions/<other-workdir-slug>/` and the `.jsonl` files of other sessions.

---

## Cursor

macOS data roots: `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` + `~/.cursor/`
(omnivue `ingestkit.FindCursorVscdbPath` also checks `~/.config/Cursor/User/globalStorage/state.vscdb` on Linux).

### Primary session record

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` (SQLite KV) — keys `composerData:<session-id>` (+ `composerHeaders`, `ItemTable`) | Rows per session | **~500 MB** on author machine (504,639,488 B) + 4.6 MB WAL + 481 MB `.backup` | Shared global DB for ALL sessions. Delete the per-session KV rows: `composerData:<id>` (session metadata), `bubbleId:<session>:*` (messages; hundreds of rows/session on author machine), `checkpointId:<session>:*`, `codeBlockDiff:<session>:*`. DB doesn't shrink without `VACUUM`. |
| `~/.cursor/projects/<project-dir>/agent-transcripts/<session-uuid>/<session-uuid>.jsonl` | Yes | **10–230 KB each** on author machine | Agentic session transcript (the omnivue fallback when bubble KV is absent). Delete with session. |

### Per-session side artifacts

| Path / store | Per-session? | Typical size | Deletion implication |
|---|---|---|---|
| `~/.cursor/projects/<project-dir>/agent-transcripts/<session-uuid>/subagents/<subagent-uuid>.jsonl` | Yes | ~20 KB each (author) | Subagent transcripts. Delete with session. |
| `state.vscdb` keys `composer.content.<hash>` | **Content-addressed, deduplicated across sessions** | hash → file-content blob | Shared by content hash — only safe to delete when no remaining session references the hash (reference-count check required). |
| `~/.cursor/ai-tracking/ai-code-tracking.db` (SQLite: `conversation_summaries` keyed by `conversationId`, `tracked_file_content`, `ai_code_hashes` [3,028 rows author], `ai_deleted_files`, `scored_commits`) | `conversation_summaries` rows per session; `tracked_file_content`/`ai_code_hashes` content-addressed/shared | 1.3 MB total (author) | Delete the session's `conversation_summaries` row (keyed by session/thread id). Leave content-addressed tables alone unless reference-counted. |

### NOT per-session (leave alone)

- `state.vscdb` itself (shared), `state.vscdb-wal`, `state.vscdb-shm`, `state.vscdb.backup`, `state.vscdb.options.json`.
- `~/.cursor/projects/<project-dir>/` per-project siblings: `canvases/`, `assets/`, `terminals/`, `mcps/` (shared across that project's sessions).
- `~/.cursor/extensions/`, `skills-cursor/`, `plugins/`, `agents/`, `ai-tracking/` (only `ai-code-tracking.db` rows are per-session), `statsig-cache.json`, `argv.json`, `cli-config.json`, `prompt_history.json`, `.gitignore`.

---

## Not per-session (leave alone)

Cross-agent shared/global stores a cleanup tool must NEVER delete (or only ever edit rows of):

| Agent | Leave-alone paths | Notes |
|---|---|---|
| OpenCode | `opencode.db` shared tables (`project`, `workspace`, `account`, `credential`, `permission`, `event`/`event_sequence`), `auth.json`, `bin/`, `log/`, `storage/project/`, `~/.cache/opencode/` | `snapshot/<project-id>/` is shared across the project's sessions — only `git gc` after row deletion. |
| Copilot | `session-store.db` (rows only), `logs/`, `config.json` | `rewind-snapshots/` are per-session but may be the single biggest reclaimable chunk. |
| Claude Code | `history.jsonl`, `settings.json`, `daemon/`, `plugins/`, `cache/`, `file-history/`, `sessions/`, `stats-cache.json`, `sessions-index.json` file itself | Edit `sessions-index.json` and `history.jsonl` entries; don't delete files. |
| Codex | `auth.json`, `config.toml`, `cache/`, `plugins/`, `rules/`, `skills/`, `memories_1.sqlite`, `.codex-global-state.json`, `session_index.jsonl` file itself | `state_5.sqlite`/`goals_1.sqlite`/`logs_2.sqlite` are shared DBs — delete rows only. |
| Pi | `auth.json`, `settings.json`, `models-store.json`, `bin/` | Truly one-file-per-session; no shared session DB at all. |
| Cursor | `state.vscdb` + WAL/SHM/backup, `composer.content.<hash>` blobs, `ai-code-tracking.db` content-addressed tables, per-project dirs (`canvases/`, `assets/`, `terminals/`, `mcps/`), `extensions/`, `skills-cursor/` | Content-addressed tables require reference-counting before any deletion. |
