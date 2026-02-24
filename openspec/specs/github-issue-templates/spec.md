# Capability: github-issue-templates

## Requirements

### Requirement: Issue template directory exists
The directory `.github/ISSUE_TEMPLATE/` SHALL exist in the repository after the change is applied.

#### Scenario: Directory present after change
- **WHEN** the change is applied
- **THEN** the directory `.github/ISSUE_TEMPLATE/` exists

---

### Requirement: Bug report template exists as YAML form
A file `.github/ISSUE_TEMPLATE/bug_report.yml` SHALL exist. It SHALL be valid YAML and SHALL be recognized by GitHub as an issue form template (contains `name:`, `description:`, and `body:` keys).

#### Scenario: File present and valid
- **WHEN** the change is applied
- **THEN** `.github/ISSUE_TEMPLATE/bug_report.yml` exists
- **THEN** the file contains `name:`, `description:`, and `body:` keys at the top level

#### Scenario: Bug template appears in GitHub new-issue chooser
- **WHEN** a user opens a new issue on the GitHub repository
- **THEN** the bug report template is listed as a selectable option

---

### Requirement: Bug report template collects required diagnostic fields
The bug report template SHALL include the following fields: description of the bug (textarea), steps to reproduce (textarea), expected behavior vs. actual behavior (textarea), OS and Go version (input), and `omnisess` version (input).

#### Scenario: All five fields present in bug template
- **WHEN** the bug report YAML is read
- **THEN** a textarea labeled "Description" or equivalent is present
- **THEN** a textarea for reproduction steps is present
- **THEN** a textarea for expected vs. actual behavior is present
- **THEN** an input for OS and Go version is present
- **THEN** an input for `omnisess` version is present

---

### Requirement: Feature request template exists as YAML form
A file `.github/ISSUE_TEMPLATE/feature_request.yml` SHALL exist. It SHALL be valid YAML and SHALL be recognized by GitHub as an issue form template.

#### Scenario: File present and valid
- **WHEN** the change is applied
- **THEN** `.github/ISSUE_TEMPLATE/feature_request.yml` exists
- **THEN** the file contains `name:`, `description:`, and `body:` keys at the top level

#### Scenario: Feature template appears in GitHub new-issue chooser
- **WHEN** a user opens a new issue on the GitHub repository
- **THEN** the feature request template is listed as a selectable option

---

### Requirement: Feature request template collects problem, solution, and alternatives
The feature request template SHALL include the following fields: the problem being solved (textarea), the proposed solution (textarea), and alternatives considered (textarea).

#### Scenario: Three fields present in feature template
- **WHEN** the feature request YAML is read
- **THEN** a textarea for the problem statement is present
- **THEN** a textarea for the proposed solution is present
- **THEN** a textarea for alternatives considered is present

---

### Requirement: Both templates assign a label automatically
The bug report template SHALL assign the label `bug` automatically. The feature request template SHALL assign the label `enhancement` automatically. Both labels SHALL be declared in the template's `labels:` field.

#### Scenario: Bug label assigned on submission
- **WHEN** a user submits an issue using the bug report template
- **THEN** the issue is automatically labeled `bug`

#### Scenario: Enhancement label assigned on submission
- **WHEN** a user submits an issue using the feature request template
- **THEN** the issue is automatically labeled `enhancement`

---

### Requirement: No blank issue option presented to users
A `.github/ISSUE_TEMPLATE/config.yml` file SHALL exist and SHALL set `blank_issues_enabled: false`, preventing users from bypassing the templates.

#### Scenario: Blank issue option hidden
- **WHEN** a user opens a new issue on the GitHub repository
- **THEN** no "Open a blank issue" link is available
- **THEN** the user must choose one of the two provided templates
