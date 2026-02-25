## Context

omnisess currently has no branch protection on `main`. The PR-based workflow (documented in `docs/process/git-workflow.md`) is convention-only — any contributor with push access can bypass it. The sibling project `ai-assistant` has a `scripts/setup_repo.sh` that configures GitHub via the `gh` CLI; we adapt a lighter version here (no CI, no required status checks).

The script must be idempotent: safe to re-run at any time without degrading stricter settings applied manually.

## Goals / Non-Goals

**Goals:**
- Enforce squash-only merges, auto-delete branches, auto-merge at repo level
- Protect `main`: require PRs, enforce linear history, no bypass for admins
- Idempotent: re-running does not weaken settings unless `--force` is passed
- Makefile `repo-setup` target for discoverability
- Document the resulting constraint in `docs/process/git-workflow.md`

**Non-Goals:**
- Required status checks (no CI yet — added when CI is introduced)
- Required reviewers or CODEOWNERS (solo project)
- Any changes to the Go source code or CLI behavior

## Decisions

**D1: Use `gh api` over `gh repo edit`**
`gh repo edit` covers repo-level settings but not branch protection rules. `gh api` (REST) covers both in one tool. Alternative: use the GitHub web UI — rejected because it is not repeatable or reviewable.

**D2: `--force` flag to allow weakening**
Default behavior: if existing branch protection is stricter (e.g., required reviews already set), skip rather than overwrite. `--force` bypasses this guard. This prevents accidental regressions on repos that get stricter settings added manually later.

**D3: `enforce_admins: false`**
Solo project — blocking admin pushes adds friction with no benefit. Can be tightened via `--force` if the project gets collaborators.

**D4: Labels created idempotently with `gh label create --force`**
`--force` on `gh label create` updates color/description if the label exists, which is the correct idempotent behavior for labels.

**D5: Script signature `./scripts/setup_repo.sh [owner/repo] [--force]`**
`owner/repo` defaults to `$(gh repo view --json nameWithOwner -q .nameWithOwner)` if omitted. This matches the ai-assistant pattern and works from any directory.

## Risks / Trade-offs

- [Risk] `gh` CLI not installed → Script exits with clear error message. Mitigation: document prerequisite in script header and in git-workflow.md.
- [Risk] Token lacks `admin:repo_hook` / `repo` scope → `gh api` calls fail with 403. Mitigation: script checks `gh auth status` at startup and prints required scopes.
- [Risk] Future CI addition requires re-running setup to add required checks → Acceptable; the script will be updated then. Non-issue now.
- [Trade-off] No automated enforcement that `make repo-setup` is run → Acceptable for a solo project; the Makefile target + docs create sufficient discoverability.

## Migration Plan

1. Merge this change to `main`
2. Run `make repo-setup` (or `./scripts/setup_repo.sh psacc/omnisess`) once
3. Verify branch protection in GitHub settings UI
4. Rollback: disable branch protection via GitHub UI or `gh api` DELETE — no code rollback needed
