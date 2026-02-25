## MODIFIED Requirements

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

### Requirement: CI workflow defines a test job
The workflow SHALL NOT define a separate job named `test`. All testing is subsumed by the `check` job via `make check`.

#### Scenario: No standalone test job
- **WHEN** the workflow YAML is read
- **THEN** no job named `test` exists at any level of the file

### Requirement: CI workflow sets up Go before running Go steps
The `check` job SHALL include `actions/setup-go@v5` with `go-version: "1.22"` and `cache: true` as a step before any Go command is run.

#### Scenario: Go toolchain pinned to 1.22
- **WHEN** the workflow YAML is read
- **THEN** `actions/setup-go` appears in the `check` job with `go-version: "1.22"`
- **THEN** no `"stable"` or floating Go version string appears

#### Scenario: Go module cache is enabled
- **WHEN** the workflow YAML is read
- **THEN** `cache: true` is set on the `actions/setup-go` step

### Requirement: CI workflow installs golangci-lint before running make check
The `check` job SHALL include a step that installs `golangci-lint` using the official installer script before `make check` is run. This is required because `make lint` expects `golangci-lint` to be on PATH and does not install it automatically.

#### Scenario: golangci-lint install step precedes make check
- **WHEN** the workflow YAML is read
- **THEN** a step that installs `golangci-lint` appears before the `run: make check` step in the `check` job

## REMOVED Requirements

### Requirement: CI workflow uses no matrix strategy
**Reason**: The single `check` job already has no matrix. This requirement is now implicit and no longer needs to be stated explicitly.
**Migration**: No action needed — the constraint is preserved by the `check` job design.
