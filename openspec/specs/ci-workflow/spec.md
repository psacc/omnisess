# Capability: ci-workflow

## Requirements

### Requirement: CI workflow file exists at expected path
A file named `.github/workflows/ci.yml` SHALL exist in the repository. It SHALL be valid YAML and SHALL be parsed successfully by GitHub Actions without syntax errors.

#### Scenario: File present after change
- **WHEN** the change is applied
- **THEN** `.github/workflows/ci.yml` exists

---

### Requirement: CI workflow triggers on push to all branches and PRs targeting main
The workflow's `on:` block SHALL specify `push` (all branches, no branch filter) and `pull_request` targeting `main`.

#### Scenario: Push trigger is branch-agnostic
- **WHEN** a commit is pushed to any branch
- **THEN** the CI workflow is triggered

#### Scenario: PR trigger targets main only
- **WHEN** a pull request is opened or updated with `main` as the base branch
- **THEN** the CI workflow is triggered

#### Scenario: PR targeting non-main branch does not trigger CI
- **WHEN** a pull request targets a branch other than `main`
- **THEN** the CI workflow is NOT triggered by the `pull_request` event (push still fires)

---

### Requirement: CI workflow defines a build job
The workflow SHALL define a single job named `check` (not `build` or `test`) that runs on `ubuntu-latest` using `go-version: "1.22"`. The job SHALL execute `make check` as its sole run step. The job SHALL NOT call `go build ./...`, `go vet ./...`, or `go test ./...` directly — all quality gates are delegated to the Makefile target.

#### Scenario: Single check job replaces build and test jobs
- **WHEN** the workflow YAML is read
- **THEN** a job named `check` exists
- **THEN** no job named `build` or `test` exists

#### Scenario: check job runs make check
- **WHEN** the `check` job executes
- **THEN** the step `run: make check` is present
- **THEN** no raw `go build`, `go vet`, or `go test` commands appear as separate steps

---

### Requirement: CI workflow defines a test job
The workflow SHALL NOT define a separate job named `test`. All testing is subsumed by the `check` job via `make check`.

#### Scenario: No standalone test job
- **WHEN** the workflow YAML is read
- **THEN** no job named `test` exists at any level of the file

---

### Requirement: CI workflow checks out code before running Go steps
The `check` job SHALL include `actions/checkout@v4` as the first step before any Go command is run.

#### Scenario: Checkout step precedes Go steps
- **WHEN** the workflow YAML is read
- **THEN** `actions/checkout` appears as the first step in the `check` job

---

### Requirement: CI workflow sets up Go before running Go steps
The `check` job SHALL include `actions/setup-go@v5` with `go-version: "1.22"` and `cache: true` as a step before any Go command is run.

#### Scenario: Go toolchain pinned to 1.22
- **WHEN** the workflow YAML is read
- **THEN** `actions/setup-go` appears in the `check` job with `go-version: "1.22"`
- **THEN** no `"stable"` or floating Go version string appears

#### Scenario: Go module cache is enabled
- **WHEN** the workflow YAML is read
- **THEN** `cache: true` is set on the `actions/setup-go` step

---

### Requirement: CI workflow installs golangci-lint before running make check
The `check` job SHALL include a step that installs `golangci-lint` using the official installer script before `make check` is run. This is required because `make lint` expects `golangci-lint` to be on PATH and does not install it automatically.

#### Scenario: golangci-lint install step precedes make check
- **WHEN** the workflow YAML is read
- **THEN** a step that installs `golangci-lint` appears before the `run: make check` step in the `check` job
