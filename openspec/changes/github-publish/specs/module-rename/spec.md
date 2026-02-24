## ADDED Requirements

### Requirement: go.mod declares the new module path
`go.mod` SHALL declare `module github.com/psacc/omnisess` as its module directive. The old module path `github.com/psacconier/sessions` SHALL NOT appear anywhere in `go.mod`.

#### Scenario: Module directive updated
- **WHEN** the change is applied
- **THEN** `go.mod` contains `module github.com/psacc/omnisess`
- **THEN** `go.mod` does not contain `github.com/psacconier/sessions`

---

### Requirement: All Go import paths use the new module path
Every Go source file that imports a package from the module SHALL use `github.com/psacc/omnisess/...` as the import path prefix. No source file SHALL contain an import path with `github.com/psacconier/sessions/...`.

#### Scenario: No residual old import paths
- **WHEN** `grep -r "github.com/psacconier/sessions" .` is run from the repo root
- **THEN** the command returns zero matches across all `.go` files

#### Scenario: New import paths compile successfully
- **WHEN** `go build ./...` is run after the rename
- **THEN** the command exits with code 0

---

### Requirement: No residual occurrences of old module path in any tracked file
The old string `github.com/psacconier/sessions` SHALL NOT appear in any file tracked by git, including Markdown, YAML, and shell scripts.

#### Scenario: Repo-wide grep returns zero matches
- **WHEN** `grep -r "psacconier" .` is run from the repo root (excluding `.git/`)
- **THEN** the command returns zero matches

---

### Requirement: CLAUDE.md references the new module path and binary name
`CLAUDE.md` SHALL be updated so that any reference to the module path uses `github.com/psacc/omnisess`, any reference to the binary uses `omnisess`, and any reference to the repository URL uses `github.com/psacc/omnisess`.

#### Scenario: CLAUDE.md updated
- **WHEN** `CLAUDE.md` is read after the change
- **THEN** the string `github.com/psacconier/sessions` does not appear
- **THEN** the string `github.com/psacc/omnisess` appears where the module path is referenced
- **THEN** `go build -o sessions .` references have been replaced with `go build -o omnisess .`

---

### Requirement: ARCHITECTURE.md references the new module path
`ARCHITECTURE.md` SHALL be updated so that any reference to the module path uses `github.com/psacc/omnisess`.

#### Scenario: ARCHITECTURE.md updated
- **WHEN** `ARCHITECTURE.md` is read after the change
- **THEN** the string `github.com/psacconier/sessions` does not appear

---

### Requirement: Makefile build target outputs binary named omnisess
The `build` target in `Makefile` SHALL produce a binary named `omnisess` (i.e., `go build -o omnisess .`). The `clean` target SHALL remove the `omnisess` binary, not `sessions`.

#### Scenario: build target produces omnisess binary
- **WHEN** `make build` is run
- **THEN** a file named `omnisess` is created in the repo root
- **THEN** no file named `sessions` is created by `make build`

#### Scenario: clean target removes omnisess binary
- **WHEN** `make clean` is run after `make build`
- **THEN** the `omnisess` binary is removed from the repo root
- **THEN** `Makefile` does not reference `rm -f sessions`

---

### Requirement: go test passes after rename
All existing tests SHALL pass with the renamed module path.

#### Scenario: go test green after rename
- **WHEN** `go test -race ./...` is run after the rename
- **THEN** the command exits with code 0
- **THEN** no test references the old module path at runtime

---

### Requirement: docs/ directory contains no residual old module path references
All Markdown files under `docs/` SHALL use `github.com/psacc/omnisess` wherever the module path is referenced. No file under `docs/` SHALL contain `github.com/psacconier/sessions`.

#### Scenario: docs grep returns zero matches for old path
- **WHEN** `grep -r "psacconier" docs/` is run
- **THEN** the command returns zero matches
