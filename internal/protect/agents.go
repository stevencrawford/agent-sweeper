package protect

import (
	"regexp"
	"slices"
	"strings"
)

// resumeArgv patterns per agent, from research 3. Each exact-id pattern has one
// capture group naming the session a live process holds.

var (
	uuidPATTERN = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`
	ulidPATTERN = `[0-9A-HJKMNP-TV-Z]{26}`
)

// agentSpec describes how processes of one agent are recognized and how a
// session id and a tentative resume are read from their command line.
type agentSpec struct {
	name string
	// binaries are the executable base names that identify a process as this
	// agent.
	binaries []string
	// ids are regexes, each with one session-id capture group: a session that
	// a live process currently runs.
	ids []*regexp.Regexp
	// resume are whole command-line tokens that mark a process which has not
	// pinned an exact id yet and may adopt the most-recent session for its
	// directory next.
	resume []string
	// bareIsResume treats a process with no session id as a tentative resume
	// (a fresh launch may still adopt the most recent session for its cwd).
	bareIsResume bool
}

var agentSpecs = []agentSpec{
	{
		name:     "OpenCode",
		binaries: []string{"opencode"},
		ids: []*regexp.Regexp{
			regexp.MustCompile(`(?:\s|^)(?:-s|--session)[=\s]+(ses_[A-Za-z0-9]+)`),
		},
		resume:       []string{"-c", "--continue"},
		bareIsResume: true,
	},
	{
		name:         "Copilot",
		binaries:     []string{"copilot", "github-copilot"},
		ids:          []*regexp.Regexp{regexp.MustCompile(`--resume[=\s]+(` + uuidPATTERN + `)`)},
		resume:       []string{"-c", "--continue"},
		bareIsResume: true,
	},
	{
		name:         "Claude Code",
		binaries:     []string{"claude"},
		ids:          []*regexp.Regexp{regexp.MustCompile(`(?:-r|--resume)[=\s]+(` + ulidPATTERN + `)`)},
		resume:       []string{"-c", "--continue"},
		bareIsResume: true,
	},
	{
		name:     "Codex",
		binaries: []string{"codex"},
		ids: []*regexp.Regexp{
			regexp.MustCompile(`resume[=\s]+(` + uuidPATTERN + `)`),
		},
		resume:       []string{"--last"},
		bareIsResume: true,
	},
	{
		name:     "Pi",
		binaries: []string{"pi"},
		ids: []*regexp.Regexp{
			regexp.MustCompile(`(?:--session|--fork)[=\s]+(` + uuidPATTERN + `)`),
		},
		resume:       []string{"-c", "--continue"},
		bareIsResume: true,
	},
	{
		name:         "Cursor",
		binaries:     []string{"cursor", "Cursor"},
		ids:          []*regexp.Regexp{regexp.MustCompile(`--composer[=\s]+(` + uuidPATTERN + `)`)},
		bareIsResume: false,
	},
}

// specFor returns the spec for an agent name, or false when the name is not a
// supported agent.
func specFor(name string) (agentSpec, bool) {
	for _, s := range agentSpecs {
		if s.name == name {
			return s, true
		}
	}
	return agentSpec{}, false
}

// argvFor scans the live processes for the agent and splits them into exact
// session ids (a live process holds the session) and tentative-resume working
// directories. running reports whether any process of the agent is alive.
func argvFor(name string, procs []Process) (exact []Mark, resumes []string, running bool) {
	spec, ok := specFor(name)
	if !ok {
		return nil, nil, false
	}
	isAgent := func(cmd string) bool {
		for f := range strings.FieldsSeq(cmd) {
			if slices.Contains(spec.binaries, baseName(f)) {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	for _, p := range procs {
		if !isAgent(p.Cmd) {
			continue
		}
		running = true
		matched := false
		for _, re := range spec.ids {
			if m := re.FindStringSubmatch(p.Cmd); len(m) >= 2 {
				exact = append(exact, Mark{ID: m[1], Reason: ReasonRunning})
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		ambiguous := spec.bareIsResume
		for _, tok := range spec.resume {
			if isToken(p.Cmd, tok) {
				ambiguous = true
				break
			}
		}
		if !ambiguous {
			continue
		}
		dir := p.CWD
		if dir == "" {
			dir = procCWD(p)
		}
		if !seen[dir] {
			seen[dir] = true
			resumes = append(resumes, dir)
		}
	}
	return exact, resumes, running
}

// isToken reports whether cmd contains tok as a whole white-space-delimited
// token, so "-c" never matches inside "--continue" or a path.
func isToken(cmd, tok string) bool {
	for f := range strings.FieldsSeq(cmd) {
		if f == tok {
			return true
		}
	}
	return false
}
