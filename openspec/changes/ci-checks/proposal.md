## Why

The repository has no CI. PRs and pushes to `main` are unvalidated — broken builds and failing tests can land silently. Adding a GitHub Actions workflow that runs `make check` closes this gap with minimal setup.

## What Changes

- Add `.github/workflows/ci.yml` — a single-job workflow that runs `make check` (fmt + vet + lint + test) on every push and on PRs targeting `main`
- Go module cache enabled via `actions/setup-go` `cache: true` to keep runs fast
- No secrets, no deploy steps, no matrix

## Capabilities

### New Capabilities

None — `ci-workflow` already exists as a spec.

### Modified Capabilities

- `ci-workflow`: Requirements change from two jobs (`build` + `test` running raw `go` commands) to a single job named `check` that delegates to `make check`. Go version pinned to `"1.22"` (not `"stable"`). Module cache enabled. Trigger remains push (all branches) + PR targeting `main`.

## Impact

- New file: `.github/workflows/ci.yml`
- No code changes
- No new dependencies
- Branch protection can reference the `check` job by name once merged
