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
The workflow SHALL define a job named `build` that runs `go build ./...` on `ubuntu-latest` using `go-version: "stable"`.

#### Scenario: Build job compiles the module
- **WHEN** the `build` job runs
- **THEN** `go build ./...` exits with code 0 on a clean checkout of the repository
- **THEN** a compilation error in any package causes the job to fail

---

### Requirement: CI workflow defines a test job
The workflow SHALL define a job named `test` that runs `go vet ./...` followed by `go test -race ./...` on `ubuntu-latest` using `go-version: "stable"`.

#### Scenario: Vet catches suspicious constructs
- **WHEN** the `test` job runs
- **THEN** `go vet ./...` is executed before the test command
- **THEN** a vet error causes the job to fail before tests run

#### Scenario: Tests run with race detector enabled
- **WHEN** the `test` job runs
- **THEN** `go test -race ./...` is executed
- **THEN** a test failure causes the job to fail

---

### Requirement: CI workflow checks out code before running Go steps
Both the `build` and `test` jobs SHALL include `actions/checkout@v4` as the first step before any Go command is run.

#### Scenario: Checkout step precedes Go steps
- **WHEN** the workflow YAML is read
- **THEN** `actions/checkout` appears as the first step in both `build` and `test` jobs

---

### Requirement: CI workflow sets up Go before running Go steps
Both the `build` and `test` jobs SHALL include `actions/setup-go@v5` with `go-version: "stable"` as a step before any Go command is run.

#### Scenario: Go toolchain available in both jobs
- **WHEN** the workflow YAML is read
- **THEN** `actions/setup-go` with `go-version: "stable"` appears in both `build` and `test` jobs
- **THEN** no Go version is hard-pinned to a specific minor version in the workflow file

---

### Requirement: CI workflow uses no matrix strategy
The workflow SHALL NOT define a `strategy.matrix` block. Both jobs run once per trigger event against a single OS and Go version.

#### Scenario: No matrix in workflow YAML
- **WHEN** the workflow YAML is read
- **THEN** no `matrix:` key appears at any level of the file
