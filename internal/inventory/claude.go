package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/engine"
	"github.com/stevencrawford/agent-sweeper/internal/model"
)

// scanClaude enumerates sessions from the project transcript files
// projects/<slug>/<session>.jsonl. Subagent transcripts live one level deeper
// under <slug>/<uuid>/ and are not sessions in their own right. The project
// slug encodes the workdir; the header line carries the real cwd when present.
func scanClaude(root string, _ time.Time) ([]model.Session, error) {
	files, err := filepath.Glob(filepath.Join(root, "projects", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sessions := make([]model.Session, 0, len(files))
	for _, file := range files {
		slug := filepath.Base(filepath.Dir(file))
		if slug == "" {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		fi, err := os.Stat(file)
		if err != nil {
			continue
		}
		s := model.Session{
			ID:           id,
			Title:        claudeHeaderString(file, "summary"),
			CWD:          claudeHeaderString(file, "cwd"),
			Label:        claudeSlugToLabel(slug),
			Branch:       claudeHeaderString(file, "gitBranch"),
			LastActivity: fi.ModTime(),
			SizeBytes:    fi.Size() + dirBytes(filepath.Join(filepath.Dir(file), id)),
		}
		s.Repo = deriveRepo(s.CWD)
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// claudeHeaderString reads a string field from the transcript's first line.
func claudeHeaderString(file, field string) string {
	if v, ok := firstLineJSON(file)[field].(string); ok {
		return v
	}
	return ""
}

// claudeSlugToLabel is the display label for a project slug when the header
// carries no cwd. Dashes are ambiguous in an encoded path, so the slug itself
// is shown rather than a lossy decode.
func claudeSlugToLabel(slug string) string {
	if slug == "" {
		return ""
	}
	return "~/.claude/projects/" + slug
}

// claudePlan deletes the transcript, its subagent/tool-results dir, shell
// snapshots, then the index entries (sessions-index.json and history.jsonl).
func claudePlan(root string, s *model.Session) *engine.SessionPlan {
	var actions []engine.Action
	dir := ""
	if s.CWD != "" {
		dir = filepath.Join(root, "projects", claudeSlug(s.CWD))
	} else if matches := glob(filepath.Join(root, "projects", "*", s.ID+".jsonl")); len(matches) == 1 {
		dir = filepath.Dir(matches[0])
	}
	if dir != "" {
		file := filepath.Join(dir, s.ID+".jsonl")
		if exists(file) {
			actions = append(actions, engine.Action{Kind: engine.RemoveFile, Path: file, Bytes: statBytes(file)})
		}
		subdir := filepath.Join(dir, s.ID)
		if exists(subdir) {
			actions = append(actions, engine.Action{Kind: engine.RemoveTree, Path: subdir, Bytes: dirBytes(subdir)})
		}
		index := filepath.Join(dir, "sessions-index.json")
		if exists(index) {
			actions = append(actions, engine.Action{Kind: engine.DropJSONKeys, Path: index, IDs: []string{s.ID}})
		}
	}
	snapshots := filepath.Join(root, "shell-snapshots", s.ID+".*")
	if matches := glob(snapshots); len(matches) > 0 {
		actions = append(actions, engine.Action{Kind: engine.RemoveGlob, Path: snapshots, Bytes: globBytes(snapshots)})
	}
	if history := filepath.Join(root, "history.jsonl"); exists(history) {
		actions = append(actions, engine.Action{Kind: engine.DropJSONLLines, Path: history, IDs: []string{s.ID}})
	}
	return &engine.SessionPlan{ID: s.ID, Agent: "Claude Code", Title: s.Title, Actions: actions}
}
