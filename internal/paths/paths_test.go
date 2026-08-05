package paths

import (
	"path/filepath"
	"testing"
)

func TestDataRootRespectsEnvOverrides(t *testing.T) {
	cases := []struct {
		name string
		env  string
		key  string
		want string
	}{
		{name: "OpenCode", env: "XDG_DATA_HOME", key: "/tmp/xdg", want: "/tmp/xdg/opencode"},
		{name: "Copilot", env: "COPILOT_HOME", key: "/tmp/cp", want: "/tmp/cp"},
		{name: "Claude Code", env: "CLAUDE_CONFIG_DIR", key: "/tmp/claude", want: "/tmp/claude"},
		{name: "Codex", env: "CODEX_HOME", key: "/tmp/codex", want: "/tmp/codex"},
		{name: "Pi", env: "PI_CODING_AGENT_DIR", key: "/tmp/pi", want: "/tmp/pi"},
		{name: "Cursor", env: "CURSOR_CONFIG_DIR", key: "/tmp/cursor", want: "/tmp/cursor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, tc.key)
			if got := DataRoot(tc.name); got != filepath.FromSlash(tc.want) {
				t.Fatalf("DataRoot(%q) with %s=%s = %q, want %q", tc.name, tc.env, tc.key, got, tc.want)
			}
		})
	}
}

func TestDataRootUnknownAgent(t *testing.T) {
	if got := DataRoot("NotAnAgent"); got != "" {
		t.Fatalf("unknown agent should resolve to empty, got %q", got)
	}
}
