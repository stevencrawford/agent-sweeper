# Ticket: Release and CI pipeline

Type: task
Status: resolved
Blocked by:
Assignee: stcrawfo (session)

## Question

How does agent-sweeper ship as a GitHub Release + Homebrew binary for three platforms?

- GitHub repo creation/remote setup for this repo
- goreleaser config mirroring omnivue's (macOS arm64/amd64, Linux arm64/amd64, Windows amd64)
- GitHub Actions workflow
- Homebrew tap + formula
- Versioning convention

**Task** — this one does rather than decides. Resolved when the pipeline exists; the answer records the resulting facts (repo URL, tap name, release URL).

## Answer

Pipeline is live; v0.1.0 shipped. Toolchain mirrors omnivue: tagpr + goreleaser, tags cut by merging the tagpr release PR.

- **Versioning**: `version/version.go` (Name/Version/Revision), wired into `cmd/root.go` as the Cobra `Version`; tagpr bumps it on release; ldflags `-X .../version.Revision` from goreleaser.
- **GitHub repo**: `https://github.com/stevencrawford/agent-sweeper` (public, remote `origin` SSH). No branch protection; PR creation for Actions enabled (`can_approve_pull_request_reviews=true` — tagpr needs it).
- **CI**: `.github/workflows/ci.yml` (build/test on ubuntu+macos, golangci-lint + gostyle on Linux) and `tagpr.yml` (tagpr → goreleaser `assets` job on the merge commit).
- **Release flow**: push to main → tagpr opens "Release for vX.Y.Z" PR → merge tags and drafts a release → goreleaser attaches binaries. Draft releases (`.tagpr` `release = draft`) get published by the merge.
- **Homebrew tap**: `https://github.com/stevencrawford/homebrew-tap` (public) with `Formula/agent-sweeper.rb` — versioned per-release URLs (`_darwin_{arm64,amd64}.zip`, lowercase names), real SHA256s. Install: `brew install stevencrawford/tap/agent-sweeper` (verified end-to-end, `brew test` passes).
- **Release artifacts** (v0.1.0): darwin/linux amd64+arm64, windows amd64, deb/rpm, checksums at `https://github.com/stevencrawford/agent-sweeper/releases/tag/v0.1.0`.
- **Gotchas resolved**: repo must be public for Homebrew; default workflow permission must allow PR creation for tagpr; first tagpr run on a fresh repo works once unshallow + PR perms are set.

Code: `version/`, `.goreleaser.yml`, `.github/workflows/{ci,tagpr}.yml`, `.github/release.yml`, `.tagpr`, `Makefile`, plus LICENSE/README/CHANGELOG. Verified: `go build`, `go test`, `go vet -vettool=gostyle`, `golangci-lint run`, `goreleaser check`, `brew install` + `brew test` from the live tap.
