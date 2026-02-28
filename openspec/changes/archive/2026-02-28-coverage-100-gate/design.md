## Context

The project has five non-exempt packages below 100% per-package statement coverage: `cmd` (82.1%), `internal/tui` (93.2%), `internal/source/claude` (98.9%), `internal/source/codex` (99.2%), `internal/source/cursor` (98.2%). The coverage gate in `make cover-check` was intentionally lowered to 80% in commit `461f85a` to accommodate `cmd/`'s untested TUI and syscall paths. This change closes all gaps and restores the gate to 100%.

## Goals / Non-Goals

**Goals:**
- Every non-exempt package reaches 100% statement coverage
- CI `cover-check` job enforces threshold = 100
- Local feedback loop: warm run (`make cover-check`, test cache hit) completes in ~2 s; cold run (after a code change) completes in ~4–8 s (packages re-compile and run in parallel, dominated by `internal/resume` ~5 s and source packages ~4 s)
- Per-package iteration via `make cover-pkg PKG=./cmd/...` completes in < 1 s for most packages
- No behavior change to the `omnisess` binary

**Non-Goals:**
- Branch coverage (Go's tooling measures statements, not branches)
- Covering `main.go`, `internal/source/gemini/`, or `tools/covercheck/` (remain exempt)
- Adding integration tests that read real `~/.claude` / `~/.cursor` data in the CI suite

## Decisions

### D1 — Function-variable injection for `syscall.Exec` and `runtime.GOOS` in `cmd/tui.go`

`openProjectDir` calls `syscall.Exec` directly, which replaces the process (never returns). Two lines are permanently unreachable in unit tests. Wrapping them in package-level `var` function references (`execFn`, `goosStr`) allows tests to override them without subprocesses.

**Alternatives considered:**
- *Subprocess test (like the existing os.Exit pattern)*: Works but is slower (spawns a process) and fragile when both syscall.Exec targets (EDITOR and macOS `open`) need independent verification.
- *Build-tag guarded stub file*: Adds file complexity for a two-line problem.
- *Exempt cmd/ at 100% (add "cmd" to -exempt)*: Defeats the goal.

**Decision**: Inject via package-level vars. Idiomatic Go, zero runtime cost, no new dependencies.

### D2 — Extract `handleTUIResult` from `runTUI`

Lines 102–127 of `runTUI` (post-`p.Run()` dispatch: nil-session, ModeAoE, ModeOpen, resumer lookup, resumer.Exec) are unreachable when the Bubble Tea program cannot start (no TTY in CI). Extracting them to `handleTUIResult(tea.Model) error` lets tests call the function directly with a pre-built `tui.Model`.

**Alternatives considered:**
- *Inject `tea.NewProgram`*: The `runProgram` var already handles the `p.Run()` error path. `handleTUIResult` is additionally needed to test the six dispatch branches without a running TUI.
- *E2E / pty test*: Heavy, slow, brittle across OS environments.

**Decision**: Extract `handleTUIResult` + inject `runProgram` var. Both are needed for full coverage.

### D3 — Inject `runProgram` var for the `p.Run()` error path

`runTUI` starts Bubble Tea with `tea.NewProgram(m, tea.WithAltScreen()).Run()`. In CI (no TTY) the program fails immediately; the error path is covered only when sessions are non-nil. The existing test for this path is skipped under `-short`. Instead of removing the skip and relying on real disk data, wrap the program construction in a `runProgram` var so tests can inject a stub that returns an error deterministically (using the existing `activeSource` mock, which is already guaranteed to have sessions).

### D4 — Register `mockResumer` in `cmd_test.go` for `resumer.Exec()` dispatch

`handleTUIResult` calls `resumer.Exec(sess, mode)` via the `resume.Registry`. The two registered resumers (claude, cursor) ultimately call `syscall.Exec`. Adding a `mockResumer` (implements `resume.Resumer`, returns nil from `Exec`) registered in `cmd_test.go`'s `init()` lets the branch be exercised in-process without replacing the test binary.

### D5 — Inject `execInAoE` var for ModeAoE dispatch

`resume.ExecInAoE` calls `syscall.Exec` (via tmux). Same pattern as D1: inject via a package-level var `execInAoE = resume.ExecInAoE` so the AoE branch in `handleTUIResult` can be tested with a stub.

### D6 — Remove dead `offset < 0` guard in `clampViewport`

`m.offset` is only ever set in `clampViewport` itself:
1. `m.offset = m.cursor` (cursor ≥ 0)
2. `m.offset = m.cursor - visible + 1` (only triggers when `cursor ≥ offset + visible`; given offset ≥ 0 and visible ≥ 1, cursor ≥ 1, so result ≥ 1)

The guard `if m.offset < 0 { m.offset = 0 }` is unreachable. Removing it eliminates one untestable statement and makes the invariant explicit in a comment.

### D7 — Source packages: targeted edge-case tests

`internal/source/claude`, `codex`, `cursor` are at 98–99%. Uncovered lines are identified at implementation time by running `make cover` and reading the HTML report. Expected gaps: empty-preview branch, ambiguous-prefix error paths, glob-error handling. All are unit-testable with existing fixture infrastructure.

### D8 — `internal/tui`: four targeted tests

Missing statements: `Init()` return, `renderRow` empty-preview branch, `View` inline-message rendering, `visibleRows` `extra++` path, `visibleRows` `rows = 1` clamp. All covered by direct method calls with appropriately constructed `Model` structs.

### D9 — CI `cover-check` job: switch to `go-version-file: go.mod`

The `check` job already uses `go-version-file: go.mod`. The `cover-check` job is hardcoded to `"1.22"`. Unifying to `go-version-file` avoids version drift.

## Risks / Trade-offs

- **[Risk] Package-level vars in production code** → Tests override them non-concurrently (each test restores via `t.Cleanup`). Safe as long as no test calls `t.Parallel()` on tests that modify these vars. Mitigation: do not call `t.Parallel()` in tests that override injection vars.
- **[Risk] Mock resumer pollutes the global resume registry** → The mock uses a synthetic tool name (`test-mock-resumer`) that cannot collide with real tools. Existing tests are unaffected.
- **[Risk] `cover-check` cold-run cost** → Go 1.22+ caches test results even with `-coverprofile`; warm runs are ~2 s. Cold runs (after any code change) take ~7 s wall time due to parallel package compilation (`internal/resume` ~5 s, source packages ~4 s each). Mitigation: add `make cover-pkg PKG=<pkg>` for per-package iteration during active development; reserve the full `make cover-check` for pre-commit validation.
- **[Trade-off] `execInAoE` var couples cmd to resume internals** → Minimal: it's a single `var` pointing to a package function. If `resume.ExecInAoE` is renamed, the compiler catches it immediately.

## Open Questions

None — all decisions are resolved.
