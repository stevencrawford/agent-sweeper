package inventory

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// codexThread is one row of the codex state SQLite threads index.
type codexThread struct {
	cwd, title string
}

// scanCodex enumerates sessions from the date-partitioned rollout files
// sessions/YYYY/MM/DD/rollout-*.jsonl (.jsonl.zst when cold-compressed),
// pairing each with cwd/title from the threads index and last-activity from
// the session index when available.
func scanCodex(root string, _ time.Time) ([]model.Session, error) {
	files, err := filepath.Glob(filepath.Join(root, "sessions", "*", "*", "*", "rollout-*.jsonl*"))
	if err != nil {
		return nil, err
	}
	threads := codexThreads(root)
	updated := codexSessionIndex(root)

	sessions := make([]model.Session, 0, len(files))
	for _, file := range files {
		id := rolloutID(file)
		fi, err := os.Stat(file)
		if err != nil {
			continue
		}
		last := fi.ModTime()
		if t, ok := parseTime(updated[id]); ok {
			last = t
		}
		th := threads[id]
		title := th.title
		if title == "" {
			title = codexHeaderValue(file, "title")
		}
		s := model.Session{
			ID:           id,
			Title:        title,
			CWD:          th.cwd,
			Label:        th.cwd,
			Repo:         deriveRepo(th.cwd),
			LastActivity: last,
			SizeBytes:    fi.Size(),
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// codexThreads reads the cwd/title index from the first readable state_*.sqlite
// threads table. The SQLite is a fast-list index only; a missing or malformed
// store degrades to file-mtime/header detection.
func codexThreads(root string) map[string]codexThread {
	out := map[string]codexThread{}
	states := glob(filepath.Join(root, "state_*.sqlite"))
	for _, st := range states {
		if !storeHasTable(st, "threads") {
			continue
		}
		db, err := openReadOnly(st)
		if err != nil {
			break
		}
		rows, err := db.Query("SELECT id, COALESCE(cwd, ''), COALESCE(title, '') FROM threads")
		if err == nil {
			for rows.Next() {
				var id, cwd, title string
				if rows.Scan(&id, &cwd, &title) == nil {
					out[id] = codexThread{cwd: cwd, title: title}
				}
			}
			rows.Close()
		}
		db.Close()
		break
	}
	return out
}

// codexSessionIndex maps session id to its updated_at from session_index.jsonl.
func codexSessionIndex(root string) map[string]string {
	updated := map[string]string{}
	f, err := os.Open(filepath.Join(root, "session_index.jsonl"))
	if err != nil {
		return updated
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec struct {
			ID      string `json:"id"`
			Updated string `json:"updated_at"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.ID != "" {
			updated[rec.ID] = rec.Updated
		}
	}
	return updated
}

// codexHeaderValue reads a string field from the rollout's first line.
func codexHeaderValue(file, field string) string {
	if v, ok := firstLineJSON(file)[field].(string); ok {
		return v
	}
	return ""
}

// rolloutID extracts the session id from a rollout file name. Codex names these
// rollout-<timestamp>-<uuid>.jsonl (or .jsonl.zst when cold-compressed); the
// session id is the trailing UUID, so we take the last 36 characters and only
// fall back to the last dash-delimited segment for non-UUID names.
func rolloutID(file string) string {
	base := filepath.Base(file)
	base = strings.TrimSuffix(base, ".zst")
	id := strings.TrimSuffix(base, ".jsonl")
	if len(id) >= 36 {
		if cand := id[len(id)-36:]; uuidRe.MatchString(cand) {
			return cand
		}
	}
	if i := strings.LastIndex(id, "-"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// uuidRe matches a canonical 8-4-4-4-12 UUID.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// codexPlan deletes the rollout file (and its archived copy), shell snapshots,
// the index line, then the threads row — the Codex-equivalent of `codex delete`.
func codexPlan(root string, s *model.Session) *engine.SessionPlan {
	var actions []engine.Action
	matches := glob(filepath.Join(root, "sessions", "*", "*", "*", "rollout-*"+s.ID+".jsonl*"))
	if len(matches) == 0 {
		matches = glob(filepath.Join(root, "archived_sessions", "rollout-*"+s.ID+".json*"))
	}
	actions = append(actions, removeActions(engine.RemoveFile, matches, exists, statBytes)...)

	snapshots := filepath.Join(root, "shell_snapshots", s.ID+".*")
	if ms := glob(snapshots); len(ms) > 0 {
		actions = append(actions, engine.Action{Kind: engine.RemoveGlob, Path: snapshots, Bytes: globBytes(snapshots)})
	}
	if idx := filepath.Join(root, "session_index.jsonl"); exists(idx) {
		actions = append(actions, engine.Action{Kind: engine.DropJSONLLines, Path: idx, IDs: []string{s.ID}})
	}
	for _, st := range stateStores(root) {
		if storeHasTable(st, "threads") {
			actions = append(actions, engine.Action{
				Kind: engine.SQLDelete, Store: st,
				SQL: "DELETE FROM threads WHERE id = ?", Args: []any{s.ID},
			})
		}
	}
	return &engine.SessionPlan{ID: s.ID, Agent: "Codex", Title: s.Title, Actions: actions}
}

// stateStores lists the state_*.sqlite files under the codex root.
func stateStores(root string) []string {
	return glob(filepath.Join(root, "state_*.sqlite"))
}
