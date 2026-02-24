## ADDED Requirements

### Requirement: make pr uses template as body base
`make pr` SHALL invoke `gh pr create` with `--title "$(git log -1 --format='%s')"` and `--body-file .github/pull_request_template.md`, so the PR body is seeded from the template rather than auto-populated from commit metadata.

#### Scenario: make pr with template body
- **WHEN** a contributor runs `make pr` on a non-main branch
- **THEN** the resulting GitHub PR body is pre-populated with the `.github/pull_request_template.md` content

#### Scenario: make pr preserves commit subject as title
- **WHEN** a contributor runs `make pr`
- **THEN** the PR title is set to the subject line of the most recent commit

### Requirement: PR body must populate all template sections before merge
Every PR in this repository SHALL have all sections of `.github/pull_request_template.md` filled in before it is merged. Unfilled placeholder text (e.g., `<!-- ... -->` comment stubs) SHALL be treated as incomplete by reviewers.

#### Scenario: Agent opens PR with fully populated body
- **WHEN** an agent opens a PR
- **THEN** the PR body contains non-empty content in Summary, Type of change, Test plan, OpenSpec change, and Breaking changes sections

#### Scenario: Reviewer blocks PR with empty sections
- **WHEN** a PR body has unfilled template sections
- **THEN** a reviewer SHALL request changes before approving merge

### Requirement: Agent session loop instructs PR body composition from template
The `CLAUDE.md` session loop step 7 (Commit & merge) SHALL include an explicit instruction to compose the PR body from `.github/pull_request_template.md`, filling Summary, Type, Test plan, OpenSpec, and Breaking changes sections.

#### Scenario: Agent follows session loop for PR creation
- **WHEN** an agent reaches step 7 of the session loop
- **THEN** the agent reads `.github/pull_request_template.md` and composes the PR body with all sections filled before running `gh pr create`
