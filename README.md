# agent-sweeper

Clean stale AI coding-agent session stores.

`agent-sweeper` detects the local session stores of the six major coding agents
— OpenCode, Copilot, Claude Code, Codex, Pi, and Cursor — reports their
filesystem footprint, and cleans stale sessions interactively. It never touches
actively-open sessions, and nothing is deleted without review.

## Install

### Homebrew

```sh
brew install stevencrawford/tap/agent-sweeper
```

### Manual

Download the archive for your platform from the
[latest release](https://github.com/stevencrawford/agent-sweeper/releases),
then place the binary on your `$PATH`.

## Usage

```
agent-sweeper sweep   # interactive cleanup flow
agent-sweeper stats   # per-agent footprint table
agent-sweeper --help
```

In `sweep`, pick an agent, filter its sessions by directory or git repo, choose
an age, review the before-footprint, confirm once, and see the after-footprint.
Actively-open sessions are protected and never listed as deletable.

## Build from source

Requires Go 1.26.

```sh
make build        # builds ./agent-sweeper with the current version
make test         # runs the test suite
make lint         # golangci-lint + gostyle
```

## License

MIT — see [LICENSE](LICENSE).