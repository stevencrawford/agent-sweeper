# SQLite session-store deletion facts

Research target: the exact tables/keys, delete order, WAL/locking behavior, and
cross-store linkage needed to delete ONE session and all its rows from each AI
coding-agent SQLite-backed session store.

Primary local reference: the omnivue repo
(`internal/ingest/{opencode,copilot,cursor,codex}/` — the exact SQL those
adapters run is quoted inline below where it matters). Web sources: the
opencode source (`anomalyco/opencode`, `packages/opencode/src/session/session.sql.ts`,
`storage/db.ts`, `session.ts`), GitHub Copilot CLI docs + issues, cursor-helper /
cursaves / tokenuse schema write-ups, and the openai/codex repo.

Path conventions (macOS shown; Linux uses `~/.config/...` for Cursor):

| Store | DB file | Live while agent runs? |
|---|---|---|
| OpenCode | `~/.local/share/opencode/opencode.db` | yes, always held open |
| Copilot | `~/.copilot/session-store.db` (+ `data.db` on new builds) | yes, during a session |
| Cursor | `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` (+ per-workspace `state.vscdb`) | yes, while IDE runs |
| Cursor AI-tracking | `~/.cursor/ai-tracking/ai-code-tracking.db` | only when a Cursor agent session runs |
| Codex | **no SQLite for sessions** — plain files under `~/.codex/` | yes, rollout file appended live |

---

## OpenCode — `opencode.db`

Location: `${XDG_DATA_HOME:-~/.local/share}/opencode/opencode.db`. Env overrides:
`XDG_DATA_HOME`, `OPENCODE_DB`. Non-stable channels use `opencode-<channel>.db`.
Source of schema: `packages/opencode/src/session/session.sql.ts` (Drizzle).

### Tables involved in a session delete

| Table | Keys | FK / cascade | Notes |
|---|---|---|---|
| `session` | `id` TEXT PK | `project_id` → `project.id` `ON DELETE CASCADE` | the parent row. `parent_id` is self-ref **without** an FK (index only) |
| `message` | `id` TEXT PK | `session_id` → `session.id` `ON DELETE CASCADE` | `data` = JSON blob (role, modelID, tokens, time) |
| `part` | `id` TEXT PK | `message_id` → `message.id` `ON DELETE CASCADE`; `session_id` is NOT NULL **but NOT a FK** (index only) | `data` = JSON discriminated by `type` (text/reasoning/tool/step-*/snapshot/patch/compaction). Denormalized `session_id` copy |
| `todo` | composite PK `(session_id, position)` | `session_id` → `session.id` `ON DELETE CASCADE` | per-session todo list |
| `session_message` (v2) | `id` PK | `session_id` → `session.id` `ON DELETE CASCADE` | event-sourced v2 tables, populated on stable installs |
| `session_input` (v2) | — | `session_id` → `session.id` | event-sourced input queue |
| `event` / `event_sequence` (v2) | — | keyed by session_id | mirrors `part` text; duplicate of transcript |
| `project` | `id` PK | — | **do NOT delete**; shared across all sessions of a project |
| `permission` | `project_id` PK | → `project.id` CASCADE | project-scoped, NOT session-scoped — **do NOT touch** on a session delete |

Also present (secret-bearing, never relevant to session delete): `account`,
`account_state`, `control_account`, `credential`.

### DELETE ORDER

FKs are declared with `ON DELETE CASCADE`, so with `PRAGMA foreign_keys = ON`
on **your** connection a plain `DELETE FROM session WHERE id = ?` cascades to
`message`, then `part` (via `message_id`), `todo`, `session_message`,
`session_input`, `event`/`event_sequence`. Safe explicit order if you prefer not
to rely on cascade (belt-and-suspenders):

1. `DELETE FROM part WHERE session_id = ?`
2. `DELETE FROM message WHERE session_id = ?`
3. `DELETE FROM todo WHERE session_id = ?`
4. `DELETE FROM session_message WHERE session_id = ?`
5. `DELETE FROM session_input WHERE session_id = ?`
6. `DELETE FROM event WHERE session_id = ?` (+ `event_sequence` if present)
7. `DELETE FROM session WHERE id = ?` — last, parent row

Child sessions: `session.parent_id` links child (sub-agent / forked) sessions.
OpenCode's own `remove()` deletes children recursively (see below). A cleaner
must either recurse over `SELECT id FROM session WHERE parent_id = ?` or
explicitly decide to leave children. `session.parent_id` has no FK, so leaving
children orphaned is silent — they just stop being listed under a parent.

Wrap everything in one `BEGIN IMMEDIATE ... COMMIT` transaction.

### What OpenCode itself does (reference behavior)

`Session.remove` (`packages/opencode/src/session/session.ts`) — recursive:
cancel background jobs, `remove(child.id)` for every `children(id)`, then
publish `Event.Deleted` and `events.remove(sessionID)`. The v2 event bridge
carries the actual row deletion; the `message`/`part`/`todo` cascade handles the
relational rows. CLI: `opencode session delete <sessionID>`. API:
`DELETE /api/session/{sessionID}`.

### WAL / locking / running-agent behavior

From `packages/opencode/src/storage/db.ts`, opencode opens the DB with:
`PRAGMA journal_mode = WAL; synchronous = NORMAL; busy_timeout = 5000;
cache_size = -64000; foreign_keys = ON; wal_checkpoint(PASSIVE)`.

- opencode keeps the DB open for the lifetime of the TUI/daemon. A live session
  appends `message`/`part` rows continuously via the WAL.
- WAL gives you snapshot-isolated reads. The DB file you see is consistent even
  while opencode writes — **but** un-checkpointed data lives in the
  `-wal` sidecar; a byte-copy backup must include `opencode.db-wal` and
  `-shm` (or checkpoint first). `PRAGMA wal_checkpoint(PASSIVE)` at the end of
  your write flush merges your changes back into the main file.
- To write (delete) against a live DB: open with `busy_timeout`, issue the
  deletes inside one `BEGIN IMMEDIATE` transaction, and be ready to retry on
  `SQLITE_BUSY`. The running opencode process may not notice the rows are gone
  (it caches sessions in memory) and may re-create/rewrite them or error on the
  next append. **Recommendation: quit opencode before deleting, or only delete
  sessions that are not the currently active one.** 
- `PRAGMA foreign_keys = ON` matters: it is ON for opencode's own connection
  (that's how its cascade works), but it is **off by default** in a fresh
  sqlite3 connection. Your tool must set it, otherwise `DELETE FROM session`
  silently leaves orphaned `message`/`part`/`todo` rows behind.
- Read-only precedent in this repo: omnivue opens with
  `file:<path>?mode=ro&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)`
  then `PRAGMA query_only = ON` (`internal/ingest/adapter.go:56`).

### Cross-store cleanup — `snapshot/` git repos and `storage/` blobs

- Snapshot layer: `~/.local/share/opencode/snapshot/<project_id>/` — a separate
  hidden **Git object DB per project** (uses `git --git-dir ... write-tree`,
  no commits). Per-`project_id`, shared across ALL sessions of that project
  (see issue #7775: the snapshot index is shared per project). Session→snapshot
  links live only in the DB: `session.summary_diffs` and
  `session.revert.snapshot` JSON, and `part.data.snapshot` on `step-start` parts.
- Deleting a session does NOT free snapshot storage, and you must **not** delete
  `snapshot/<project_id>/` for one session — other sessions in the same project
  reference the same tree objects. Orphaned objects become unreachable and are
  cleaned by opencode's hourly `git gc --prune=7.days`. (Bug #17753: this dir
  can grow to 170GB+.)
- Blob layer: `~/.local/share/opencode/storage/` holds JSON blobs keyed by
  `["session_diff", <sessionID>]` and similar session-scoped keys (revert diff,
  summaries). Session-specific subkeys under `storage/session_diff/<id>` can be
  removed per session; they are not shared. OpenCode's own delete does not
  clean these either.
- Legacy (pre-Feb-2026) installs may still have per-session JSON files under
  `~/.local/share/opencode/storage/session/...` (one-time-migrated to SQLite).
  A session whose JSON survived migration is invisible to the current UI; safe
  to also delete those files by session id.

---

## GitHub Copilot — `~/.copilot/session-store.db`

Location: `~/.copilot/session-store.db` (+ `~/.copilot/data.db` on newer
builds, + `-wal`/`-shm` sidecars). The DB is a **derived index** over the
session files in `~/.copilot/session-state/<uuid>/` — `/chronicle` and the
`copilot --resume` picker read it. GitHub docs: "To remove data for a particular
CLI session locally, delete the relevant session directory from
`~/.copilot/session-state/`. After doing this you must manually reindex the
session store." The store is reconstructable from the files
(`copilot --reindex` / `session_indexing`).

### Tables (schema confirmed by multiple independent sources)

| Table | Keys | Notes |
|---|---|---|
| `sessions` | `id` TEXT PK | `id` is the session UUID **which is also the session-state dir name**; `cwd`, `repository`, `branch`, `summary`, `created_at`, `updated_at` (ISO-8601 TEXT), `host_type` |
| `turns` | `id` INTEGER PK, `session_id` TEXT (indexed), `turn_index` | `user_message`, `assistant_response` (≈ first 1000 chars), `timestamp` |
| `checkpoints` | `session_id`, `checkpoint_number` | `title`, `overview`, `history`, `work_done`, `technical_details`, `important_files`, `next_steps`, `created_at` |
| `session_files` | `session_id`, `file_path`, `tool_name`, `turn_index`, `first_seen_at` | files edited per session |
| `session_refs` | `session_id`, `ref_type`, `ref_value`, `turn_index`, `created_at` | commits/PRs/issues linked |
| `search_index` | FTS5 virtual table; `content`, `session_id`, `source_type`, `source_id` | full-text index; `source_id`/rowid unrelated to `turns.id` — delete by `session_id` |
| `assistant_usage_events` | `session_id` + per-request | new (~Jul 2026); real model + token rows per request |
| `data.db.sessions` | `session_id` | authoritative running token totals (`model`, `total_input_tokens`, `total_output_tokens`, `total_cached_tokens`, `total_reasoning_tokens`) on newer builds |

The `turns` table joins by `session_id`; omnivue reads exactly this shape
(`SELECT turn_index, user_message, assistant_response, timestamp FROM turns
WHERE session_id = ?`) and the sessions table
(`SELECT id, cwd, repository, branch, summary, created_at, updated_at ... FROM sessions`,
`internal/ingest/copilot/sessions.go`).

### DELETE ORDER

Because the store is a derived index, either order (DB-first or dir-first)
works, but doing both keeps `/chronicle` and `--resume` consistent. Single
transaction:

1. `DELETE FROM search_index WHERE session_id = ?` (FTS5 — no FK, would orphan)
2. `DELETE FROM turns WHERE session_id = ?`
3. `DELETE FROM checkpoints WHERE session_id = ?`
4. `DELETE FROM session_files WHERE session_id = ?`
5. `DELETE FROM session_refs WHERE session_id = ?`
6. `DELETE FROM assistant_usage_events WHERE session_id = ?` (if present)
7. `DELETE FROM sessions WHERE id = ?`
8. Same for `data.db` (`sessions` by id) on builds that have it.
9. Filesystem: `rm -rf ~/.copilot/session-state/<uuid>/` (contains
   `events.jsonl`, `session.db` (todos/inbox SQLite — deleted with the dir),
   `workspace.yaml`, `checkpoints/`, `rewind-snapshots/`, `files/`,
   `research/`, plan.md).

### What breaks if orphaned

- DB row left but dir removed → ghost session in `/chronicle` / `--resume` that
  cannot open (a known real-world inconsistency, cf. copilot-cli issues #2654,
  #2655, #2836 about drift between the store and session-state).
- Dir left but DB row removed → session invisible in the picker but still
  occupying disk; a later reindex resurrects it (so "deleting" the dir alone and
  skipping the store isn't permanent once the user runs `/reindex`).
- Deleting the whole `session-store.db` loses the derived index for ALL
  sessions; it is recoverable by reindex but this is an all-or-nothing blast
  radius.

### WAL / locking / running-agent behavior

- Both `session-store.db` and `data.db` run in WAL. Un-checkpointed turns live
  in the `-wal` file; a backup must copy `-wal`/`-shm` or checkpoint.
- A live CLI session writes to `session-store.db` periodically and at shutdown,
  and holds an `inuse.<pid>.lock` in its session-state dir. Deleting a **live**
  session's rows races the CLI's incremental writes (it may re-insert turns).
  Skip sessions with a live lock file, or skip by recency.
- Docs explicitly bless the filesystem delete (`rm -rf session-state/<uuid>/`)
  as the supported per-session removal, then `copilot --reindex`. That is the
  least-fragile path; DB-only delete still needs the dir gone to be permanent.

### Cross-store linkage

`session-state/<uuid>/` ↔ `session-store.db` row: same UUID. `rewind-snapshots/`
and `checkpoints/` are files inside the session dir and die with it. There is no
git-repo linkage like OpenCode; nothing shared across sessions in the store
(except you must not truncate FTS `search_index` globally).

---

## Cursor — `state.vscdb`

Two SQLite DBs, each with exactly two KV tables (no FKs, no relational joins):

```sql
CREATE TABLE ItemTable    (key TEXT UNIQUE, value BLOB);
CREATE TABLE cursorDiskKV (key TEXT UNIQUE, value BLOB);
```

- Global: `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`
  (Linux: `~/.config/Cursor/...`) — conversation CONTENT.
- Per-workspace: `~/Library/Application Support/Cursor/User/workspaceStorage/<ws>/state.vscdb`
  — sidebar INDEX (`ItemTable` key `composer.composerData`, and
  `composer.composerHeaders` in newer builds).

A session = a "composer" with a UUID (`composerId`). Everything is keyed by that
UUID; there is no session row to cascade from.

### Rows involved in deleting one composer (global DB, `cursorDiskKV`)

| Key pattern | Content | Delete? |
|---|---|---|
| `composerData:<id>` | session header + `fullConversationHeadersOnly[]` | yes |
| `bubbleId:<id>:<bubble>` | one message per bubble | yes (all `LIKE 'bubbleId:<id>:%'`) |
| `checkpointId:<id>:<cp>` | file-snapshot checkpoints (undo/restore) | yes (all) |
| `messageRequestContext:<id>:<msg>` | full request context sent to model | yes (all) |
| `codeBlockDiff:<id>:<diff>` | inline code-suggestion diffs | yes (all) |
| `codeBlockPartialInlineDiffFates:<id>:*` | inline-diff acceptance state | yes (all) |
| `agentKv:checkpoint:<id>` | Merkle root for LLM context | yes |
| `agentKv:blob:<sha256>` | content-addressed blob | **no — shared**, content-addressable |
| `composer.content.<hash>` | content-addressed blobs, SHARED across conversations | **no** |

Workspace DB (`ItemTable` key `composer.composerData`): the value is JSON with
`allComposers[]`, `selectedComposerIds[]`, `lastFocusedComposerIds[]`. Remove the
`composerId` from **all three** arrays and `UPDATE` the row. This is what makes
the conversation leave the sidebar. (The famous "vanished chat" forum thread is
exactly this coupling: sidebar reads ONLY `allComposers` in the workspace DB;
content lives in the global DB. Delete both sides or the ghost/phantom bug
appears.)

### DELETE ORDER / rules

No FK — issue explicit deletes; order between the two DBs doesn't matter but do
both:
1. Global `cursorDiskKV`: delete `composerData:<id>`, all
   `bubbleId:<id>:%`, `checkpointId:<id>:%`, `messageRequestContext:<id>:%`,
   `codeBlockDiff:<id>:%`, `codeBlockPartialInlineDiffFates:<id>:%`,
   `agentKv:checkpoint:<id>%`.
2. Workspace `ItemTable`: rewrite the `composer.composerData` JSON to drop the
   id from `allComposers`/`selectedComposerIds`/`lastFocusedComposerIds`.
3. NEVER touch `composer.content.*` / `agentKv:blob:*` (content-addressed,
   shared; other composers reference them).

### What breaks if orphaned

- Global rows gone, workspace index entry left → sidebar ghost that won't
  render (cursaves: "Missing `composerData` or `bubbleId` entries won't
  render"; `checkpointId` entries missing → agent conversation can't be
  continued).
- Workspace entry gone, global rows left → invisible to UI but still on disk
  (the recover scenario in the forum thread).

### WAL / locking / running-agent behavior

- `state.vscdb` is WAL-mode (observed "3 with WAL"). Cursor (IDE) holds it open
  for the app's lifetime; per-workspace DBs are opened on workspace load.
- **Cursor must be quit** before writing. A live IDE can clobber your deletes on
  the next autosave and may keep the file locked (`SQLITE_BUSY`). The
  `cursorDiskKV` renames performed by Cursor on workspace rename are also
  disruptive mid-flight.
- No `PRAGMA foreign_keys` concerns — there are no FKs.
- Related non-DB stores with the same UUID, cleaned separately if you want a
  complete wipe: `~/.cursor/projects/<hash>/*.jsonl` transcripts (incl.
  `agent-transcripts/*.txt`), `~/.cursor/chats/<projectHash>/<conversationId>/store.db`.

---

## Cursor — `~/.cursor/ai-tracking/ai-code-tracking.db`

SQLite at `~/.cursor/ai-tracking/ai-code-tracking.db` (Linux
`~/.config/Cursor/...`). Attribution DB for AI-vs-human code metrics; consumed
by Cursor's own "AI code attribution" and by third-party cost tools.

### Tables and the session link

| Table | Link to conversation | Notes |
|---|---|---|
| `conversation_summaries` | `conversationId` | `title`, `tldr`, `model`, `mode`, `updatedAt` — the conversation↔session summary row |
| `ai_code_hashes` | `conversationId` | one row per AI-authored code line: `hash`, `source`, `fileName`, `model`, `timestamp` |
| `tracked_file_content` | `conversationId` | `gitPath`, `content`, `model` |
| `scored_commits` | **no conversationId** | keyed by commit (split of AI/human lines) — do NOT delete unless the commit belongs solely to this conversation |

The link column is `conversationId` = the same composer UUID used in
`state.vscdb`.

### Is it safe to delete rows there?

Yes. Safe per-conversation deletes:
`DELETE FROM conversation_summaries WHERE conversationId = ?`,
`DELETE FROM ai_code_hashes WHERE conversationId = ?`,
`DELETE FROM tracked_file_content WHERE conversationId = ?`.

Impact is only loss of attribution/cost-metric data; nothing in the IDE breaks
(sessions render from `state.vscdb`, not this DB). `scored_commits` should be
left alone unless the commit is exclusively produced by the deleted
conversation. No FKs, no cascade, no WAL concern beyond the standard "close
Cursor / skip live sessions" guidance. This DB is **optional** cleanup — deleting
`state.vscdb` rows alone leaves the session fully usable; deleting here just
removes attribution history.

---

## OpenAI Codex — `~/.codex` (NOT SQLite)

Codex does **not** store sessions in SQLite. Sessions are plain JSONL files;
SQLite is only a derived index.

### Exact layout

```
~/.codex/                                  # CODEX_HOME (env override)
├── sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl   # one file per session (the transcript; may be .zst-compressed when cold)
├── archived_sessions/rollout-<ts>-<uuid>.jsonl     # archived copies (same naming)
├── session_index.jsonl                    # append-only id → thread_name journal (title cache)
├── state_<N>.sqlite / state_<N>.sqlite-wal / -shm # threads/jobs index (derived; schema drifts)
├── logs_1.sqlite                          # ring-buffer log DB
├── shell_snapshots/<thread-uuid>.<ns>.sh  # env snapshot per session start
├── auth.json / config.toml / history.jsonl / memories/ / skills/ ...
```

The session id is the UUIDv7 in the rollout filename AND in the leading
`session_meta` record. The `state_*.sqlite` `threads` table holds
(`id`, `cwd`, `title`, timestamps, `rollout_path`) — a cheap index over the
rollout files; readers (incl. omnivue's codex adapter) treat the JSONL as the
source of truth and the SQLite as fast-list-only.

### Deleting one session

No child tables, no FKs — a session is: (1) its rollout file
`(live sessions/... or archived_sessions/)`, (2) its `session_index.jsonl`
line(s), (3) its `shell_snapshots/<thread-uuid>.*.sh`, (4) an optional
`threads` row in `state_<N>.sqlite`.

1. Delete the rollout file (`sessions/YYYY/MM/DD/rollout-<id>.jsonl`, or the
   archived copy too).
2. Rewrite `session_index.jsonl` without the `id` line.
3. Remove `shell_snapshots/<id>.*.sh`.
4. Optionally `DELETE FROM threads WHERE id = ?` in `state_<N>.sqlite` (and
   subagent child `threads` rows where `parent_thread_id = ?` if you want the
   whole tree). Leaving the row is tolerable — Codex logs a "stale rollout
   path" error and self-heals (issue #31074), but removing it is cleaner.
5. Parent/child: Codex tracks `thread_spawn_edge`; children are separate
   rollout files. Deleting a parent session leaves its subagent rollouts unless
   you recurse on `thread_spawn_edge`/`parent_thread_id`.

**Preferred**: shell out to Codex's own command — `codex delete <SESSION>`
permanently deletes the transcript (and `codex archive` hides without deleting).
The CLI does exactly the above. grikomsn/codex-chat-manager does the same
cascade manually (deletes archived + active rollout JSONLs, matching
`session_index.jsonl` lines, matching `shell_snapshots/*.sh`; never touches
SQLite).

WAL/locking: the state SQLite runs with `-wal` sidecars but is a cache; a live
`codex` process appends to the active session's rollout file, so do not delete
the currently-running session's rollout mid-session (the recorder errors).
Otherwise no locking concerns — plain file removal is safe while codex is not
writing that file. No backup requirement beyond normal care.

Note: omnivue's `docs/ADAPTERS.md` also lists `~/.codex/edits/*.json` and
`plans/*.json`; the actual codex adapter and the openai/codex source show all
session data inside the rollout JSONL (event types `event_msg`,
`response_item`, `session_meta`) — the `edits/`/`plans/` dirs were not observed
in upstream layouts.

---

## Cross-cutting safety (WAL, backups, running agents)

1. **All three SQLite stores run in WAL mode.** This means:
   - Reads are non-blocking and snapshot-consistent even while the agent
     writes — good for listing/preview.
   - Writes contend for a single writer lock. Set `PRAGMA busy_timeout` (e.g.
     5000ms) and wrap all deletes for one session in a single
     `BEGIN IMMEDIATE ... COMMIT` transaction, retrying on `SQLITE_BUSY`.
   - Data not yet checkpointed lives in the `-wal` sidecar. Any byte-copy
     backup of the DB must copy `-db` + `-wal` + `-shm` together, OR run
     `PRAGMA wal_checkpoint(PASSIVE)` / use SQLite's backup API
     (`sqlite3 <db> ".backup <dest>"` or `VACUUM INTO`) to get a consistent
     snapshot. Copying only the main file silently drops recent rows.
   - `immutable=1` (or `mode=ro` without the `-wal` present) misses
     un-checkpointed rows — read with `mode=ro` and let SQLite see the WAL
     (that is what omnivue's `OpenReadOnlyDB` does).

2. **Backups before writing: recommended, not optional for destructive ops.**
   Take a `VACUUM INTO`/`.backup` copy before the first delete in a batch, or
   a cheap copy of `db+wal+shm`. Restore-on-corruption is otherwise impossible
   after a mid-write crash. For all-file stores (Codex) a copy of the rollout
   files suffices.

3. **Running-agent behavior (the "live agent holds the DB open" question):**
   - OpenCode: DB held open for the app's lifetime; live sessions append
     message/part rows constantly. Deleting the active session races the
     append loop and the in-memory session cache. **Quit the agent or skip the
     active session.** OpenCode sets `foreign_keys = ON` on its own connection;
     your tool must too, or `DELETE FROM session` will orphan `message`/`part`.
   - Copilot: live session holds an `inuse.<pid>.lock` in its session-state dir
     and periodically writes `session-store.db`. Skip locked/recent sessions;
     GitHub's supported removal is deleting the session-state dir + reindex.
   - Cursor: `state.vscdb` is held open by the IDE; quit Cursor before writing
     or you fight its autosave.
   - Codex: only the active rollout file is being appended; don't delete a
     running session's file.

4. **`PRAGMA foreign_keys`:** OpenCode and (per source) none of the other
   stores depend on it except OpenCode's cascade. Default sqlite3 connections
   have it OFF. Set it explicitly on any connection that issues a session-row
   delete against `opencode.db`.

5. **Shared content-addressed / cross-session data you must NOT delete:**
   - OpenCode `snapshot/<project_id>/` git repos (shared per project),
     `project`/`permission` rows.
   - Cursor `composer.content.*` and `agentKv:blob:*` (shared across composers).
   - Copilot: nothing shared except avoid truncating FTS `search_index` globally.
   - Codex `state_*.sqlite`: shared index; only touch the specific `threads`
     row.
