# Ticket: Git-repo grouping semantics

Type: grilling
Status: open
Blocked by: 07

## Question

Ticket 07's review added a **git-repo grouping mode** to the sweep flow: after picking an agent, the user chooses directory or git-repo grouping; git mode groups sessions by repo and branch, multi-selects branches, and shows the branch per row in the dry-run. How does real detection feed that mode?

- **Repo identity derivation**: per session, how is the repo identified — git remote URL at the session's cwd, the `.git` directory path, a parsed owner/name, or a fallback? Sessions with a cwd but no repo (untracked/non-git dirs) and sessions with no cwd at all — what buckets do they land in, and are they sweepable in git mode?
- **Branch capture**: where does the branch come from — git HEAD at session time, a per-store field (opencode `git_branch`?, copilot workspace, claude/codex transcripts), or a best-effort read of the repo at sweep time? What if the branch has since been merged/deleted (the motivating case)?
- **Dry-run behavior**: confirm the branch column and the merged-branch cleanup flow from 07's prototype; what does the picker show when many sessions share one repo across branches (nested repo → branch, or flat)?
- **Mode interplay**: does the mode picker also gate `stats` (see 09)? Does git mode remain a per-sweep choice or become a config/setting?

Decide the data model and derivation for git-repo mode. Grill with a human.