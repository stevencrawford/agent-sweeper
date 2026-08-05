# Ticket: Deletion semantics design

Type: prototype
Status: resolved
Blocked by: 01, 02, 04
Claimed by: wayfinder session (working 08)

## Question

Given the artifact inventory (01), DB deletion mechanics (02), and path map (04), what is the consolidated deletion plan?

- Order of deletion per agent (files first vs record last, so an interrupted sweep leaves no orphaned side files)
- Error handling and partial-failure behavior (what's reported, what's left behind)
- How the dry-run before-footprint is computed to match exactly what deletion will reclaim
- Any cross-store ordering (OpenCode snapshot/ dirs vs DB rows; Copilot session-state/ dirs vs DB rows)

Prototype the deletion engine — the single component every other slice feeds. Link the prototype as an asset.

## Answer

Resolved by a grilling with the human (all five decisions confirmed) and a working deletion-engine prototype, reviewed by the human.

Prototype asset: `internal/engine/` — `plan.go` (the plan/action model), `engine.go` (execution), `engine_test.go` (9 tests against real SQLite files via `modernc.org/sqlite`). `go build`, gostyle vet, golangci-lint, `go test ./...` all clean. Driver `modernc.org/sqlite` (pure-Go) added to go.mod, matching omnivue.

**Design decisions (all accepted by the human):**

1. **Files first, record last.** A session's plan lists filesystem artifacts, then the record (DB rows, index entries). A session stops at its first error, so the residue of an interrupted sweep is always a *visible, self-healing ghost* (record present, files gone) that a re-run finishes — never invisible orphaned files (record gone, files remain) that detection can never find again.
2. **Continue and report.** One failed session or locked store never aborts the sweep; per-session outcomes (deleted / failed) are reported, remaining sessions still run.
3. **Trust the plan.** The dry-run builds the exact per-session plan once and sums it; the confirmed sweep executes that same plan. `Plan.Reclaim()` == dry-run number == `Result.BytesReclaimed()` (tested).
4. **No auto-VACUUM.** Reclaim counts filesystem bytes removed. SQLite row deletes don't shrink the DB file; a sweep that deleted rows sets `Result.NeedsVacuum()` so the after-report can say the file is unchanged until VACUUM.
5. **Shared stores excluded.** OpenCode `snapshot/` and Cursor `composer.content.*`/`agentKv:blob:*` are never in per-session plans and never counted in reclaim; project `git gc` and content-addressed reference-counting are deferred (see map fog).

**Engine semantics:** per-session-per-store deletes run in one `BEGIN IMMEDIATE` txn with `foreign_keys=ON` (so OpenCode's cascade works) and a 5s busy timeout, retried 3× on `SQLITE_BUSY`/`LOCKED`. All actions are idempotent: missing files, dirs, globs, index files, and KV keys are not errors, and a re-run converges.

**Action model** (`ActionKind`): `RemoveFile`, `RemoveTree`, `RemoveGlob` (filesystem), `SQLDelete` (DB rows), `DropJSONLLines` (codex `session_index.jsonl`, claude `history.jsonl`), `DropJSONKeys` (claude `sessions-index.json`), `StripKV` (cursor workspace `composer.composerData` — removes the id from allComposers/selectedComposerIds/lastFocusedComposerIds in one txn).

**Consolidated per-agent deletion plan** (ordered; → = files first, record last):

| Agent | Ordered actions |
|---|---|
| OpenCode | RemoveTree `storage/{message,part,session_diff}/<id>` blobs → SQLDelete `opencode.db` session row (FK cascade; recurse `session.parent_id` for children). `snapshot/<project-id>/` NOT touched |
| Copilot | RemoveTree `session-state/<uuid>/` → SQLDelete `session-store.db` (search_index→turns→checkpoints→session_files→session_refs→assistant_usage_events→sessions) → SQLDelete `data.db` sessions row |
| Claude Code | RemoveFile project jsonl → RemoveTree `<uuid>/` subagent dir → RemoveGlob shell-snapshots → DropJSONKeys `sessions-index.json` → DropJSONLLines `history.jsonl` (edit entry, never delete file; human confirmed) |
| Codex | RemoveFile rollout (+ archived copy) → RemoveGlob `shell_snapshots/<id>.*.sh` → DropJSONLLines `session_index.jsonl` → SQLDelete `state_*.sqlite` threads row. Equivalent to `codex delete <SESSION>` |
| Pi | RemoveFile the single `sessions/<cwd-slug>/<ts>_<uuid>.jsonl` — file is the record, no ordering |
| Cursor | RemoveTree `agent-transcripts/<uuid>/` → SQLDelete global `state.vscdb` KV rows (composerData/bubbleId/checkpointId/messageRequestContext/codeBlockDiff/agentKv:checkpoint by id) → StripKV workspace `composer.composerData` (the visible record, last) → optional SQLDelete `ai-code-tracking.db` conversation_summaries row. `composer.content.*` NOT touched |

**Invariant guaranteed by construction:** the dry-run before-footprint is the plan's reclaim; what the sweep actually deletes is exactly that. A failed session reclaims nothing and stays visible for a re-run.
