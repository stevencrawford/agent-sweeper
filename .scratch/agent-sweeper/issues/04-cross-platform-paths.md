# Ticket: Cross-platform session-store paths

Type: research
Status: resolved
Blocked by:

## Question

For each of the six agents, what are the exact session-store locations on **macOS, Linux, and Windows** (XDG vs `%APPDATA%`)?

- Default paths per OS
- Env overrides (`XDG_DATA_HOME`, `CLAUDE_CONFIG_DIR`, etc.)
- Where the per-session side artifacts live relative to the store root

This feeds path resolution in the sweep/stats code and the deletion-semantics design.

**Research** — resolve with a `/research` subagent. Write the findings as a facts doc under `research/platform-paths.md` and link it here.

## Answer

Resolved. Full facts doc: [`research/platform-paths.md`](../research/platform-paths.md). Highlights:

- **OpenCode**: pure XDG on **every** OS — `$XDG_DATA_HOME/opencode` else `~/.local/share/opencode` on macOS AND Linux, `%USERPROFILE%\.local\share\opencode` on Windows. **Not** `~/Library/Application Support`, **not** `%APPDATA%`. Overrides `XDG_DATA_HOME`, `OPENCODE_DB`.
- **Copilot**: `~/.copilot` dotfile on all OSes (Windows `%USERPROFILE%\.copilot`); `COPILOT_HOME` override; only cache is platform-native (`%LOCALAPPDATA%\copilot`).
- **Claude Code**: `~/.claude` (Windows `%USERPROFILE%\.claude`); `CLAUDE_CONFIG_DIR` override.
- **Codex**: `~/.codex` (Windows `%USERPROFILE%\.codex`); `CODEX_HOME`, `CODEX_SQLITE_HOME` overrides.
- **Pi**: `~/.pi/agent` (Windows inferred `%USERPROFILE%\.pi\agent`); `PI_CODING_AGENT_DIR` / `PI_CODING_AGENT_SESSION_DIR` overrides.
- **Cursor**: **two roots** — `state.vscdb` under the app-data root (`~/Library/Application Support/Cursor`, `~/.config/Cursor`, `%APPDATA%\Cursor`) under `User/globalStorage/` + `User/workspaceStorage/`, while transcripts live in `~/.cursor/projects/<slug>/agent-transcripts/` on all platforms. `CURSOR_CONFIG_DIR` override.
- Note: omnivue's own `~/.cursor/state.vscdb` claim is wrong (real path under app data); `ai-code-tracking.db` is under `~/.cursor/ai-tracking/`.
