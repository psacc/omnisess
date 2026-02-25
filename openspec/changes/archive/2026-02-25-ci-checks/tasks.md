## 1. Create workflow file

- [x] 1.1 Create `.github/workflows/` directory if it doesn't exist
- [x] 1.2 Write `.github/workflows/ci.yml` with `on: push` (all branches) and `pull_request` targeting `main`
- [x] 1.3 Define single job `check` on `ubuntu-latest` with `actions/checkout@v4` as first step
- [x] 1.4 Add `actions/setup-go@v5` step with `go-version: "1.22"` and `cache: true`
- [x] 1.5 Add golangci-lint install step (official installer script) before `make check`
- [x] 1.6 Add `run: make check` as the final step

## 2. Verify locally

- [x] 2.1 Validate YAML syntax (`python3 -c "import yaml, sys; yaml.safe_load(sys.stdin)" < .github/workflows/ci.yml`)
- [x] 2.2 Confirm job is named `check` (grep the file)
- [x] 2.3 Confirm no `matrix:` key appears anywhere in the file

## 3. Commit and merge

- [ ] 3.1 Commit on a branch (`ci/add-github-actions`)
- [ ] 3.2 Open PR with `make pr`, fill PR template
- [ ] 3.3 Confirm CI workflow triggers on the PR and `check` job passes
- [ ] 3.4 Merge PR — verify `check` job appears in branch protection options
