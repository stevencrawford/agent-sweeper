# Ticket: Deletion semantics design

Type: prototype
Status: open
Blocked by: 01, 02, 04

## Question

Given the artifact inventory (01), DB deletion mechanics (02), and path map (04), what is the consolidated deletion plan?

- Order of deletion per agent (files first vs record last, so an interrupted sweep leaves no orphaned side files)
- Error handling and partial-failure behavior (what's reported, what's left behind)
- How the dry-run before-footprint is computed to match exactly what deletion will reclaim
- Any cross-store ordering (OpenCode snapshot/ dirs vs DB rows; Copilot session-state/ dirs vs DB rows)

Prototype the deletion engine — the single component every other slice feeds. Link the prototype as an asset.
