package protect

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// gatherMarkers reads the agent's durable "in use" markers at scan time:
// Copilot's per-session lock files, Claude's background-session roster, and
// Cursor's focused-composer state (only while a Cursor process is running).
// Every read is best-effort; a missing store degrades to no markers.
func gatherMarkers(a *model.Agent, running bool) []Mark {
	switch a.Name {
	case "Copilot":
		return copilotLocks(a.DataRoot)
	case "Claude Code":
		return claudeRoster(a.DataRoot)
	case "Cursor":
		if !running {
			return nil
		}
		return cursorFocus(a.DataRoot)
	}
	return nil
}

// copilotLocks returns every session whose `inuse.<pid>.lock` is held by a
// live copilot process. Lock files accumulate on crashes (research 3), so the
// owning pid is validated: alive AND shaped like a copilot binary.
func copilotLocks(dataRoot string) []Mark {
	pattern := filepath.Join(expandHome(dataRoot), "session-state", "*", "inuse.*.lock")
	lockFiles, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var marks []Mark
	for _, lock := range lockFiles {
		pidStr, err := os.ReadFile(lock)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidStr)))
		if err != nil || !copilotProcessAlive(pid) {
			continue
		}
		id := filepath.Base(filepath.Dir(lock))
		marks = append(marks, Mark{ID: id, Reason: ReasonLock})
	}
	return marks
}

// copilotProcessAlive reports whether pid names a live process whose command
// line belongs to the copilot CLI, guarding against reused pids.
func copilotProcessAlive(pid int) bool {
	// #nosec G204 -- pid is parsed from an int, never user input; ps is a
	// fixed binary with fixed arguments.
	cmd, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil || len(cmd) == 0 {
		return false
	}
	base := baseName(string(cmd))
	return base == "copilot" || base == "github-copilot"
}

// claudeRoster returns the session ids Claude lists as live: the daemon roster
// and per-job state files. Short ids recorded there may not exactly equal full
// transcript ids; unmatched entries simply protect nothing.
func claudeRoster(dataRoot string) []Mark {
	root := expandHome(dataRoot)
	var marks []Mark
	seen := map[string]bool{}
	addMark := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			marks = append(marks, Mark{ID: id, Reason: ReasonRoster})
		}
	}

	rosterPath := filepath.Join(root, "daemon", "roster.json")
	if data, err := os.ReadFile(rosterPath); err == nil {
		var entries []struct {
			SessionID string `json:"sessionId"`
			ID        string `json:"id"`
		}
		if json.Unmarshal(data, &entries) == nil {
			for _, e := range entries {
				if e.SessionID != "" {
					addMark(e.SessionID)
				} else if e.ID != "" {
					addMark(e.ID)
				}
			}
		}
	}
	jobs, err := filepath.Glob(filepath.Join(root, "jobs", "*", "state.json"))
	if err != nil {
		jobs = nil
	}
	for _, job := range jobs {
		if data, err := os.ReadFile(job); err == nil {
			var st struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if json.Unmarshal(data, &st) == nil {
				addMark(st.ID)
			}
		}
	}
	return marks
}

// cursorFocus returns the composer ids that Cursor has open in any workspace:
// selectedComposerIds and lastFocusedComposerIds from each workspace's
// state.vscdb. Only meaningful while a Cursor process runs (guarded by the
// caller), so a closed editor never over-protects.
func cursorFocus(dataRoot string) []Mark {
	pattern := filepath.Join(expandHome(dataRoot), "User", "workspaceStorage", "*", "state.vscdb")
	stores, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var marks []Mark
	seen := map[string]bool{}
	for _, store := range stores {
		for _, id := range focusedComposers(store) {
			if !seen[id] {
				seen[id] = true
				marks = append(marks, Mark{ID: id, Reason: ReasonFocused})
			}
		}
	}
	return marks
}

// focusedComposers reads one workspace store's composer.composerData KV row and
// returns the composer ids it marks as focused. Read-only; errors yield nil.
func focusedComposers(store string) []string {
	dsn := "file:" + filepath.ToSlash(store) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil
	}
	defer db.Close()
	var raw []byte
	// #nosec G202 -- fixed table and column identifiers from the agent layout.
	err = db.QueryRow(
		"SELECT value FROM ItemTable WHERE key = ?", "composer.composerData").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return nil
	}
	var doc struct {
		Selected []string `json:"selectedComposerIds"`
		Focused  []string `json:"lastFocusedComposerIds"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, doc.Selected...), doc.Focused...) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// expandHome expands a leading ~ to the user's home directory, returning p
// unchanged when the home directory cannot be resolved.
func expandHome(p string) string {
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
