# Git Workflow

Branch, commit, review, and merge rules for agents working in this repository.

## 1. Branch Naming

| Prefix | Use when | Example |
|--------|----------|---------|
| `fix/<slug>` | Bug fix | `fix/active-detection` |
| `feat/<slug>` | New feature or capability | `feat/gemini-source` |
| `chore/<slug>` | Non-functional: refactors, tooling, CI | `chore/lint-config` |
| `docs/<slug>` | Documentation-only changes | `docs/pr-workflow` |

Slugs are lowercase, hyphen-separated, and short (2-4 words max).

## 2. When to Branch vs. Commit to Main

**Always branch.** Now that the repo is public at `github.com/psacc/omnisess`, all changes go through a PR — including doc-only changes.

The only exception is automated tooling commits (e.g. `go mod tidy` triggered by Dependabot) that arrive via their own PR.

## 3. Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/) with the prefixes used in this repo:

```
<type>: <short summary>
```

Types: `fix`, `feat`, `chore`, `docs`, `test`, `refactor`

Examples from this repo's history:

```
fix: improve active session detection reliability
feat: add development harness -- tests, linting, and pre-commit hooks
chore: rename AGENTS.md -> CLAUDE.md, add doc cross-references
docs: add agent-to-agent review process with decision rules
```

For agent-decided design decisions, append to the commit body:

```
Decision (agent-decided): <what and why>
```

## 4. The Full Flow

```
1. Plan      /opsx:new — create OpenSpec change with proposal → design → specs → tasks
2. Branch    git checkout -b <prefix>/<slug>
3. Implement Write code following CLAUDE.md invariants; /opsx:apply to work through tasks
4. Verify    make check (must be clean — zero warnings, zero failures)
5. Smoke     Run the relevant `omnisess` subcommand against real local data
6. Commit    git add <files> && git commit (conventional message)
7. Push      git push -u origin <branch>
8. PR        make pr  (or: gh pr create — see §5)
9. Review    Spawn a reviewer subagent against the PR diff
10. Address  Fix all findings (new commit on the branch; push again)
11. Merge    gh pr merge --squash --delete-branch
12. Archive  /opsx:archive — move OpenSpec change to completed, update main specs
```

Steps 3-6 may repeat within a branch. Steps 9-10 may repeat if the reviewer finds new issues after fixes.

## 5. Opening a PR

Use `make pr` to push the branch and open a PR in one step:

```bash
make pr
```

This runs: `git push -u origin HEAD && gh pr create --fill`

Or manually:

```bash
git push -u origin HEAD
gh pr create --title "<title>" --body "$(cat <<'EOF'
## Summary
- <bullet>

## Test plan
- [ ] make check passes
- [ ] smoke test: omnisess <subcommand> produces expected output

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

## 6. Review and Merge Rules

Review rules depend on the decision classification from [`agent-review.md`](agent-review.md).

### Review is mandatory, but not human

Every PR MUST be reviewed before merge. The default reviewer is a **subagent**, not a human.

The implementing agent:

1. Completes the fix/feature on a branch and pushes
2. Opens a PR with `make pr`
3. Spawns a reviewer subagent (type: `coderabbit:code-reviewer`) against the PR diff
4. Addresses all findings (new commit, push)
5. Merges per the rules below

Human review is only required for one-way door escalations (see below).

### Two-way door changes (agent merges after subagent review)

ALL of these must be true:

- Classified as two-way door per `agent-review.md` Section 1
- **Subagent review completed** and all findings addressed
- `make check` passes with zero warnings and zero test failures
- Smoke test ran and produced expected output
- OpenSpec change archived (`/opsx:archive`) if applicable
- No new external dependencies added

If all conditions hold, the agent runs `gh pr merge --squash --delete-branch`. Do not ask a human.

### One-way door changes (escalate to human)

ANY of these triggers escalation:

- Classified as one-way door per `agent-review.md` Section 1
- New external dependency (`go.mod` change adding a module)
- Changes to public interfaces (`Source`, `model.*` types, CLI flags)
- Change affects 3+ packages

The agent opens the PR but does NOT merge. Leave a summary using the escalation format from `agent-review.md` Section 4.

### Uncertain

If classification is unclear, request a reviewer subagent per `agent-review.md` Section 2. Do not merge until resolved.

## 7. Pre-Merge Checklist

Before merging any PR, verify every item:

- [ ] `make check` clean (fmt + vet + lint + test, zero failures)
- [ ] Smoke test: ran relevant `omnisess` subcommand, output is correct
- [ ] OpenSpec change archived if this PR completed a change
- [ ] Commit messages: follow conventional format, include `agent-decided` tag if applicable
- [ ] No untracked files left behind (build artifacts, temp files)
- [ ] PR description is accurate and complete

## 8. Post-Merge

After the PR is merged:

1. GitHub deletes the branch automatically (`--delete-branch` flag)
2. Pull main locally: `git checkout main && git pull`
3. Verify `make check` still passes on `main`
4. Update `ARCHITECTURE.md` if the codemap changed (new packages, renamed files)
