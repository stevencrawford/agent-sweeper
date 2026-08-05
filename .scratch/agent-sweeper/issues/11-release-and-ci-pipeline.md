# Ticket: Release and CI pipeline

Type: task
Status: open
Blocked by:

## Question

How does agent-sweeper ship as a GitHub Release + Homebrew binary for three platforms?

- GitHub repo creation/remote setup for this repo
- goreleaser config mirroring omnivue's (macOS arm64/amd64, Linux arm64/amd64, Windows amd64)
- GitHub Actions workflow
- Homebrew tap + formula
- Versioning convention

**Task** — this one does rather than decides. Resolved when the pipeline exists; the answer records the resulting facts (repo URL, tap name, release URL).
