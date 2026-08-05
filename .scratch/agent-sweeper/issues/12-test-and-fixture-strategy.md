# Ticket: Test and fixture strategy for destructive operations

Type: grilling
Status: resolved
Blocked by: 08
Claimed by: wayfinder session (working 12)

## Question

Since `sweep` deletes real user data, how do we test deletion safely?

- Fixture session stores (synthetic JSONL files, golden SQLite DBs with known rows)
- Dry-run assertions that nothing is deleted without confirm
- Deletion assertions against throwaway temp stores only
- What is off-limits in tests (never touch real agent stores)
- Coverage bar (omnivue runs golangci-lint + gostyle + go test)

Decide the test strategy and the fixture layout. Grill with a human.

## Answer

Resolved by grilling with the human (six decisions accepted) and a working fixture seam.

Code: `internal/testutil/` — `StoreRoot(t)` is the single seam every fixture path flows from; `SeedDB`/`WriteFile`/`ReadFile`/`CountRows` build scheme-minimal throwaway stores; `UnderRoot(root, path)` refuses any path that escapes the fixture root. Guard tests in `testutil_test.go`. Engine tests route through the seam (`engine_test.go` `newDB`/`countRows`/`readFile`).

**Decisions (all accepted by the human):**

1. **Shared helpers, inline fixtures.** Keep the existing inline `t.TempDir()` construction style; extract the repeated scaffolding (SQLite seeder, file readers, row counter) into a shared `testutil` package. No `testdata/` golden trees, no snapshot framework.
2. **Scheme-minimal fidelity.** Fixtures carry only the tables/rows a plan's actions need — never a faithful mirror of real agent schemas. Per-agent store shapes live in the research docs, not the test fixtures.
3. **Never-use-real-store guard = single-root seam.** All destructive tests build paths from `StoreRoot(t)` (a `t.TempDir()`); `UnderRoot` fails loudly if a path escapes the root, so a typo'd `~/.copilot/...` can never be swept. Detection/plan code only ever sees paths confined to the seam.
4. **No coverage threshold.** CI gate stays golangci-lint + gostyle + `go test`; no `-coverprofile` bar.
5. **Dry-run == confirm: existing test suffices.** `TestDryRunPlanEqualsExecution` already pins the invariant; no additional per-package duplicates.
6. **Off-limits:** real agent stores and real home-dotdirs are never readable or writable by tests; the seam makes that structural, not conventional.

Verified: `go build`, `go test ./...` (all green), `go vet -vettool=gostyle`, `golangci-lint run` — all clean.
