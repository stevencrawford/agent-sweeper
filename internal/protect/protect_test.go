package protect

import (
	"testing"
	"time"

	"github.com/stevencrawford/agent-sweeper/internal/model"
)

func sessions() []model.Session {
	now := time.Now()
	day := 24 * time.Hour
	return []model.Session{
		{ID: "a", CWD: "/x", LastActivity: now.Add(-2 * day)},
		{ID: "b", CWD: "/x", LastActivity: now.Add(-1 * day)},
		{ID: "c", CWD: "/y", LastActivity: now.Add(-1 * time.Hour)},
	}
}

func TestDetectExactIDProtects(t *testing.T) {
	rep := Detect(sessions(),
		[]Mark{{ID: "a", Reason: ReasonRunning}}, nil, nil, time.Now())
	if rep["a"] != ReasonRunning {
		t.Fatalf("want a protected as running, got %q", rep["a"])
	}
	if _, ok := rep["b"]; ok {
		t.Fatalf("b must not be protected, got %v", rep)
	}
}

func TestDetectMarkerProtects(t *testing.T) {
	rep := Detect(sessions(),
		nil, []Mark{{ID: "b", Reason: ReasonLock}}, nil, time.Now())
	if rep["b"] != ReasonLock {
		t.Fatalf("want b protected as in-use, got %q", rep["b"])
	}
}

func TestDetectResumeProtectsNewestInDir(t *testing.T) {
	rep := Detect(sessions(), nil, nil, []string{"/x"}, time.Now())
	if rep["b"] != ReasonResume {
		t.Fatalf("want b (newest in /x) protected as resume, got %q", rep["b"])
	}
	if _, ok := rep["a"]; ok {
		t.Fatalf("a must not be protected by the /x resume, got %v", rep)
	}
}

func TestDetectGraceWindowProtectsRecent(t *testing.T) {
	rep := Detect(sessions(), nil, nil, nil, time.Now())
	if rep["c"] != ReasonRecent {
		t.Fatalf("want c protected as recent, got %q", rep["c"])
	}
	if _, ok := rep["a"]; ok {
		t.Fatalf("a is outside the grace window and must not be protected")
	}
}

func TestDetectPrecedenceExplicitBeatsResumeBeatsRecent(t *testing.T) {
	// c is recent AND would be picked by a /y resume; an explicit marker wins.
	rep := Detect(sessions(),
		nil, []Mark{{ID: "c", Reason: ReasonLock}}, []string{"/y"}, time.Now())
	if rep["c"] != ReasonLock {
		t.Fatalf("explicit marker must outweigh resume and recency, got %q", rep["c"])
	}

	// b is recent AND newest in /x; the resume reason outweighs recency.
	rep = Detect(sessions(), nil, nil, []string{"/x"}, time.Now())
	if rep["b"] != ReasonResume {
		t.Fatalf("resume must outweigh recency, got %q", rep["b"])
	}
}

func TestDetectIgnoresUnknownIDs(t *testing.T) {
	rep := Detect(sessions(),
		[]Mark{{ID: "nope", Reason: ReasonRunning}}, nil, nil, time.Now())
	if _, ok := rep["nope"]; ok {
		t.Fatalf("unknown session id must not protect anything, got %v", rep)
	}
	if _, ok := rep["c"]; !ok {
		t.Fatal("a recent session is still protected by the grace window")
	}
}

func TestDetectEmptyResumeDirProtectsNewestOverall(t *testing.T) {
	rep := Detect(sessions(), nil, nil, []string{""}, time.Now())
	if rep["c"] != ReasonResume {
		t.Fatalf("empty resume dir should protect newest overall (c), got %q", rep["c"])
	}
}

func TestArgvForExactResumeID(t *testing.T) {
	procs := []Process{
		{PID: 1, Cmd: "/usr/local/bin/opencode -s ses_123abc"},
	}
	exact, resumes, running := argvFor("OpenCode", procs)
	if !running {
		t.Fatal("expected running")
	}
	if len(exact) != 1 || exact[0].ID != "ses_123abc" || exact[0].Reason != ReasonRunning {
		t.Fatalf("want exact ses_123abc running, got %+v", exact)
	}
	if len(resumes) != 0 {
		t.Fatalf("exact id must not be a resume, got %v", resumes)
	}
}

func TestArgvForContinueIsResume(t *testing.T) {
	procs := []Process{
		{PID: 1, Cmd: "/usr/local/bin/opencode -c", CWD: "/x"},
	}
	exact, resumes, _ := argvFor("OpenCode", procs)
	if len(exact) != 0 {
		t.Fatalf("continue must not name an exact id, got %+v", exact)
	}
	if len(resumes) != 1 || resumes[0] != "/x" {
		t.Fatalf("want resume dir /x, got %v", resumes)
	}
}

func TestArgvForBareIsResume(t *testing.T) {
	procs := []Process{
		{PID: 1, Cmd: "/usr/bin/env node /usr/local/bin/opencode", CWD: "/x"},
	}
	_, resumes, _ := argvFor("OpenCode", procs)
	if len(resumes) != 1 || resumes[0] != "/x" {
		t.Fatalf("bare opencode should be a tentative resume for /x, got %v", resumes)
	}
}

func TestArgvForUnrelatedProcessIgnored(t *testing.T) {
	procs := []Process{
		{PID: 1, Cmd: "/usr/bin/vim file.go"},
	}
	exact, resumes, running := argvFor("OpenCode", procs)
	if running || len(exact) != 0 || len(resumes) != 0 {
		t.Fatalf("vim is not opencode, got running=%v exact=%v resumes=%v", running, exact, resumes)
	}
}

func TestIsTokenNoPartialMatch(t *testing.T) {
	if isToken("opencode --continue", "-c") {
		t.Fatal("-c must not match inside --continue")
	}
	if !isToken("opencode -c", "-c") {
		t.Fatal("-c should match as a whole token")
	}
}

func TestDetectPropagatesChildProtectionToRoot(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	tree := []model.Session{
		{ID: "root", Children: []string{"child-1", "child-2"}, LastActivity: now.Add(-3 * day)},
		{ID: "child-1", Children: []string{"grand-1"}, LastActivity: now.Add(-3 * day)},
		{ID: "child-2", LastActivity: now.Add(-3 * day)},
		{ID: "grand-1", LastActivity: now.Add(-3 * day)},
	}
	// The grandchild is the only live member; its reason must fold up to the
	// root so a subtree sweep never touches an open descendant.
	rep := Detect(tree, nil, []Mark{{ID: "grand-1", Reason: ReasonRunning}}, nil, now)
	if rep["grand-1"] != ReasonRunning {
		t.Fatalf("grandchild = %q, want running", rep["grand-1"])
	}
	if rep["child-1"] != ReasonRunning {
		t.Fatalf("child-1 = %q, want running (propagated)", rep["child-1"])
	}
	if rep["root"] != ReasonRunning {
		t.Fatalf("root = %q, want running (propagated)", rep["root"])
	}
	if _, ok := rep["child-2"]; ok {
		t.Fatalf("child-2 must not be protected, got %v", rep)
	}
}
