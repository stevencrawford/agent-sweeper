// Package paths resolves each coding agent's real session-store data root on
// the current OS. Every agent has a documented root and a documented set of
// environment overrides (research 04 / platform-paths.md); the resolution
// rules below follow that table so detection works on macOS, Linux, and
// Windows.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// expand resolves a leading ~ to the user's home directory. A path that
// cannot be resolved is returned unchanged.
func expand(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/"))
}

// DataRoot returns the real session-store data root for the named agent, or ""
// when the name is not a supported agent.
func DataRoot(name string) string {
	switch name {
	case "OpenCode":
		return opencode()
	case "Copilot":
		return copilot()
	case "Claude Code":
		return claude()
	case "Codex":
		return codex()
	case "Pi":
		return pi()
	case "Cursor":
		return cursorAgentHome()
	}
	return ""
}

// OpenCode is pure XDG on every platform: $XDG_DATA_HOME/opencode, else
// ~/.local/share/opencode. macOS and Windows do NOT special-case it.
func opencode() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "opencode")
	}
	return expand("~/.local/share/opencode")
}

// Copilot lives in a home dotdir with the whole root overridable by
// COPILOT_HOME; the cache home is separate and never a session store.
func copilot() string {
	if v := os.Getenv("COPILOT_HOME"); v != "" {
		return v
	}
	return expand("~/.copilot")
}

// Claude Code lives in a home dotdir, overridable by CLAUDE_CONFIG_DIR.
func claude() string {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	return expand("~/.claude")
}

// Codex lives in a home dotdir, overridable by CODEX_HOME.
func codex() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	return expand("~/.codex")
}

// Pi lives under a home dotdir, overridable by PI_CODING_AGENT_DIR.
func pi() string {
	if v := os.Getenv("PI_CODING_AGENT_DIR"); v != "" {
		return v
	}
	return expand("~/.pi/agent")
}

// CursorAgentHome is the ~/.cursor agent home that holds the per-project
// session transcripts on every platform.
func cursorAgentHome() string {
	if v := os.Getenv("CURSOR_CONFIG_DIR"); v != "" {
		return v
	}
	return expand("~/.cursor")
}

// CursorAppData is the VS Code-style app data root that holds state.vscdb.
// macOS uses ~/Library/Application Support, Linux ~/.config, Windows %APPDATA%.
func CursorAppData() string {
	switch runtime.GOOS {
	case "darwin":
		return expand("~/Library/Application Support/Cursor")
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "Cursor")
		}
		return expand("~\\AppData\\Roaming\\Cursor")
	default:
		return expand("~/.config/Cursor")
	}
}

// CursorGlobalState is the global storage.state.vscdb path.
func CursorGlobalState() string {
	return filepath.Join(CursorAppData(), "User", "globalStorage", "state.vscdb")
}
