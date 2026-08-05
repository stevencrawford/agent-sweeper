# Ticket: TUI flow and screens

Type: prototype
Status: resolved
Blocked by:
Claimed by: wayfinder session (working 07)

## Question

What are the bubbletea screens for `sweep` and what does each show?

1. Agent picker (the six detected agents, with footprint)
2. Directory picker for the selected agent (multi-select groups of distinct cwds, each row showing its session count; group = canonical path, label = original spelling; an anonymous `(no directory)` bucket holds no-cwd sessions — see 05)
3. Age picker (the enum)
4. Dry-run before-footprint report (per directory, per session, sizes)
5. Single confirm
6. Delete progress
7. After-footprint report (reclaimed bytes)

Prototype the flow as a rough, runnable TUI stub wired to the real six-agent data shape (names, dirs, sessions, sizes) — not yet real detection. Link the prototype as an asset.

## Answer

Resolved by a runnable prototype stub, reviewed by the human.

Prototype asset: `main.go` + `internal/{model,mock,tui}/` in the repo root (`go run .`). Bubbletea v1.3.10 + lipgloss; `go build`, `go test`, golangci-lint, and gostyle all clean.

**Flow confirmed (with one change made during review):**

1. **Agent picker** — six agents, each row shows name, session count, total footprint (e.g. `OpenCode 5 sessions 460.0MiB`).
2. **Grouping-mode picker** *(added during review — see below)* — "directory" or "git repository".
3. **Directory picker** (dir mode) — multi-select rows of distinct cwds, session count + bytes per row; canonical path is the key, label the display; `(no directory)` bucket for no-cwd sessions. Requires ≥1 selection; space toggles, enter continues.
4. **Branch picker** (git mode) — multi-select rows of `repo · branch` with session count + bytes; branch shown per row in the dry-run.
5. **Age picker** — the 06 enum (`1d…all`), single-select.
6. **Dry-run report** — grouped by selection, one line per session (title, id, size, branch in git mode). **Only deletable rows render** (per human review): active/too-young sessions are excluded from the listing and folded into a single protected-count footnote. Footer: `would delete N sessions, reclaiming X`.
7. **Single confirm** — `y`/`n`, warns deletion is permanent and active sessions are never touched.
8. **Delete progress** — simulated progress bar (prototype has no real deletion).
9. **After-footprint report** — `Reclaimed X from <agent> across N sessions`.

**Change made during review (reopens 05):** the human asked for a **git-repo grouping mode** — an extra step after the agent picker choosing directory vs git-repo grouping, with branch info in the dry-run and branch multi-select, motivated by merged-branch cleanup driven by flags. Implemented as a mode picker + branch picker (see 2 and 4); `model.BranchGroup` groups by repo+branch. The prototype is wired to synthetic repo/branch fields in mock; how the real detection derives repo identity and branch is now ticket **13 git-repo grouping semantics**. The "drive cleanup via flags" want feeds the scriptable-mode fog item.

**Bug found while building:** `GroupByCWD` ordering was nondeterministic (map iteration), which could make the dry-run total disagree with the confirm total run-to-run. Fixed with a stable sort (count desc, then path asc, anonymous bucket last); covered by `internal/model/model_test.go`.

Keys: ↑/↓/j/k move, space toggle (dir/branch), enter continue, esc back, y/n confirm, q/ctrl+c quit.
