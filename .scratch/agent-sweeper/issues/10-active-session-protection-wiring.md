# Ticket: Active-session protection wiring

Type: grilling
Status: open
Blocked by: 03

## Question

Given the active-session detection recipes (03), how does the sweep flow mark sessions as protected?

- Where protection is computed (during detection, before the dry-run)
- How protected sessions appear in the dry-run report (counted but not deletable)
- Behavior when every matching session is active/protected
- The fail-safe when detection is ambiguous

Decide and implement the protection wiring. Grill with a human.
