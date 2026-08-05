# Changelog

## [v0.1.0](https://github.com/stevencrawford/agent-sweeper/commits/v0.1.0) - 2026-08-05

## [Unreleased]

### Added

- Interactive `sweep` flow: pick an agent, group sessions by directory or git
  repo, pick an age, review the before-footprint, confirm once, delete, see the
  after-footprint.
- `stats` command: per-agent footprint table (AGENT | SESSIONS | RECLAIMABLE |
  STORE-ROW).
- Active-session protection: sessions currently open in any of the six agents
  are detected and never deleted.
- Cross-platform detection of OpenCode, Copilot, Claude Code, Codex, Pi, and
  Cursor session stores.
