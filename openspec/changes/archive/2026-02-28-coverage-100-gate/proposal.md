## Why

The CI coverage gate is set at 80% per-package, which was a temporary lowering introduced in commit `461f85a` to get `cmd/` passing. Five non-exempt packages are below 100%: `cmd` (82.1%), `internal/tui` (93.2%), `internal/source/claude` (98.9%), `internal/source/codex` (99.2%), `internal/source/cursor` (98.2%). Closing these gaps and raising the gate to 100% eliminates the implicit permission to leave coverage holes and ensures every new path added must be tested.

## What Changes

- Raise the `cover-check` threshold in `Makefile` from `80` → `100`
- Add function-variable injection points in `cmd/tui.go` to make `syscall.Exec` and `runtime.GOOS` calls testable without spawning subprocesses
- Extract `handleTUIResult` from `runTUI` so the post-TUI-run dispatch (mode routing, resumer lookup) can be unit-tested independently of the Bubble Tea program lifecycle
- Register a `mockResumer` in `cmd/cmd_test.go` so `resumer.Exec()` can be exercised without replacing the process
- Remove dead code `if m.offset < 0 { m.offset = 0 }` from `internal/tui/model.go` (`clampViewport`): offset cannot be negative; this guard is never reached
- Add ~20 targeted tests spread across `cmd/`, `internal/tui/`, `internal/source/claude/`, `internal/source/codex/`, `internal/source/cursor/`
- Fix CI `cover-check` job to use `go-version-file: go.mod` (consistent with the `check` job)

## Capabilities

### New Capabilities
- `coverage-gate-100`: Per-package 100% statement coverage requirement enforced in CI via `cover-check` job; defines exempt packages, threshold, and tooling contract

### Modified Capabilities
<!-- No existing spec requirement changes. The ci-workflow spec does not currently describe the cover-check job. -->

## Impact

- **`cmd/tui.go`**: Minor refactor — adds 3 package-level `var` injection points (`execFn`, `goosStr`, `runProgram`) and extracts `handleTUIResult`; no behavior change
- **`internal/tui/model.go`**: Removes 3 lines of unreachable dead code in `clampViewport`; no behavior change
- **Test files**: New test cases in `cmd/cmd_test.go`, `cmd/tui_test.go`, `internal/tui/model_test.go`, and the three source parser test files
- **`Makefile`**: Threshold 80 → 100 in `cover-check` target
- **`.github/workflows/ci.yml`**: `cover-check` job switches from `go-version: "1.22"` to `go-version-file: go.mod`
- **No API or CLI behavior changes**
