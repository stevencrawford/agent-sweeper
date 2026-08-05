# agent-sweeper

Clean stale AI coding-agent session stores.

`agent-sweeper` detects the local session stores of the six major coding agents
— OpenCode, Copilot, Claude Code, Codex, Pi, and Cursor — reports their
filesystem footprint, and cleans stale sessions interactively or from a script.
It never touches actively-open sessions, and nothing is deleted without review:
`stats` and `sweep --dry-run` are strictly read-only.

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
agent-sweeper stats                     # per-agent footprint table
agent-sweeper stats --json              # same, machine-readable
agent-sweeper sweep                     # interactive cleanup flow
agent-sweeper sweep --agent Claude\ Code --age 30d --dry-run   # preview only
agent-sweeper sweep --agent Claude\ Code --age 30d --yes       # delete (see below)
agent-sweeper --help
```

In `sweep`, pick an agent, filter its sessions by directory or git repo, choose
an age, review the before-footprint, confirm once, and see the after-footprint.
Actively-open sessions are protected and never listed as deletable.

Non-interactive mode (`--agent`) filters by `--mode dir|git`, `--dir`,
`--repo`, `--branch`, and `--age 1d|3d|7d|30d|90d|1y|all`. It never deletes
without `--yes`, and `--yes` never bypasses active-session protection — a
session opened since the plan was built is skipped at execution time. Pass
`--dry-run` to print the plan and exit without deleting, and `--json` for
machine-readable output.

## Build from source

Requires Go 1.26.

```sh
make build        # builds ./agent-sweeper with the current version
make test         # runs the test suite
make lint         # golangci-lint + gostyle
```

## License

MIT — see [LICENSE](LICENSE).