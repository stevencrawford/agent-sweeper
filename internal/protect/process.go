package protect

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Process is a live process observed at scan time.
type Process struct {
	PID int
	// Cmd is the full command line as reported by the system process list.
	Cmd string
	// CWD is the working directory, resolved best-effort; empty when
	// unavailable.
	CWD string
}

// snapshot lists every live process with its command line. On platforms
// without ps, or when ps fails, it returns nil — the caller degrades to the
// marker and grace-window signals alone.
func snapshot() []Process {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil
	}
	var procs []Process
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidText, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil {
			continue
		}
		procs = append(procs, Process{PID: pid, Cmd: strings.TrimSpace(rest)})
	}
	return procs
}

// baseName returns the executable name of a process command line: the leading
// token, quotes stripped, path removed. A process whose argv0 was not reported
// falls back to the first plausible token.
func baseName(cmd string) string {
	argv0, _, _ := strings.Cut(cmd, " ")
	argv0 = strings.Trim(argv0, `"'`)
	if argv0 == "" {
		return ""
	}
	return filepath.Base(argv0)
}

// procCWD resolves a process's working directory via lsof, best-effort. It is
// only called for processes that already match an agent, so the cost is
// bounded by the number of live agent processes.
func procCWD(p Process) string {
	// #nosec G204 -- pid is parsed from an int, never user input; lsof is a
	// fixed binary with fixed arguments.
	out, err := exec.Command("lsof", "-a", "-d", "cwd", "-p", strconv.Itoa(p.PID), "-Fn").Output()
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}
