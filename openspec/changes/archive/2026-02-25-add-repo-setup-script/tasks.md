## 1. Script: scripts/setup_repo.sh

- [x] 1.1 Create `scripts/setup_repo.sh` with shebang, usage comment, and prerequisite checks (`gh` installed + authenticated)
- [x] 1.2 Implement `owner/repo` argument parsing with fallback to `gh repo view --json nameWithOwner`
- [x] 1.3 Implement `--force` flag parsing
- [x] 1.4 Apply repo-level settings via `gh api PATCH /repos/{owner}/{repo}`: squash-only merges, auto-merge enabled, delete-branch-on-merge
- [x] 1.5 Implement branch protection guard: read existing protection, skip (or warn) if stricter settings detected and `--force` not set
- [x] 1.6 Apply branch protection via `gh api PUT /repos/{owner}/{repo}/branches/main/protection`: require PRs, `required_linear_history=true`, `enforce_admins=false`, no required status checks
- [x] 1.7 Create labels `ai-consensus` and `human-review-required` via `gh label create --force`
- [x] 1.8 Mark script executable (`chmod +x scripts/setup_repo.sh`)

## 2. Makefile

- [x] 2.1 Add `repo-setup` target that calls `./scripts/setup_repo.sh`
- [x] 2.2 Ensure `repo-setup` appears in `make help` output with a description

## 3. Docs

- [x] 3.1 Update `docs/process/git-workflow.md`: add section stating direct push to `main` is blocked by GitHub branch protection and all changes must go through a PR
- [x] 3.2 Add reference to `make repo-setup` in that section for applying/refreshing protection

## 4. Verify

- [x] 4.1 Run `bash -n scripts/setup_repo.sh` (syntax check)
- [x] 4.2 Run `make repo-setup` against the actual repo and confirm settings in GitHub UI
- [x] 4.3 Re-run `make repo-setup` without `--force` and confirm no error and no regression
