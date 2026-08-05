# Ticket: stats command data model

Type: grilling
Status: open
Blocked by: 01

## Question

Given the artifact inventory (01), what does `stats` report?

- Per-agent totals (session count, bytes) before any interaction
- Drill-down into a selected agent's per-directory / per-session footprint
- Whether `stats` and the `sweep` dry-run share one footprint computation (they should)

Decide the data model and the output shape (table, size formatting). Grill with a human.
