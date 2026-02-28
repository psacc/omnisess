## ADDED Requirements

### Requirement: cover-check enforces 100% per-package statement coverage
The `make cover-check` target SHALL run `go test -short -coverprofile=coverage.out ./...` followed by `go run ./tools/covercheck` with `-threshold 100`. The threshold SHALL be exactly 100 (not 80 or any other value). The following packages SHALL be exempt: the root package (`github.com/psacc/omnisess`), `gemini`, and `tools/covercheck`. All other packages SHALL meet the 100% threshold or the target SHALL exit non-zero.

#### Scenario: cover-check passes when all packages are at 100%
- **WHEN** `make cover-check` is run and every non-exempt package has 100% statement coverage
- **THEN** the command exits zero and prints `ok` for each package

#### Scenario: cover-check fails when any non-exempt package is below 100%
- **WHEN** `make cover-check` is run and any non-exempt package has less than 100% statement coverage
- **THEN** the command exits non-zero and prints `FAIL` for the offending package

#### Scenario: exempt packages are not checked
- **WHEN** `make cover-check` is run
- **THEN** `github.com/psacc/omnisess` (root), `internal/source/gemini`, and `tools/covercheck` are not reported and do not cause failure

---

### Requirement: CI cover-check job enforces the 100% threshold
The `.github/workflows/ci.yml` SHALL define a job named `cover-check` that runs on `ubuntu-latest`, sets up Go using `go-version-file: go.mod` (not a hardcoded version string), and executes `make cover-check`. The job SHALL fail if `make cover-check` exits non-zero.

#### Scenario: cover-check job uses go-version-file
- **WHEN** the workflow YAML is read
- **THEN** the `cover-check` job's `actions/setup-go` step uses `go-version-file: go.mod`, not a hardcoded version string

#### Scenario: cover-check job runs make cover-check
- **WHEN** the `cover-check` job executes
- **THEN** `make cover-check` is the run step and exits zero on a clean codebase

---

### Requirement: cmd package reaches 100% statement coverage under -short
The `cmd` package SHALL have 100% statement coverage when tests are run with `-short`. All coverage SHALL be achievable without a real TTY, without real `~/.claude` or `~/.cursor` data, and without replacing the test process via `syscall.Exec`. Testability SHALL be achieved through package-level injection variables (`execFn`, `goosStr`, `runProgram`, `execInAoE`) and a `handleTUIResult` function extracted from `runTUI`. A `make cover-pkg PKG=<pkg>` target SHALL exist for fast per-package iteration during development.

#### Scenario: p.Run() error path is covered under -short
- **WHEN** `go test -short -coverprofile=coverage.out ./cmd/...` is run
- **THEN** the error-return branch of `runTUI` (wrapping p.Run error) is executed

#### Scenario: TUI dispatch branches are covered without a real TTY
- **WHEN** `go test -short -coverprofile=coverage.out ./cmd/...` is run
- **THEN** `handleTUIResult` branches for nil session, ModeAoE, ModeOpen, no-resumer, and resumer-exec are all executed

#### Scenario: syscall.Exec lines in openProjectDir are covered
- **WHEN** `go test -short -coverprofile=coverage.out ./cmd/...` is run
- **THEN** both `execFn(editorPath, ...)` and `execFn(openPath, ...)` lines are executed (via injected no-op)

---

### Requirement: internal/tui package reaches 100% statement coverage
The `internal/tui` package SHALL have 100% statement coverage. Dead code SHALL be removed rather than worked around. Specifically, the unreachable `if m.offset < 0` guard in `clampViewport` SHALL be deleted. Tests SHALL cover: `Init()`, the empty-preview branch in `renderRow`, inline-message rendering in `View`, the `extra++` path in `visibleRows`, and the `rows = 1` clamp in `visibleRows`.

#### Scenario: Init() is exercised by a test
- **WHEN** `go test -short -coverprofile=coverage.out ./internal/tui/...` is run
- **THEN** the `Init()` method's return statement is executed

#### Scenario: empty-preview fallback is exercised
- **WHEN** `renderRow` is called with a session whose `Preview` field is empty
- **THEN** the `previewText = s.QualifiedID()` line is executed

#### Scenario: inline message rendering is exercised
- **WHEN** `View()` is called on a Model whose `message` field is non-empty
- **THEN** the message rendering block inside `View()` is executed

#### Scenario: visibleRows clamp is exercised
- **WHEN** `visibleRows()` is called on a Model whose `height` is less than the chrome line count
- **THEN** the `rows = 1` clamp line is executed

---

### Requirement: internal/source packages reach 100% statement coverage
`internal/source/claude`, `internal/source/codex`, and `internal/source/cursor` SHALL each reach 100% statement coverage. Any remaining uncovered statements (identified via `make cover`) SHALL be covered with targeted unit tests using existing fixture infrastructure (`testdata/`, `setupFakeHome` helpers). No new external dependencies SHALL be introduced.

#### Scenario: claude source reaches 100%
- **WHEN** `go test -short -coverprofile=coverage.out ./internal/source/claude/...` is run
- **THEN** the package reports 100% statement coverage

#### Scenario: codex source reaches 100%
- **WHEN** `go test -short -coverprofile=coverage.out ./internal/source/codex/...` is run
- **THEN** the package reports 100% statement coverage

#### Scenario: cursor source reaches 100%
- **WHEN** `go test -short -coverprofile=coverage.out ./internal/source/cursor/...` is run
- **THEN** the package reports 100% statement coverage
