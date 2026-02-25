# Git Workflow

Branch, commit, review, and merge rules for agents working in this repository.

## 1. Branch Naming

| Prefix | Use when | Example |
|--------|----------|---------|
| `fix/<slug>` | Bug fix | `fix/active-detection` |
| `feat/<slug>` | New feature or capability | `feat/gemini-source` |
| `chore/<slug>` | Non-functional: refactors, tooling, CI | `chore/lint-config` |

Slugs are lowercase, hyphen-separated, and short (2-4 words max).

## 2. Branch Protection

Direct push to `main` is **blocked by GitHub branch protection**. All changes MUST go through a pull request — there is no bypass, including for admins.

Branch protection settings applied:
- Squash-only merges (no merge commits, no rebase)
- Linear history required
- Auto-delete branch on merge
- `enforce_admins: false` (solo project — tighten via `--force` if collaborators join)

To apply or refresh protection settings after cloning or reconfiguring the repo:

```bash
make repo-setup
```

This is idempotent — safe to re-run at any time. Run `make repo-setup FORCE=1` to overwrite stricter manually-applied settings.

## 3. When to Branch vs. Commit to Main

**Commit directly to `main`** only when ALL of these are true:

- The change is doc-only (`.md` files, comments) OR config-only (`.golangci.yml`, `Makefile`)
- No Go source files are touched
- `make check` passes

**Create a branch** when ANY of these is true:

- Go source files change (`.go`)
- Test files change
- `go.mod` or `go.sum` change
- The change spans more than one commit

When in doubt, branch. Branches are free; broken main is not.

## 4. Commit Messages

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

## 5. The Full Flow

```
1. Branch    git checkout -b <prefix>/<slug>
2. Implement Write code, following CLAUDE.md invariants
3. Verify    make check (must be clean -- zero warnings, zero failures)
4. Smoke     make smoke
5. Commit    git add <files> && git commit (conventional message)
6. Review    Spawn a reviewer subagent against the branch diff
7. Address   Fix all findings from review (amend or new commit)
8. Classify  Two-way door → self-merge. One-way door → push, escalate.
9. Merge     make merge (squash-merges branch into main, keeps linear history)
```

Steps 2-5 may repeat within a branch. Steps 6-7 may repeat if the reviewer finds new issues after fixes. Each commit should be a coherent, reviewable unit.

## 6. Review and Merge Rules

Review rules depend on the decision classification from [`agent-review.md`](agent-review.md).

### Review is mandatory, but not human

Every branch MUST be reviewed before merge. The default reviewer is a **subagent**, not a human.

The implementing agent:

1. Completes the fix/feature on a branch
2. Spawns a reviewer subagent against the branch diff
3. Addresses all findings from the review (amend or new commit)
4. Proceeds to merge per the rules below

Human review is only required for one-way door escalations (see below).

### PR body requirements

Every PR MUST have all sections of `.github/pull_request_template.md` populated before merge. Unfilled placeholder text or empty sections are grounds for a reviewer to request changes. When opening a PR manually, compose the body from the template:

```bash
gh pr create --title "<commit subject>" --body "$(cat <<'EOF'
## Summary
- <bullet>

## Type of change
- [ ] Bug fix
- [ ] New feature
- [x] Chore / tooling

## Test plan
- [x] `make check` passes
- [x] `make smoke` passes

## OpenSpec change
`<change-name>` or N/A

## Breaking changes
Yes / No — <description if yes>
EOF
)"
```

Do not use `gh pr create --fill`; it bypasses the template.

### Two-way door changes (agent self-merges after subagent review)

ALL of these must be true:

- Classified as two-way door per `agent-review.md` Section 1
- **Subagent review completed** and all findings addressed
- `make check` passes with zero warnings and zero test failures
- `make smoke` ran and produced expected output
- Exec plan status updated (moved to `completed/` if done)
- No new external dependencies added

If all conditions hold, the agent runs `make merge` (squash-merge into main) and verifies. Do not ask a human.

### One-way door changes (escalate to human)

ANY of these triggers escalation:

- Classified as one-way door per `agent-review.md` Section 1
- New external dependency (`go.mod` change adding a module)
- Changes to public interfaces (`Source`, `model.*` types, CLI flags)
- Change affects 3+ packages

The agent pushes the branch but does NOT merge. Leave a summary using the escalation format from `agent-review.md` Section 4.

### Uncertain

If classification is unclear, request a reviewer subagent per `agent-review.md` Section 2. Do not merge until resolved.

## 7. Pre-Merge Checklist

Before any merge to `main`, verify every item:

- [ ] `make check` clean (fmt + vet + lint + test, zero failures)
- [ ] `make smoke` passed (binary in PATH, `omnisess list --limit=1` exits 0)
- [ ] Exec plan: status updated (`active/` -> `completed/` if finished)
- [ ] Commit messages: follow conventional format, include `agent-decided` tag if applicable
- [ ] No untracked files left behind (build artifacts, temp files)
- [ ] Branch deleted after merge

## 8. Post-Merge

After merging to `main`:

1. Delete the feature branch: `git branch -d <branch>`
2. Verify `make check` still passes on `main`
3. Update `ARCHITECTURE.md` if the codemap changed (new packages, renamed files)
