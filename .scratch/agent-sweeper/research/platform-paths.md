# Agent session-store paths by platform

Facts doc for `agent-sweeper`. Compiled 2026-08-05.

Confidence marking:
- **confirmed** = verified against the agent's own source code and/or official docs, and (where noted) a live macOS install on this machine.
- **inferred** = cross-platform best guess from third-party tooling or analogy; needs a runtime check.

Legend: `~` = `$HOME` / `%USERPROFILE%`. "Data root" means the top-level session-store directory.

---

## OpenCode

Primary source: `anomalyco/opencode` `packages/core/src/global.ts` (uses `xdg-basedir@5.1.0`, which is **pure XDG on every platform**, no macOS Application Support / Windows APPDATA special-casing), plus official docs `opencode.ai/docs/troubleshooting/`. Confirmed on a live macOS install.

| OS | Session-store root | Env override | Side-artifact locations (relative to root) |
| --- | --- | --- | --- |
| macOS | `$XDG_DATA_HOME/opencode` else `~/.local/share/opencode` — **confirmed** (macOS does NOT use `~/Library/Application Support/opencode` for the CLI) | `XDG_DATA_HOME`; `OPENCODE_DB` (SQLite path, absolute or relative to data root); `OPENCODE_CONFIG` / `OPENCODE_CONFIG_DIR` (config file / config dir); `OPENCODE_TEST_HOME` (test-only home override) | `opencode.db` (+ `-wal`/`-shm`) — all session/message/diff/token/cost data; `log/`; `storage/` (current JSON layout: `session/`, `message/`, `part/`, `session_diff/`, `project/`); `snapshot/<projectID>/<hash>/` — git bare repos for file rewind; `tool-output/`; `repos/`; `auth.json`. Legacy layout (pre-migration): `project/` |
| Linux | `$XDG_DATA_HOME/opencode` else `~/.local/share/opencode` — **confirmed** | same as macOS | same as macOS |
| Windows | `$XDG_DATA_HOME\opencode` else `%USERPROFILE%\.local\share\opencode` — **confirmed** (XDG path, NOT `%APPDATA%`/`%LOCALAPPDATA%`; docs literally say paste `%USERPROFILE%\.local\share\opencode` in WIN+R) | same as macOS | same as macOS (e.g. `%USERPROFILE%\.local\share\opencode\opencode.db`, `...\snapshot\`, `...\storage\`) |

Notes:
- The whole CLI path set is XDG: config `$XDG_CONFIG_HOME/opencode` (`~/.config/opencode/opencode.json`), state `$XDG_STATE_HOME/opencode`, cache `$XDG_CACHE_HOME/opencode` — on macOS and Windows too.
- The **OpenCode Desktop app (Tauri)** is separate: it uses `~/Library/Application Support/<app-id>` (macOS) / `%APPDATA%\<app-id>` (Windows) and sets `XDG_*` vars pointing inside that dir. Session data for the CLI lives in the XDG root above, not the desktop app dir.

---

## GitHub Copilot (CLI / coding agent)

Primary source: GitHub docs "Copilot CLI configuration directory" (`docs.github.com/copilot/reference/copilot-cli-reference/cli-config-dir-reference`) and "chronicle". Confirmed on a live macOS install.

| OS | Session-store root | Env override | Side-artifact locations (relative to root) |
| --- | --- | --- | --- |
| macOS | `~/.copilot` — **confirmed** | `COPILOT_HOME` (replaces entire `~/.copilot`); `COPILOT_CACHE_HOME` (cache only; default `~/Library/Caches/copilot`) | `session-store.db` (SQLite, cross-session index/search); `session-state/<uuid>/events.jsonl` (session transcript); `session-state/<uuid>/checkpoints/` (checkpoint/plan data); `session-state/<uuid>/rewind-snapshots/` (file backups); also observed per-session `plan.md`, `files/`, `research/`, `session.db`, `workspace.yaml`; `command-history-state/`; `logs/`; `config.json`; `settings.json`; `agents/`; `skills/`; `plugins/`; `mcp-config.json`; `permissions-config.json`. Legacy (pre-0.0.342): `history-session-state/` |
| Linux | `~/.copilot` — **confirmed** | same; cache default `$XDG_CACHE_HOME/copilot` or `~/.cache/copilot` | same as macOS |
| Windows | `%USERPROFILE%\.copilot` — **confirmed** (docs show `$HOME\.copilot` on Windows; the home dotfile is portable) | same; cache default `%LOCALAPPDATA%\copilot` (NOT session data — don't scan it) | same as macOS |

Notes:
- Older CLI versions stored state under `$XDG_STATE_HOME/.copilot`; current versions auto-migrate it into `~/.copilot`. A "with `XDG_CONFIG_HOME` → `$XDG_CONFIG_HOME/copilot`" claim floats around in third-party writeups but is stale; the official docs name `~/.copilot` + `COPILOT_HOME`.
- The Windows cache dir `%LOCALAPPDATA%\copilot` must not be confused with the session store.

---

## Claude Code

Primary source: Anthropic docs "Explore the .claude directory" + "Manage sessions" (`code.claude.com/docs`). Confirmed on a live macOS install.

| OS | Session-store root | Env override | Side-artifact locations (relative to root) |
| --- | --- | --- | --- |
| macOS | `~/.claude` — **confirmed** | `CLAUDE_CONFIG_DIR` (moves the whole store); `CLAUDE_CODE_SKIP_PROMPT_HISTORY` (suppress transcript+history writes); `cleanupPeriodDays` setting (retention); `--no-session-persistence` flag | `projects/<encoded-path>/<session-uuid>.jsonl` (transcripts; project path with non-alphanumerics → `-`); `projects/<path>/<uuid>/subagents/agent-*.jsonl`; `projects/<path>/<uuid>/tool-results/` (large tool outputs); `file-history/<path>/` (pre-edit checkpoint snapshots); `sessions/` (running-session markers); `history.jsonl` (prompt history); `plans/<slug>.md` (implementation plans); `shell-snapshots/`; `todos/`; `settings.json`; global `~/.claude.json` lives at HOME root, NOT inside `~/.claude` |
| Linux | `~/.claude` — **confirmed** | same | same as macOS |
| Windows | `%USERPROFILE%\.claude` — **confirmed** (docs: "On Windows, `~/.claude` resolves to `%USERPROFILE%\.claude`"; project path encoding uses `C--Users-...` style) | same | same as macOS |

Notes:
- A 2026 third-party article claims transcripts moved under `projects/<path>/sessions/<id>.jsonl`; a live current install shows `<uuid>.jsonl` directly in the project dir plus a sibling `<uuid>/` folder for subagents/tool-results. Treat the `sessions/` subdir variant as **inferred / version-dependent**.

---

## OpenAI Codex CLI

Primary source: OpenAI developer docs (codex config-advanced, environment-variables, app/windows) + Rust source (`find_codex_home()` = `$HOME` + `.codex` unless `CODEX_HOME`). Confirmed on a live macOS install.

| OS | Session-store root | Env override | Side-artifact locations (relative to root) |
| --- | --- | --- | --- |
| macOS | `~/.codex` — **confirmed** | `CODEX_HOME` (root; must pre-exist); `CODEX_SQLITE_HOME` (SQLite state, default `CODEX_HOME`); `CODEX_INSTALL_DIR` (binary install dir, not session data) | `session_index.jsonl`; `sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl` (session transcripts); `archived_sessions/` (recent feature); `config.toml`; `auth.json`; `history.jsonl`; `logs/` (e.g. `codex-tui.log`); `edits/*.json`; `plans/*.json`; `rules/`; `skills/`; `shell_snapshots/`; `plugins/`; SQLite state files incl. `state_*.sqlite`, `goals_*.sqlite`, `logs_*.sqlite`, `memories_*.sqlite` |
| Linux | `~/.codex` — **confirmed** | same | same as macOS |
| Windows | `%USERPROFILE%\.codex` — **confirmed** (OpenAI docs: same Codex home as native CLI; "not `%APPDATA%`") | same (`CODEX_HOME=/mnt/c/Users/<user>/.codex` used to share into WSL) | same as macOS (e.g. `%USERPROFILE%\.codex\sessions\YYYY\MM\DD\rollout-*.jsonl`) |

---

## Pi (pi.dev coding agent)

Primary source: pi.dev docs + `earendil-works/pi` source (`packages/coding-agent/src/config.ts`: `getAgentDir() = join(os.homedir(), ".pi", "agent")`; `session-manager.ts`). Confirmed on a live macOS install.

| OS | Session-store root | Env override | Side-artifact locations (relative to root) |
| --- | --- | --- | --- |
| macOS | `~/.pi/agent` — **confirmed** | `PI_CODING_AGENT_DIR` (config dir, default `~/.pi/agent`); `PI_CODING_AGENT_SESSION_DIR` (session dir, overridden by `--session-dir`); `--session-dir` CLI flag | `sessions/--<encoded-cwd>--/<timestamp>_<uuid>.jsonl` (JSONL, one per session; cwd encoded by replacing `/`, `\`, `:` with `-`); `auth.json`; `settings.json`; `models-store.json`; `bin/`; `<app>-debug.log` |
| Linux | `~/.pi/agent` — **confirmed** | same | same as macOS |
| Windows | `%USERPROFILE%\.pi\agent` — **inferred** (source uses `os.homedir()`, no Windows branch; needs a Windows runtime check) | same | same as macOS (paths joined with `\`) |

---

## Cursor

Primary sources: cursor.com CLI docs (`cli-config.json` table), cursor-helper DeepWiki, plus several independent transcript scanners (GrayCodeAI/trace, benvenker/agent-session-search, jlbgit/Curios) that all agree on `~/.cursor/projects/<slug>/agent-transcripts/`. Confirmed on a live macOS install.

Cursor has **two roots** — a VS Code-style app data dir, and a `~/.cursor` agent home.

| OS | Session-store root (app data) | Session-store root (agent home) | Env override | Side-artifact locations |
| --- | --- | --- | --- | --- |
| macOS | `~/Library/Application Support/Cursor` — **confirmed** | `~/.cursor` — **confirmed** | `CURSOR_CONFIG_DIR` (documented override for the `~/.cursor` CLI-config home); `--user-data-dir` (Electron flag, **inferred** for app data) | App data: `User/globalStorage/state.vscdb` (global composer/agent DB; `cursorDiskKV` table = conversation content + `composer.composerHeaders`); `User/workspaceStorage/<hash>/state.vscdb` (per-workspace DB; `ItemTable`); `User/History/` (local file history). Agent home: `projects/<slug>/agent-transcripts/*.jsonl` (session transcripts); `projects/<slug>/agent-tools/`; `ai-tracking/ai-code-tracking.db` (SQLite summaries/model/cost/tokens — NOTE: under `ai-tracking/`, not at root); `chats/` (per-chat SQLite `store.db`); `prompt_history.json`; `cli-config.json`; `argv.json`; `agents/`; `skills-cursor/`; `plugins/` |
| Linux | `~/.config/Cursor` — **confirmed** (DeepWiki + multiple sources) | `~/.cursor` — **inferred** (cross-platform by analogy; needs Linux runtime check) | `CURSOR_CONFIG_DIR`; `XDG_CONFIG_HOME` (Linux/BSD: cli-config read from `$XDG_CONFIG_HOME/cursor/cli-config.json`) | same layout as macOS |
| Windows | `%APPDATA%\Cursor` — **confirmed** (multiple sources: `%AppData%\Roaming\Cursor\User\workspaceStorage\...\state.vscdb`) | `%USERPROFILE%\.cursor` — **inferred** (CLI docs confirm cli-config at `$env:USERPROFILE\.cursor\cli-config.json`; the `projects/` transcript root is cross-platform by analogy) | `CURSOR_CONFIG_DIR`; `--user-data-dir` | same layout as macOS |

Notes:
- `state.vscdb` does **not** live under `~/.cursor` — it lives in the app data root under `User/globalStorage/` (global) and `User/workspaceStorage/<hash>/` (per workspace), on every OS.
- The per-project agent transcripts (`projects/<slug>/agent-transcripts/`) are in the `~/.cursor` agent home on **all** platforms.
- WAL caveat: `.vscdb` writes are WAL-mode; copy `-wal`/`-shm` alongside the db for a consistent read.

---

## Gaps / discrepancies vs omnivue reference

- omnivue is Unix-default-only: `internal/ingest/registry.go` registers `~/.local/share/opencode`, `~/.copilot`, `~/.cursor`, `~/.pi/agent/sessions`, `~/.claude`, `~/.codex` with no per-OS handling; `internal/xdg/xdg.go` only provides `StateHome`. Windows/macOS-specific roots (Claude `%USERPROFILE%\.claude`, Codex `%USERPROFILE%\.codex`, Cursor `%APPDATA%\Cursor`, opencode `%USERPROFILE%\.local\share\opencode`) are **not** modeled.
- omnivue `docs/ADAPTERS.md` says Cursor source is `~/.cursor/state.vscdb`. **Wrong** — `state.vscdb` is at `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` (macOS), `~/.config/Cursor/User/globalStorage/state.vscdb` (Linux), `%APPDATA%\Cursor\User\globalStorage\state.vscdb` (Windows). omnivue's actual adapter code does probe those three locations (`internal/ingest/cursor/cursor_test.go`).
- omnivue `docs/ADAPTERS.md` says `~/.cursor/ai-code-tracking.db`; real location is `~/.cursor/ai-tracking/ai-code-tracking.db`.
- omnivue `docs/ADAPTERS.md` says Cursor transcripts are `~/.cursor/projects/<uuid>/*.jsonl`; real layout is `~/.cursor/projects/<project-slug>/agent-transcripts/*.jsonl` (transcripts live one level deeper).
- omnivue `docs/ADAPTERS.md` says Pi sessions are `~/.pi/agent/sessions/*.jsonl`; real layout nests per-cwd: `~/.pi/agent/sessions/--<encoded-cwd>--/<timestamp>_<uuid>.jsonl`.
- omnivue opencode root `~/.local/share/opencode` is correct for macOS/Linux/Windows (XDG everywhere) but omnivue has no `XDG_DATA_HOME`/`OPENCODE_DB` awareness.
- omnivue Codex paths `session_index.jsonl`, `edits/*.json`, `plans/*.json` — `session_index.jsonl` confirmed on a live install; `edits/` and `plans/` were **empty/absent** there, and the current Codex transcript source is `sessions/YYYY/MM/DD/rollout-*.jsonl`.

## Key surprises

1. **OpenCode is pure-XDG on every OS**, including macOS (`~/.local/share/opencode`, not `~/Library/Application Support/opencode`) and Windows (`%USERPROFILE%\.local\share\opencode`, not `%APPDATA%`).
2. **Cursor's per-OS split**: `state.vscdb` lives in the app-data root (`~/Library/Application Support/Cursor` / `~/.config/Cursor` / `%APPDATA%\Cursor`), while the agent transcripts live in `~/.cursor/projects/` on all platforms.
3. **Copilot and Codex keep the Unix dotfile on Windows**: `%USERPROFILE%\.copilot` and `%USERPROFILE%\.codex` (no AppData). Only the Copilot *cache* uses `%LOCALAPPDATA%`.
