## ADDED Requirements

### Requirement: PR template file exists
The repository SHALL contain a `.github/pull_request_template.md` file that GitHub auto-populates into the PR body when a contributor opens a pull request.

#### Scenario: GitHub uses the template automatically
- **WHEN** a contributor opens a new pull request on GitHub
- **THEN** the PR body is pre-populated with the content of `.github/pull_request_template.md`

### Requirement: Summary section
The template SHALL include a Summary section that prompts for up to 3 bullet points focused on WHY the change is being made, not what files were changed.

#### Scenario: Summary is why-focused
- **WHEN** a contributor fills in the Summary section
- **THEN** the prompt guides them to explain motivation, not enumerate changed files

### Requirement: Type of change section
The template SHALL include a Type of change section with checkboxes for: bug fix, new feature, chore, docs.

#### Scenario: Contributor selects change type
- **WHEN** a contributor opens a PR
- **THEN** they can check one or more type checkboxes to classify the change

### Requirement: Test plan with mandatory checkboxes
The template SHALL include a Test plan section with the following checkboxes that MUST be checked before merge:
- `make check` passes
- `make smoke` passes
- Relevant `omnisess` subcommand tested against real data

#### Scenario: Test plan is visible and actionable
- **WHEN** a contributor opens a PR
- **THEN** unchecked test plan items are visually prominent and signal incomplete validation to reviewers

### Requirement: OpenSpec traceability section
The template SHALL include an OpenSpec section where the contributor specifies the change name if the PR implements an OpenSpec change, or writes "N/A" if not applicable.

#### Scenario: OpenSpec change is traceable
- **WHEN** a PR implements an OpenSpec-tracked change
- **THEN** the change name is recorded in the PR body for cross-reference

### Requirement: Breaking changes section
The template SHALL include a Breaking changes section with a yes/no indicator and a description field that is filled in when the answer is yes.

#### Scenario: Breaking change is declared
- **WHEN** a PR introduces a breaking change
- **THEN** the contributor selects "yes" and provides a description of the breaking behavior
