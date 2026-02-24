# 007 — Multi-Mode Resume Support

**Status**: Not started
**Priority**: Medium (unblocks TUI usability for non-Claude sessions)
**Estimated effort**: 6-10 hours across 5 phases

## Goal

Replace the hardcoded `resumeClaude()` in `cmd/tui.go` with a strategy-pattern `Resumer` interface so the TUI can resume sessions from any tool that supports it, using tool-specific CLI commands and multiple resume modes (resume, fork, open-project).

## Scope

- New `internal/resume/` package with `Resumer` interface and registry
- Per-tool resume implementations: `internal/resume/claude/`, `internal/resume/cursor/`
- TUI keybindings for mode selection (resume vs fork)
- Degraded `open-project` fallback for non-resumable tools

## Non-Goals

- Windows support (existing code is `!windows` gated; stays that way)
- AoE/tmux attach (requires runtime process cross-ref; deferred)
- Codex/Gemini resume (stub sources; defer until sources are real)
- `--continue` mode (useful but lower priority; can be added later)
- Changing the Source interface or any source package

---

## Phase 1 — Extract Resumer Interface + Claude Impl (Pure Refactor)

**Goal**: Introduce the `Resumer` abstraction, move `resumeClaude()` behind it. Zero behavior change.

### Tasks

1.1. Create `internal/resume/resume.go`:
   - Define `Mode` type (`ModeResume`, `ModeFork`, `ModeOpen`)
   - Define `Resumer` interface: `Tool() model.Tool`, `Modes() []Mode`, `Exec(session *model.Session, mode Mode) error`
   - Implement registry: `Register(r Resumer)`, `Get(tool model.Tool) (Resumer, bool)`, `Modes(tool model.Tool) []Mode`

1.2. Create `internal/resume/claude/claude.go`:
   - Implement `Resumer` for Claude Code
   - `Modes()` returns `[ModeResume]` (fork added in Phase 2)
   - `Exec()` contains the existing `resumeClaude()` logic: `LookPath("claude")`, `os.Chdir`, `syscall.Exec` with `--resume <id>`
   - Call `resume.Register()` in `init()`
   - Build tag: `//go:build !windows`

1.3. Refactor `cmd/tui.go`:
   - Remove `resumeClaude()` function
   - After TUI selection: call `resume.Get(sess.Tool)` to obtain the resumer
   - If no resumer registered, show error (replaces the current `sess.Tool != model.ToolClaude` check)
   - Call `resumer.Exec(sess, resume.ModeResume)`

1.4. Add blank import `_ "github.com/psacc/omnisess/internal/resume/claude"` to `cmd/root.go`

1.5. Add unit tests:
   - `internal/resume/resume_test.go`: registry Register/Get/Modes round-trip
   - `internal/resume/claude/claude_test.go`: verify `Tool()` returns `model.ToolClaude`, `Modes()` includes `ModeResume`, and `Exec()` with a non-existent binary returns a clear error

### Acceptance Criteria

- `make check` passes
- `sessions tui` + select Claude session = identical behavior to current
- `sessions tui` + select Cursor session = shows "resume not supported for cursor" (same as today, but driven by registry miss instead of hardcoded tool check)
- No import cycle between `resume/` and `source/`

---

## Phase 2 — Claude Fork Mode

**Goal**: Add `--fork-session` support so users can branch a Claude conversation.

### Tasks

2.1. Update `internal/resume/claude/claude.go`:
   - Add `ModeFork` to `Modes()` return value
   - In `Exec()`, when `mode == ModeFork`: append `--fork-session` to argv (`claude --resume <id> --fork-session`)

2.2. Update unit test to verify fork argv construction (test the argv building logic by extracting it into a testable helper, or mock `syscall.Exec` via an interface).

### Acceptance Criteria

- `make check` passes
- Manual test: TUI fork action launches `claude --resume <id> --fork-session` (verified in Phase 4 when keybinding exists; for now, write a manual integration test script in `docs/`)

---

## Phase 3 — Cursor Agent Resume

**Goal**: Add `cursor agent --resume <chatId>` support.

### Tasks

3.1. **Manual validation first**: Run `cursor agent --resume <known-chat-id>` by hand. Confirm our parsed IDs from `conversation_summaries` / chat store match what the CLI expects. Document the result.

3.2. Create `internal/resume/cursor/cursor.go`:
   - Implement `Resumer` for Cursor
   - `Modes()` returns `[ModeResume]`
   - `Exec()`: `LookPath("cursor")`, `os.Chdir(session.Project)`, `syscall.Exec` with `cursor agent --resume <id>`
   - Call `resume.Register()` in `init()`
   - Build tag: `//go:build !windows`

3.3. Add blank import `_ "github.com/psacc/omnisess/internal/resume/cursor"` to `cmd/root.go`

3.4. Add unit tests in `internal/resume/cursor/cursor_test.go`: same pattern as Claude tests.

### Acceptance Criteria

- `make check` passes
- `sessions tui` + select Cursor session = execs `cursor agent --resume <id>` in correct CWD
- If Cursor CLI not installed, clear error message

---

## Phase 4 — TUI Mode Picker

**Goal**: Add keybindings so users can choose resume mode per session.

### Tasks

4.1. Update `internal/tui/model.go`:
   - Import `internal/resume` package
   - On `enter`: default to `ModeResume` (existing behavior)
   - On `f` key: set selected mode to `ModeFork`, select session, quit
   - Add `selectedMode resume.Mode` field to `Model`
   - Add `SelectedMode() resume.Mode` accessor
   - Only show `f` keybinding hint if `resume.Modes(sess.Tool)` includes `ModeFork`

4.2. Update footer dynamically:
   - Default: `j/k: navigate  enter: resume  q: quit`
   - When cursor is on a Claude session: `j/k: navigate  enter: resume  f: fork  q: quit`
   - When cursor is on a non-resumable tool: `j/k: navigate  q: quit` (enter still blocked)

4.3. Update `cmd/tui.go`:
   - Read `result.SelectedMode()` instead of hardcoding `ModeResume`
   - Pass mode to `resumer.Exec(sess, mode)`

4.4. Add TUI test cases in `internal/tui/model_test.go`:
   - `f` key on Claude session sets `ModeFork`
   - `f` key on Cursor session is ignored (no fork mode)
   - `enter` on Cursor session sets `ModeResume` (after Phase 3 registers cursor resumer)

### Acceptance Criteria

- `make check` passes
- `enter` on Claude session = resume (unchanged)
- `f` on Claude session = fork
- `f` on Cursor session = no-op
- Footer text is contextual

---

## Phase 5 — Degraded Open-Project Fallback

**Goal**: Tools without a resume CLI can still "open" the project directory in the user's `$EDITOR` or shell.

### Tasks

5.1. Create `internal/resume/fallback/fallback.go`:
   - Implement `Resumer` with `Modes() = [ModeOpen]`
   - `Tool()` is not meaningful here -- instead, register as a catch-all fallback
   - Alternative design: add `GetOrFallback(tool model.Tool) Resumer` to the registry that returns the fallback resumer when no tool-specific one exists

5.2. `Exec()` for `ModeOpen`:
   - If `$EDITOR` is set: `syscall.Exec($EDITOR, [$EDITOR, session.Project], env)`
   - Else: `syscall.Exec("/bin/sh", ["/bin/sh"], env)` with CWD set to project dir
   - Or simpler: `open <project-path>` on macOS (opens Finder/IDE), `xdg-open` on Linux

5.3. Update TUI footer: show `o: open project` for sessions where only `ModeOpen` is available.

5.4. Update `cmd/tui.go`: handle `ModeOpen` from the TUI selection.

5.5. Add unit tests for fallback resumer.

### Acceptance Criteria

- `make check` passes
- Codex/Gemini sessions in TUI show `o: open project` hint
- `o` on a Codex session opens the project directory
- Claude/Cursor sessions are unaffected (their resumers take priority)

---

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Cursor ID mismatch | Phase 3 unusable | Manual validation in 3.1 before writing code. Gate the PR on confirmed working IDs. |
| `resume` pkg imports `model` which `tui` also imports | Import cycle if TUI imports resume | `tui` imports `resume` for `Mode` type only. `resume` never imports `tui`. No cycle. |
| `syscall.Exec` is untestable | Low test confidence on exec path | Extract argv-building into a pure function; test that. Accept `Exec()` itself is integration-tested manually. |
| Fallback `open` behavior varies by OS | Confusing UX on Linux | Default to `$EDITOR` with documented fallback chain. Phase 5 is lowest priority and can be cut. |
| Adding `resume` dependency to `tui` package | Coupling increase | `Mode` is a simple string type. If coupling feels wrong, define `Mode` in `model/` instead. Two-way door. |

## Dependency Graph

```
Phase 1 (refactor) ── required by all others
  |
  +-- Phase 2 (fork mode) ── independent
  +-- Phase 3 (cursor resume) ── independent
  |
  Phase 2 + Phase 3 ── required by Phase 4 (TUI picker)
  |
  Phase 4 ── required by Phase 5 (fallback)
```

Phases 2 and 3 can be done in parallel. Phase 4 depends on both. Phase 5 depends on Phase 4.
