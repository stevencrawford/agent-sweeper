package inventory

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// repoCache memoizes deriveRepo per directory so repeated sessions in one
// directory spawn at most one git process.
var (
	repoCache = map[string]string{}
	repoMu    sync.Mutex
)

// enumerator describes how one agent's real store is scanned into sessions.
type enumerator struct {
	name string
	root func() string
	scan func(root string, now time.Time) ([]model.Session, error)
}

// exists reports whether path exists on disk.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// glob scans matches for a pattern, treating any glob error as "no matches".
// Session stores may have absent or malformed index globs; a best-effort match
// list degrades detection to zero sessions rather than failing a scan.
func glob(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

// globBytes sums the sizes of every regular file matching a glob, returning
// zero when the glob is absent. Used to attribute bytes to a cloud of artifacts
// (e.g. snapshots) whose count is not known ahead of time.
func globBytes(pattern string) int64 {
	var total int64
	for _, m := range glob(pattern) {
		total += statBytes(m)
	}
	return total
}

// statBytes returns the size of a regular file, or 0 when it cannot be read.
func statBytes(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return 0
	}
	return fi.Size()
}

// dirBytes returns the recursive byte size of a directory tree, or 0 when it
// cannot be read. Used only for the raw detection-time size; the reclaim
// footprint is always plan-based.
func dirBytes(path string) int64 {
	var total int64
	err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	if err != nil {
		return 0
	}
	return total
}

// firstLineJSON parses the first non-empty line of a JSONL file into a map.
// Used to pull cwd/branch/summary headers out of session transcripts.
func firstLineJSON(path string) map[string]any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var obj map[string]any
	dec := json.NewDecoder(f)
	if err := dec.Decode(&obj); err != nil {
		return nil
	}
	return obj
}

// parseTime parses an ISO-8601 timestamp with the layouts agent stores use.
func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// claudeSlug encodes a workdir path the way Claude Code names its project
// directories: every non-alphanumeric rune becomes a dash.
func claudeSlug(path string) string {
	var b strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return b.String()
}

// deriveRepo best-effort resolves a directory's git remote identity, returning
// "" when the directory is not a git repo or git is unavailable. Branch is a
// session-time recorded label only (decision 13), so this never reads HEAD.
func deriveRepo(cwd string) string {
	if cwd == "" || !exists(cwd) {
		return ""
	}
	repoMu.Lock()
	if v, ok := repoCache[cwd]; ok {
		repoMu.Unlock()
		return v
	}
	repoMu.Unlock()

	// Cheap existence check before spawning anything.
	if !exists(filepath.Join(cwd, ".git")) && !isGitWorktree(cwd) {
		repoMu.Lock()
		repoCache[cwd] = ""
		repoMu.Unlock()
		return ""
	}
	out, err := gitConfig(cwd, "remote.origin.url")
	repo := ""
	if err == nil && out != "" {
		repo = normalizeRepo(out)
	}
	repoMu.Lock()
	repoCache[cwd] = repo
	repoMu.Unlock()
	return repo
}

// isGitWorktree reports whether dir is inside a git worktree (a .git file,
// not a directory).
func isGitWorktree(cwd string) bool {
	fi, err := os.Stat(filepath.Join(cwd, ".git"))
	return err == nil && !fi.IsDir()
}

// normalizeRepo trims scheme and trailing slash from a git remote URL.
func normalizeRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	for _, prefix := range []string{"https://", "http://", "ssh://", "git@"} {
		remote = strings.TrimPrefix(remote, prefix)
	}
	if i := strings.Index(remote, ".git"); i >= 0 {
		remote = remote[:i]
	}
	remote = strings.TrimSuffix(remote, "/")
	return remote
}

// gitConfig runs `git -C dir config --get <key>` best-effort.
func gitConfig(dir, key string) (string, error) {
	// #nosec G204 -- dir comes from a local session scan, never user input;
	// git is a fixed binary with fixed arguments.
	out, err := exec.Command("git", "-C", dir, "config", "--get", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
