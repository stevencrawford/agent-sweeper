# Ticket: Git-repo grouping semantics

Type: grilling
Status: resolved
Blocked by: 07
Claimed by: wayfinder session (working 13)

## Question

Ticket 07's review added a **git-repo grouping mode** to the sweep flow: after picking an agent, the user chooses directory or git-repo grouping; git mode groups sessions by repo and branch, multi-selects branches, and shows the branch per row in the dry-run. How does real detection feed that mode?

- **Repo identity derivation**: per session, how is the repo identified — git remote URL at the session's cwd, the `.git` directory path, a parsed owner/name, or a fallback? Sessions with a cwd but no repo (untracked/non-git dirs) and sessions with no cwd at all — what buckets do they land in, and are they sweepable in git mode?
- **Branch capture**: where does the branch come from — git HEAD at session time, a per-store field (opencode `git_branch`?, copilot workspace, claude/codex transcripts), or a best-effort read of the repo at sweep time? What if the branch has since been merged/deleted (the motivating case)?
- **Dry-run behavior**: confirm the branch column and the merged-branch cleanup flow from 07's prototype; what does the picker show when many sessions share one repo across branches (nested repo → branch, or flat)?
- **Mode interplay**: does the mode picker also gate `stats` (see 09)? Does git mode remain a per-sweep choice or become a config/setting?

Decide the data model and derivation for git-repo mode. Grill with a human.

## Answer

Resolved by grilling with the human (four decisions accepted) plus the flow fix raised mid-grill.

Code: `internal/tui/tui.go` — `onlyWithRepo` filters sessions before `GroupByRepoBranch`; back-nav from dir/branch pickers restores `m.cursor` to the current mode so the git↔directory pivot is clean. Tests: `tui_test.go` (`TestGitModeExcludesNoRepoSessions`, `TestGitModeEscBackLandsOnModePicker`, `TestGitModeEscBackAllowsDirectoryPivot`).

**Decisions (all accepted by the human):**

1. **Repo identity — native field, else cwd→.git.** Use the agent's native repo field where it exists (Copilot `sessions.repository`; OpenCode project→git-root linkage); for agents that store only cwd, resolve at scan time by reading `cwd/.git` (remote URL → owner/name). Best-effort: unreadable → no repo. OpenCode snapshot/`project/` mapping is the native source for OpenCode sessions.
2. **Branch capture — session-time branch only.** The branch is whatever the agent recorded at session start (Copilot `sessions.branch`); no read-time HEAD derivation. A merged/deleted branch stays labeled as it was at session time — that recorded label *is* the cleanup signal (the branch exists in the picker even after deletion, so the user can sweep everything that ran on it). No other agent stores a branch today; those sessions group at repo level with empty branch.
3. **Picker shape — flat repo·branch rows.** Keep 07's flat multi-select list. **No-repo sessions are not in git mode at all** — `onlyWithRepo` filters them out; sessions without a resolved repo are swept exclusively via directory mode (they never appear as an anonymous git bucket). This keeps the merged-branch cleanup list honest: every row is a real repo you can act on.
4. **Mode interplay — per-sweep choice, stats unaffected.** Git mode stays a per-sweep choice on the sweep command only; `stats` remains the 09 per-agent directory table. No persisted config/setting.

**Flow fix (raised mid-grill):** pivoting git mode → back → directory mode was broken. `esc` from the branch picker returned to the mode picker with `m.cursor` still holding the branch-list index (which can exceed the mode picker's 0–1 range), so no mode row highlighted and `down` was clamped — the user appeared stuck in git mode. Fix: back-nav from dir/branch pickers restores `m.cursor = int(m.mode)`, so esc lands on the mode picker with the current mode selected and the user can cleanly flip to directory mode (or back). Dir/branch pickers themselves are reachable in one step from the mode picker.

Verified: `go build`, `go test ./...` (all green, incl. new pivot/exclusion tests), `go vet -vettool=gostyle`, `golangci-lint run` (0 issues).