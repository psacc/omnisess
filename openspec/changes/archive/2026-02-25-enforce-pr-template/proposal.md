## Why

`make pr` bypasses the PR template by using `--fill`, which auto-populates the PR body from commit messages and leaves all template sections unfilled. Agents and contributors have no explicit instruction to use the template, so PRs arrive without Summary, Test plan, or OpenSpec traceability.

## What Changes

- `Makefile`: replace `gh pr create --fill` with `gh pr create --title "$(git log -1 --format='%s')" --body-file .github/pull_request_template.md` so the template is used as the body base
- `docs/process/git-workflow.md` §5: add rule "PR body must populate all sections of `.github/pull_request_template.md` before merge"; fix the manual `gh pr create` example to remove `--fill`
- `CLAUDE.md` step 7 (Commit & merge): add instruction to compose the PR body from `.github/pull_request_template.md`, filling Summary, Type, Test plan, OpenSpec, and Breaking changes sections

## Capabilities

### New Capabilities

- `enforce-pr-template`: Rule set and tooling fix that ensures every PR opened in this repo uses the standard template, with all sections populated, whether opened via `make pr` or manually by an agent

### Modified Capabilities

_(none — no existing spec-level requirements are changing)_

## Impact

- `Makefile`: one-line change to the `pr` target
- `docs/process/git-workflow.md`: two additions to §5 (rule + example fix)
- `CLAUDE.md`: one sentence added to step 7
- No Go source changes, no dependency changes, no API surface changes
