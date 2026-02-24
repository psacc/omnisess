# Capability: public-readme

## Requirements

### Requirement: README file exists at repo root
A `README.md` file SHALL exist at the repository root. It SHALL be valid Markdown and SHALL render without errors on GitHub.

#### Scenario: File present after change
- **WHEN** the change is applied
- **THEN** a file named `README.md` exists at the repository root

---

### Requirement: README opens with one-line description and CI badge
The README SHALL begin with the project name `omnisess` and a one-sentence description of the tool's purpose. A GitHub Actions CI status badge SHALL appear on the first or second line, linking to the `ci.yml` workflow on the `main` branch.

#### Scenario: Description and badge visible at top
- **WHEN** the README is rendered on GitHub
- **THEN** the first visible element is the project name and description
- **THEN** a CI badge is present and links to `.github/workflows/ci.yml` on `main`

---

### Requirement: README contains install section with go install command
The README SHALL contain an **Install** section. It SHALL include the command `go install github.com/psacc/omnisess@latest` and SHALL note that the installed binary name is `omnisess`.

#### Scenario: Install command is present and correct
- **WHEN** the README is read
- **THEN** the Install section contains `go install github.com/psacc/omnisess@latest`
- **THEN** the Install section states the resulting binary is named `omnisess`

#### Scenario: Local build alternative documented
- **WHEN** the README is read
- **THEN** the Install section includes an alternative `go build -o omnisess .` command for local development

---

### Requirement: README contains quick-start section with command examples
The README SHALL contain a **Quick start** section showing at least 3 runnable command examples: `omnisess list`, `omnisess search "<query>"`, and `omnisess active`. Each example SHALL show representative output.

#### Scenario: Three core commands shown with output
- **WHEN** the README quick-start section is read
- **THEN** examples for `omnisess list`, `omnisess search`, and `omnisess active` are present
- **THEN** each example includes a sample output block

---

### Requirement: README contains supported sources table
The README SHALL contain a **Supported sources** section with a Markdown table listing: Claude Code, Cursor, Codex, and Gemini. Each row SHALL include the source name and its implementation status (e.g., full, stub).

#### Scenario: Four sources listed with status
- **WHEN** the supported sources table is read
- **THEN** rows for Claude Code, Cursor, Codex, and Gemini are present
- **THEN** each row indicates the current implementation status

---

### Requirement: README contains commands reference table
The README SHALL contain a **Commands** section with a table listing all top-level subcommands: `list`, `search`, `active`, `show`, `tui`. Each row SHALL include the command name and a one-line description.

#### Scenario: All five commands listed
- **WHEN** the commands table is read
- **THEN** rows for `list`, `search`, `active`, `show`, and `tui` are present
- **THEN** each row has a non-empty description

---

### Requirement: README contains contributing section
The README SHALL contain a **Contributing** section describing the contribution workflow: fork the repo, create a branch, run `make check`, open a pull request.

#### Scenario: Contributing workflow documented
- **WHEN** the contributing section is read
- **THEN** the steps fork → branch → `make check` → PR are present
- **THEN** `make check` is identified as the required local verification step before opening a PR

---

### Requirement: README contains license section
The README SHALL contain a **License** section stating that the project is released under the MIT License and linking to the `LICENSE` file.

#### Scenario: License reference present
- **WHEN** the license section is read
- **THEN** "MIT" is mentioned
- **THEN** a link or reference to the `LICENSE` file is present
