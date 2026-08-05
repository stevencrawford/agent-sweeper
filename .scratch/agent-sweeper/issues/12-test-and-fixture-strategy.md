# Ticket: Test and fixture strategy for destructive operations

Type: grilling
Status: open
Blocked by: 08

## Question

Since `sweep` deletes real user data, how do we test deletion safely?

- Fixture session stores (synthetic JSONL files, golden SQLite DBs with known rows)
- Dry-run assertions that nothing is deleted without confirm
- Deletion assertions against throwaway temp stores only
- What is off-limits in tests (never touch real agent stores)
- Coverage bar (omnivue runs golangci-lint + gostyle + go test)

Decide the test strategy and the fixture layout. Grill with a human.
