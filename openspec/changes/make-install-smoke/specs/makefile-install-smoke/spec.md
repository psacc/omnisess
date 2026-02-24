## ADDED Requirements

### Requirement: make install installs binary from local source
The Makefile SHALL provide an `install` target that runs `go install .` to build and install the omnisess binary from the local source tree into `$GOPATH/bin` (typically `~/go/bin/omnisess`).

#### Scenario: install succeeds from project root
- **WHEN** the developer runs `make install` from the project root
- **THEN** `go install .` runs and exits 0, placing the omnisess binary in `~/go/bin/omnisess`

#### Scenario: install fails if source does not compile
- **WHEN** the developer runs `make install` and the Go source has a compile error
- **THEN** the target exits non-zero with the compiler error visible on stderr

### Requirement: make smoke verifies installed binary is reachable and functional
The Makefile SHALL provide a `smoke` target that verifies the installed omnisess binary is on PATH and executes successfully by running `omnisess list --limit=1`.

#### Scenario: smoke passes when omnisess is on PATH
- **WHEN** the developer runs `make smoke` and omnisess is on PATH
- **THEN** `omnisess list --limit=1` runs, exits 0, and outputs a valid (possibly empty) session list

#### Scenario: smoke fails clearly when omnisess is not on PATH
- **WHEN** the developer runs `make smoke` and omnisess is not found on PATH
- **THEN** the target exits non-zero and prints a message instructing the developer to add `~/go/bin` to PATH (e.g., `export PATH="$PATH:$HOME/go/bin"`)

### Requirement: git-workflow.md pre-merge checklist references make smoke
The `docs/process/git-workflow.md` pre-merge checklist SHALL reference `make smoke` as the smoke test step rather than a manual command.

#### Scenario: checklist uses make smoke
- **WHEN** a developer reads the pre-merge checklist in git-workflow.md
- **THEN** the smoke test step instructs them to run `make smoke`

### Requirement: README.md getting-started includes make install and make smoke
The `README.md` getting-started section SHALL include `make install && make smoke` as part of the initial setup or verification flow.

#### Scenario: new developer follows getting-started
- **WHEN** a developer follows the README.md getting-started section
- **THEN** they encounter `make install` to install the binary and `make smoke` to verify it works
