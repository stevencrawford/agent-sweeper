# Ticket: Active-session protection wiring

Type: grilling
Status: resolved
Blocked by: 03
Assignee: stcrawfo (session)

## Question

Given the active-session detection recipes (03), how does the sweep flow mark sessions as protected?

- Where protection is computed (during detection, before the dry-run)
- How protected sessions appear in the dry-run report (counted but not deletable)
- Behavior when every matching session is active/protected
- The fail-safe when detection is ambiguous

Decide and implement the protection wiring. Grill with a human.

## Answer

Decided by grilling, implemented as a real detector. Code: `internal/protect/` (detector), wired in `internal/tui/` and `cmd/sweep.go`.

- **Where protection is computed**: once at sweep load — `protect.ScanOne` (real: `ps` process list + per-agent resume-argv parsing + durable markers) marks `Session.Active` before the pickers render — and again at confirm, where a **fresh** scan drops any session that became active since the dry-run (a session opened after the dry-run is never deleted).
- **How protected sessions appear**: the dry-run lists them dimmed per selected group with the protection reason (e.g. "a live process is running it"), and the summary counts them separately from the deletable set.
- **Every matching session active/protected**: 0 matches blocks the flow — it stays on the age picker with an explanation instead of advancing to a meaningless empty confirm.
- **Fail-safe (protect over delete)**: exact resume argv (`opencode -s`, `claude -r`, `codex resume`, `pi --session`, `copilot --resume`, `cursor --composer`) and durable markers protect outright (Copilot `inuse.<pid>.lock` with PID+liveness validation; Claude `daemon/roster.json` + `jobs/*/state.json`; Cursor focused composers from `state.vscdb`, gated on a running Cursor); tentative resumes protect the most-recent session per directory; a 24h grace window (`protect.DefaultGrace`) protects anything recently active — the hard rule covering Pi and interactive Claude, which have no durable marker.

Gaps (best-effort by design, all read-only): Claude roster short-ids may not exactly equal full transcript ids; mock session ids will never match real argv (mock protection demos via the grace window).

Tests: `internal/protect/protect_test.go` (pure `Detect` core + argv parsing) and `internal/tui/tui_test.go` (protected rows listed, 0-match block, confirm re-validation). `go build`, `go vet -vettool=gostyle`, `golangci-lint run`, `go test ./...` all pass.
