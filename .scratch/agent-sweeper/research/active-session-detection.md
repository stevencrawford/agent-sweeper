# Active-Session Detection Research

**Goal:** for each supported agent, determine which sessions are *currently open / in use* so agent-sweeper can protect them from deletion.

**Method:** local reference (omnivue adapters in `internal/ingest/`, resume-command patterns in `internal/terminal/`, `docs/ADAPTERS.md`) plus web research of each agent's official docs and GitHub source.

**Resume-command argv patterns observed in omnivue** (each agent's `ResumeCommand` — these are the exact argv strings a live process carries):

| Agent | Resume argv | Session ID format |
|---|---|---|
| OpenCode | `opencode -s <id>` | `ses_<base62>` |
| Claude Code | `claude -r <id>` | ULID (26-char, e.g. `01J...`) |
| Codex | `codex resume <id>` | UUID v7 (rollout `rollout-<ts>-<uuid>.jsonl`) |
| Pi | `pi --session <id>` | UUID (`<timestamp>_<uuid>.jsonl`) |
| Copilot | `copilot --resume=<id>` | UUID |
| Cursor | `cursor --composer <id>` | Composer UUID |

---

## OpenCode

**Storage root:** `~/.local/share/opencode/` (macOS/Linux). Sessions live in SQLite `opencode.db` (`session`, `message`, `part` tables) in modern versions; older versions used JSON files under `storage/session/<projectID>/<id>.json`, `storage/message/`, `storage/part/`. `~/.local/share/opencode/project/<projectID>.json` maps project → git root / worktree. Session IDs look like `ses_561eca5ebffeCngoybZWxbTrD8`.

**Process detection**
- Binary: `opencode` (single Bun-compiled binary; the TUI spawns an in-process worker/server, so one OS process per session typically).
- argv patterns (from `packages/opencode/src/cli/cmd/tui.ts`, `run.ts`):
  - `opencode -s <ses_id>` / `opencode --session <ses_id>` → **exact session ID, high confidence**
  - `opencode run -s <id> ...` / `opencode run --continue` → non-interactive run
  - `opencode -c` / `--continue` → "continue the last session" → resolves to the most-recent session for the process cwd (must resolve via DB query)
  - `opencode [project-dir]` (positional) → **new** session in that dir (nothing to protect yet)
  - `opencode serve` / `opencode web` → headless server; sessions it serves are only visible via its HTTP API

**Active-session state files / DB markers**
- **None durable.** opencode's `sessions.active()` (spec `specs/v2/session.md`) is explicitly *process-local runtime state, empty after restart*; "activity is not durable across process restarts." The `session` table has no `active`/`status` column that flips on exit.
- **Live-server source of truth:** when running, the TUI exposes an HTTP server. `GET /session/status` returns `{ [sessionID]: SessionStatus }` (a live status map), and `GET /session` lists all. Practical only if you can discover the port (only set when `--port`/`--hostname` given; otherwise random).

**Recipe (running process → session id)**
1. Scan processes for binary `opencode`. For each, parse argv for `-s`/`--session` → exact id. Confidence **High**.
2. If argv has `-c`/`--continue` or `--continue` on `run`: the process's cwd pins the project; protect the session with the max `time_updated` in `opencode.db` for that project/directory. Confidence **Medium** (race: another session may be newer).
3. If argv is bare (new session) or `serve`/`web`: no pre-existing session is protected by that process; but protect the most-recent session for the cwd anyway (see fail-safe).
4. cwd also matters for *listing*: opencode scopes sessions per project dir (issue #3551 confirms the session list is per-directory).

**Confidence: High** for `-s` argv; **Medium** for `-c`/continue resolution.

---

## Claude Code

**Storage:** `~/.claude/projects/<encoded-path>/<session-id>.jsonl` transcripts (encoded path = absolute project path with non-alphanumerics → `-`), plus `~/.claude.json` (global config + project history), `~/.claude/history.jsonl`. Session IDs are ULIDs. Also `~/.claude/projects/<path>/sessions-index.json`. `CLAUDE_CONFIG_DIR` overrides the root (also inconsistently relocates `~/.claude.json`).

**Process detection**
- Binary: `claude` (Node). Background/daemon: supervisor process (`claude daemon`).
- argv patterns:
  - `claude -r <id>` / `claude --resume <id>` → **exact session ID** (High). Also `claude -p --resume <id>` (headless continuation).
  - `claude -c` / `--continue` → most recent session in current directory.
  - `claude -r` / `--resume` with no arg → interactive picker (no single pinned session; ambiguous).
  - `claude --bg <prompt>` → spawns background session, prints short ID; session continues under the supervisor daemon even after the terminal dies.
  - `claude -p ...` (headless) → one-shot; session is created, run, and closed within one process lifetime.

**Active-session state files / markers**
- **Background sessions (strongest durable marker):**
  - `~/.claude/daemon/roster.json` — "List of running background sessions, used to reconnect after a restart." Each entry = a live background session (short ID).
  - `~/.claude/jobs/<short-id>/state.json` — per-session supervisor state (kind, cwd, status: `working`/`blocked`/`done`/`failed`/`stopped`).
  - `~/.claude/daemon.log`, `~/.claude/daemon/pipe.key` (Windows).
  - Shell-out surface: `claude agents --json` returns live sessions as JSON with `pid`, `cwd`, `kind`, `startedAt`, `sessionId`, `name`, `status`; `claude daemon status` reports supervisor reachability/PID/session count. Prefer these over parsing files directly.
- **Interactive sessions: no durable marker.** A running `claude` writes/append to its transcript JSONL. Recency heuristic (omnivue uses it): if the last JSONL entry / file mtime is < ~5 min old, treat as active.

**Recipe (running process → session id)**
1. Scan for binary `claude`; parse argv for `-r`/`--resume` → exact id. **High**.
2. For `-c`/`--continue`: cwd → project dir → newest `.jsonl` by mtime / last-append timestamp. **Medium**.
3. Background: parse `~/.claude/daemon/roster.json` + `~/.claude/jobs/*/state.json` (or shell out to `claude agents --json` / `claude daemon status`) → protect every listed live session. **High**.
4. Fallback heuristic: any project JSONL with mtime within N minutes of "now" is presumed live. **Medium** (background stopped sessions can sit for ~1h before process eviction, but stay listed in roster).

**Confidence: High** (argv + roster/`claude agents --json`); **Medium** for mtime heuristic on interactive sessions.

---

## OpenAI Codex CLI

**Storage:** `~/.codex/` (`CODEX_HOME`). Sessions are rollout files `~/.codex/sessions/YYYY/MM/DD/rollout-<iso-ts>-<uuid>.jsonl`. Indexes: legacy `~/.codex/session_index.jsonl`, and current SQLite `~/.codex/state_5.sqlite` (a `threads` table with id, title, cwd, rollout path, archived, pinned, `updated_at`). The JSONL rollout is the append-only source of truth; the SQLite index is a rebuildable projection. Session id = the UUID (v7) in the rollout filename / `session_meta` line.

**Process detection**
- Binary: `codex` (Rust `codex-cli`); node packages are `@openai/codex` — `pgrep -f "@openai/codex"` is an official diagnostic in openai/codex issue #24228.
- argv patterns:
  - `codex resume <uuid>` → **exact session ID** (High)
  - `codex resume --last` → most recent session in cwd (Medium — resolve via `state_5.sqlite` threads `updated_at` or rollout mtime, scoped to cwd unless `--all`)
  - `codex resume` (no args) → interactive picker (ambiguous)
  - `codex exec resume <uuid> "..."` → headless continuation
  - `codex -c experimental_resume="<rollout-path>"` → legacy path-based resume (old versions)
  - bare `codex` → new session (creates new thread; nothing pre-existing to protect)

**Active-session state files / markers**
- **None durable.** No active/lock marker. The rollout file is *not even written until the first API turn completes* (issue #18690). `thread.started` / session id is emitted on stdout/JSON events immediately, so a live process always has the id in its argv or (for SDK spawns) an env/pipe.
- `session_meta` line in the rollout records `id` and `cwd` — useful to resolve `--last` candidates, not for liveness.

**Recipe (running process → session id)**
1. Scan for binary `codex`; parse argv: `resume <uuid>` → exact id. **High**.
2. `resume --last` (or `exec resume` w/o id): cwd → newest thread in `state_5.sqlite` (or newest rollout under cwd). **Medium**.
3. Fallback: `session_index.jsonl` / `state_5.sqlite` newest row per cwd, and protect anything younger than the grace window.

**Confidence: High** for `resume <uuid>` argv; **Medium** for `--last`.

---

## GitHub Copilot CLI

**Storage:** `~/.copilot/` (config dir, overridable). Session history: `~/.copilot/session-state/<uuid>/` with `events.jsonl`, `checkpoints/`, `rewind-snapshots/`, `session.db`. Cross-session SQLite: `~/.copilot/session-store.db` (`sessions`, `turns`, `session_files`). Session IDs are UUIDs.

**Process detection**
- Binaries: `copilot` (Homebrew Cask `copilot-cli`, e.g. `/opt/homebrew/Caskroom/copilot-cli/<ver>/copilot`); inner binary `github-copilot` when bundled via agency (`~/.copilot-cli/<ver>/copilot`).
- argv patterns:
  - `copilot --resume=<id>` / `copilot --resume <id>` → **exact session ID** (High)
  - `copilot --continue` / `-c` → most recent session (session picker logic; resolves to most recent by activity)
  - `copilot --resume` (no arg) → interactive picker (ambiguous)
  - `copilot` (no args) → new session

**Active-session state files / markers (strongest of all six agents)**
- **Lock files:** `~/.copilot/session-state/<uuid>/inuse.<pid>.lock` — file content is the owning PID; a live lock = session currently held open ("session in use by another CLI or application" warning). This is the canonical "currently open" marker.
- **Staleness caveat:** stale `inuse.*.lock` files accumulate on SIGKILL/crash (issues #3086, #3255, #2609). **Must** validate: (a) PID is alive, and (b) `ps -p <pid> -o command=` actually looks like the Copilot CLI (PIDs get reused). Pattern from `copilot-ide` `src/main/sessionLocks.ts`.
- `events.jsonl` first/session records include `"alreadyInUse": true/false`; `session-store.db` `sessions.updated_at` for recency.

**Recipe (running process → session id)**
1. Scan for binaries `copilot` / `github-copilot`; parse argv for `--resume`/`-s` → exact id. **High**.
2. Enumerate `~/.copilot/session-state/<uuid>/inuse.<pid>.lock`; keep locks whose PID is live AND whose command matches a copilot binary → those session UUIDs are open. **High** (this is the designed lock, modulo stale-lock cleanup).
3. `--continue`/`-c`: protect the most-recent session per cwd (from `session-store.db` or events.jsonl mtime). **Medium**.

**Confidence: High.**

---

## Pi

**Storage:** `~/.pi/agent/sessions/--<encoded-cwd>--/<timestamp>_<uuid>.jsonl` (encoded-cwd = absolute path with `/` → `-`). First JSONL line is a session header: `{"type":"session","version":3,"id":"<uuid>","timestamp":"...","cwd":"/path"}` (+ optional `parentSession`). Sessions are trees inside the file (branching via `parentId`); the "active leaf" is in-memory, not durable. `~/.pi/agent/trust.json` is unrelated. Session id = uuid.

**Process detection**
- Binary: `pi` (pi.dev Node-based CLI).
- argv patterns:
  - `pi --session <id|path>` / `pi --fork <id|path>` → **exact session id** (High)
  - `pi -c` / `--continue` → most recent session for cwd
  - `pi -r` / `--resume` → interactive picker (ambiguous)
  - `pi --session <file-path>` → the argv value may be a full path rather than an id — resolve to the header id.
  - `pi` (no args) → new session.

**Active-session state files / markers**
- **None.** No lock file, no roster, no "current session" pointer. Pi's session-manager holds the active file/id only in memory (`packages/coding-agent/src/core/session-manager.ts`). Nothing durable distinguishes "open now" from "closed".

**Recipe (running process → session id)**
1. Scan for binary `pi`; parse `--session`/`--fork` value → exact id (or resolve path → header id). **High**.
2. For `-c`/`--continue`: cwd → encoded dir `--<cwd>--` → newest `.jsonl` by mtime. **Medium**.
3. Fallback: no durable liveness signal, so rely on recency (mtime) + grace window, and lean on the global fail-safe.

**Confidence: High** for explicit `--session`; **Medium** overall (no lock/roster marker exists).

---

## Cursor

**Storage:** `~/Library/Application Support/Cursor/` (macOS) or `~/.config/Cursor/` (Linux), with `User/globalStorage/state.vscdb` (app-wide; all composer content in `cursorDiskKV` table) and `User/workspaceStorage/<hash>/state.vscdb` per workspace (registry + UI state) plus sibling `workspace.json` (`{"folder":"file:///path"}` → the workspace's cwd). Legacy global `~/.cursor/state.vscdb` and `~/.cursor/ai-code-tracking.db` also exist (omnivue ADAPTERS.md). Composer = chat/agent session, UUID id. `~/.cursor/projects/<uuid>/*.jsonl` hold agentic transcript JSONL (omnivue also reads these).

**Process detection**
- Binary: `Cursor` (Electron app; GUI, not normally launched with a composer id). CLI wrapper `cursor` exists; `cursor --composer <id>` opens a specific composer (omnivue's resume command).
- A running GUI process's *argv* is not a reliable session pointer (it typically has the app path; the opened folder may appear after `--` / as an argument only for `cursor <dir>` invocations). Treat argv as a weak signal; use state DBs instead.

**Active-session state files / markers**
- **Strongest durable signal — per-workspace DB** `workspaceStorage/<hash>/state.vscdb`, table `ItemTable`, key `composer.composerData`:
  - `allComposers[]` — every composer in the workspace with `composerId`, `name`, `lastUpdatedAt`, `isArchived`, `isDraft`, `unifiedMode` ("agent"), `createdOnBranch`, etc.
  - **`selectedComposerIds`** and **`lastFocusedComposerIds`** — the currently open / most recently focused composer(s) in that workspace's UI. **This is the "current session" marker.**
- **Global DB** `globalStorage/state.vscdb`, table `cursorDiskKV`, keys `composerData:<id>` (full composer payload incl. `status` `"completed"|"aborted"|…`, `lastUpdatedAt`) and `bubbleId:<id>:<bid>` (messages). `workbench.backgroundComposer.persistentData` holds `lastOpenedBcIds` (background-agent composer ids).
- Cursor 3.0+ central index: global `ItemTable` key `composer.composerHeaders` (per-chat entries tagged with `workspaceIdentifier`).

**Recipe (running process → session id)**
1. Find live `Cursor` processes; map each open window/workspace to `workspaceStorage/<hash>/workspace.json` folder. Practical approximation on macOS: read the running app's argv/AppleScript window titles, or simply consider *all* workspaces whose DB has `selectedComposerIds`/`lastFocusedComposerIds` non-empty while a `Cursor` process runs.
2. For each workspace DB: protect `selectedComposerIds` + `lastFocusedComposerIds` (join against global `composerData:<id>` for status/lastUpdatedAt). Also protect any composer whose `status` is neither `completed` nor `aborted` (still running). **Medium–High.**
3. Background agents: protect `lastOpenedBcIds` from `workbench.backgroundComposer.persistentData`.
4. Fallback: newest `composerData:<id>` by `lastUpdatedAt` per workspace, within the grace window.

**Confidence: Medium** — GUI app; "focused composer" markers are the designed signal but require joining workspace DB → global DB and don't distinguish windows perfectly. Recency + status are the safety net.

---

## Ambiguity and fail-safe

**Default policy: protect rather than delete.**

Concrete rules for agent-sweeper:

1. **Process argv wins.** If a live process for an agent carries a session id in its argv (`opencode -s`, `claude -r`, `codex resume`, `pi --session`, `copilot --resume`, `cursor --composer`), that session is open → protect, no matter what state files say.
2. **Durable "in use" markers protect.** Copilot `inuse.<pid>.lock` (with PID liveness + command-shape check), Claude Code `~/.claude/daemon/roster.json` + `jobs/*/state.json` live entries, Cursor `selectedComposerIds`/`lastFocusedComposerIds` + non-terminal composer `status`.
3. **"Continue/last" ambiguity.** A running process launched with `-c`/`--continue`/`resume --last` is about to adopt the *most recent* session for its cwd. When that signal is present (or when argv is bare), protect the most-recent session per (agent, project-dir). This over-protects a little but prevents deleting a session the user is one keystroke away from resuming.
4. **Recency grace window.** Any session whose `lastUpdatedAt`/file mtime is inside a configurable grace period (e.g. 24 h) is presumed live unless proven closed by a durable marker. This covers Pi (no marker at all), interactive Claude Code, and OpenCode continue-cases.
5. **Ambiguous pickers.** If a live process is showing an interactive picker (`claude -r` no-arg, `codex resume` no-arg, `copilot --resume` no-arg, `pi -r`), protect the *most recent* candidates it could select (at minimum the newest per dir) — delete nothing it might be looking at.
6. **Garbage-collect conservatively.** Only delete sessions that (a) are not referenced by any live process argv for that agent, (b) have no live lock/roster/focus marker, and (c) are older than the grace window. Ship a `--dry-run` that prints the protect-list, and require a second explicit flag (e.g. `--yes`/`--force`) to actually remove.

**Rationale:** two of six agents (Pi; interactive Claude Code) have *no* durable active marker, and stale markers are a known problem elsewhere (Copilot stale `inuse.*.lock`). The cost of over-protection (a session survives one more sweep) is far lower than the cost of deleting a live session's history.
