## Why

Direct push to `main` is currently possible — no branch protection is configured. This allows accidental bypasses of the PR-based workflow and makes linear history enforcement aspirational rather than enforced. The setup script exists in the sibling `ai-assistant` project; adapting a lighter version here is low-effort and closes the gap.

## What Changes

- Add `scripts/setup_repo.sh`: idempotent GitHub repo settings bootstrap (squash-only merges, auto-merge, delete-branch-on-merge, branch protection requiring PRs + linear history; no required status checks; enforce_admins=false)
- Add `repo-setup` Makefile target that calls the script
- Update `docs/process/git-workflow.md` to document that direct push to main is blocked and PRs are required

## Capabilities

### New Capabilities

- `repo-setup-script`: Bootstrap script + Makefile target to configure GitHub repo settings and branch protection idempotently

### Modified Capabilities

- `git-workflow-docs`: Existing workflow doc gains branch-protection note (requirement-level change: direct push is now explicitly prohibited)

## Impact

- New file: `scripts/setup_repo.sh`
- Modified: `Makefile` (new `repo-setup` target, referenced in `make setup` or standalone)
- Modified: `docs/process/git-workflow.md` (direct push blocked, PR required)
- No code changes; no API changes; no new dependencies
