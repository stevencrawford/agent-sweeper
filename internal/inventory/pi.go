package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// scanPi enumerates sessions from sessions/<cwd-slug>/<ts>_<id>.jsonl. Every
// file is one self-contained session with no side artifacts; the header line
// carries the real cwd when present.
func scanPi(root string, _ time.Time) ([]model.Session, error) {
	files, err := filepath.Glob(filepath.Join(root, "sessions", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sessions := make([]model.Session, 0, len(files))
	for _, file := range files {
		base := filepath.Base(file)
		id := strings.TrimSuffix(base, ".jsonl")
		if i := strings.LastIndex(id, "_"); i >= 0 {
			id = id[i+1:]
		}
		fi, err := os.Stat(file)
		if err != nil {
			continue
		}
		cwd := piHeaderString(file, "cwd")
		s := model.Session{
			ID:           id,
			Title:        piHeaderString(file, "title"),
			CWD:          cwd,
			Label:        cwd,
			Repo:         deriveRepo(cwd),
			LastActivity: fi.ModTime(),
			SizeBytes:    fi.Size(),
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// piHeaderString reads a string field from the session file's header line.
func piHeaderString(file, field string) string {
	if v, ok := firstLineJSON(file)[field].(string); ok {
		return v
	}
	return ""
}

// piPlan deletes the single session JSONL file — the file is the session.
func piPlan(root string, s *model.Session) *engine.SessionPlan {
	matches := glob(filepath.Join(root, "sessions", "*", "*_"+s.ID+".jsonl"))
	return &engine.SessionPlan{
		ID: s.ID, Agent: "Pi", Title: s.Title,
		Actions: removeActions(engine.RemoveFile, matches, exists, statBytes),
	}
}
