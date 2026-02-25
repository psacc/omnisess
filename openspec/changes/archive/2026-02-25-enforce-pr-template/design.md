## Context

The repo has a `.github/pull_request_template.md` (added in the `pr-template` change) but `make pr` uses `gh pr create --fill`, which auto-populates the PR body from commit metadata rather than loading the template. The result: every PR opened via `make pr` arrives with a body that ignores all template sections. Agents also have no explicit instruction in `CLAUDE.md` or `git-workflow.md` to fill the template when opening PRs manually.

## Goals / Non-Goals

**Goals:**
- Fix `make pr` to use the template file as the body base
- Add a rule in `git-workflow.md` §5 requiring all template sections to be populated before merge
- Add a step in `CLAUDE.md` session loop step 7 instructing agents to compose the PR body from the template

**Non-Goals:**
- CI enforcement of template completeness (future concern)
- Linting PR body content
- Multiple templates per change type

## Decisions

**Use `--body-file` instead of `--fill`**
`gh pr create --fill` populates title and body from the last commit. `--body-file .github/pull_request_template.md` loads the template as the body, leaving the agent or contributor to fill it in. Using `--title "$(git log -1 --format='%s')"` separately preserves the commit subject as title — this is the behavior `--fill` provided that is still useful, decoupled from the body population.

**Doc-only enforcement, no CI gate**
Adding a CI check to validate template completeness would require parsing PR bodies in GitHub Actions, which is fragile (e.g., checkbox state via API). The doc rule is sufficient for agent compliance; unchecked boxes remain visible to human reviewers. CI gate deferred to a separate change if evidence shows the doc rule is insufficient.

**Target both CLAUDE.md and git-workflow.md**
Agents orient from `CLAUDE.md` session loop and refer to `git-workflow.md` for PR details. Both need the instruction to achieve full coverage — one without the other leaves a gap.

## Risks / Trade-offs

- [Risk] `--body-file` opens the template verbatim; agent must overwrite the body or edit it before submitting → Mitigation: the `CLAUDE.md` instruction explicitly says to fill all sections, and agents are expected to compose the body programmatically via `gh pr create --body "$(cat <<'EOF' ... EOF)"` rather than rely on `make pr` for final submission
- [Trade-off] `make pr` with `--body-file` opens an editor in interactive mode if no body override is supplied → Acceptable; `make pr` is a convenience shortcut, not the final submission path for agents

## Open Questions

_(none)_
