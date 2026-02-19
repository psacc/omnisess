# 008 — AoE Resume Integration

**Status**: Not started
**Priority**: High (user-requested)
**Estimated effort**: 4-6 hours across 4 phases
**Design doc**: `docs/design-docs/resume-modes.md`

## Goal

Add the ability to resume sessions via AoE (Agents of Empire) and direct tmux, giving users persistent background sessions that survive terminal close. Use the same Resumer strategy pattern from the design doc.

## Context

AoE wraps AI tool CLIs in tmux sessions. Key constraint: **AoE cannot resume existing sessions** — `aoe add` always starts fresh, with no `--args` passthrough to inject `--resume <UUID>`. Two approaches address this:

| Approach | Resume existing? | AoE-managed? | Effort |
|----------|-----------------|--------------|--------|
| Direct tmux: `tmux new-session "claude --resume <uuid>"` | Yes | No | S |
| AoE new session: `aoe add -c <tool> -l <project>` | No (fresh) | Yes | S |

We implement both. Direct tmux is the primary resume path. AoE is the "open in AoE" path.

## Scope

- `internal/resume/` package with `Resumer` interface and registry (same as 007 design)
- Claude resumer: `syscall.Exec` (existing), direct tmux, AoE new session
- Cursor resumer: `cursor agent --resume <chatId>`, AoE new session
- TUI keybindings: `Enter` = direct resume, `t` = tmux background resume, `a` = open in AoE

## Non-Goals

- Upstream AoE PR for `--args` flag (separate effort, tracked as follow-up)
- Windows support (stays `!windows` gated)
- Codex/Gemini resume (stubs; defer until sources are real)

---

## Phase 1 — Resumer Interface + Claude Direct Resume (Pure Refactor)

**Goal**: Extract resume logic into strategy pattern. No new functionality.

### Tasks

1.1. Create `internal/resume/resume.go`:
   - `Mode` type: `ModeResume`, `ModeFork`, `ModeTmux`, `ModeAoE`, `ModeOpen`
   - `Resumer` interface: `Tool() model.Tool`, `Modes() []Mode`, `Exec(session *model.Session, mode Mode) error`
   - Registry: `Register()`, `Get()`, `Modes()`

1.2. Create `internal/resume/claude/claude.go`:
   - Implement `Resumer` for Claude
   - `Modes()` returns `[ModeResume]` initially
   - `Exec(ModeResume)`: existing logic — `LookPath("claude")`, `os.Chdir`, `syscall.Exec("claude", "--resume", id)`
   - Build tag: `//go:build !windows`

1.3. Refactor `cmd/tui.go`:
   - Remove `resumeClaude()` function
   - Use `resume.Get(sess.Tool)` → `resumer.Exec(sess, resume.ModeResume)`
   - If no resumer, show error

1.4. Add blank import to `cmd/root.go`

1.5. Tests: registry round-trip, Claude `Tool()`, `Modes()`, `Exec()` with missing binary

### Acceptance Criteria

- `make check` passes
- Identical TUI behavior — Claude resume works, non-Claude shows error
- No import cycle between `resume/` and `source/`

---

## Phase 2 — Tmux Background Resume

**Goal**: Resume any session in a background tmux session that persists.

### Tasks

2.1. Add `ModeTmux` to Claude resumer `Modes()`

2.2. Implement `Exec(ModeTmux)` in Claude resumer:
   ```
   tmux new-session -d -s "sessions-<short-id>" -c <project-dir> "claude --resume <uuid>"
   ```
   Then attach: `tmux attach-session -t "sessions-<short-id>"`

2.3. Create `internal/resume/cursor/cursor.go`:
   - `Modes()`: `[ModeResume, ModeTmux]`
   - `Exec(ModeResume)`: `cursor agent --resume <chatId>` (direct)
   - `Exec(ModeTmux)`: `tmux new-session -d "cursor agent --resume <chatId>"`
   - `init()` → `resume.Register()`

2.4. Add shared tmux helper in `internal/resume/tmux.go`:
   - `ExecInTmux(sessionName, projectDir string, argv []string) error`
   - Checks `tmux` in PATH, creates session, attaches
   - Used by both Claude and Cursor resumers

2.5. Tests for tmux helper (mock exec), Cursor resumer

### Acceptance Criteria

- `make check` passes
- TUI `t` key on Claude session → opens tmux session with `claude --resume`
- TUI `t` key on Cursor session → opens tmux session with `cursor agent --resume`
- If tmux not installed, clear error

---

## Phase 3 — AoE Integration

**Goal**: Open a session in AoE (new AoE-managed session in the same project).

### Tasks

3.1. Add `ModeAoE` to both Claude and Cursor resumers

3.2. Implement shared AoE helper in `internal/resume/aoe.go`:
   - `ExecInAoE(tool, projectDir, title string) error`
   - Checks `aoe` in PATH
   - Runs `aoe add -c <tool> --title <title> --launch <project-dir>`
   - Falls back to error if AoE not installed

3.3. Cross-reference check: before creating a new AoE session, run `aoe list --json` and check if a session already exists for the same tool+project. If found, `aoe session attach <id>` instead of `aoe add`.

3.4. Handle `ModeAoE` in both Claude and Cursor `Exec()` — delegate to the shared helper

3.5. Tests for AoE helper (mock exec), cross-reference logic

### Acceptance Criteria

- `make check` passes
- TUI `a` key on any session → creates/attaches AoE session
- If AoE session already exists for tool+project → attaches instead of creating duplicate
- If AoE not installed, clear error message
- If tmux not installed (required by AoE), clear error

---

## Phase 4 — TUI Mode Picker

**Goal**: Expose resume modes as TUI keybindings with contextual help.

### Tasks

4.1. Add `selectedMode resume.Mode` field to TUI Model

4.2. Keybinding handling in Update():
   - `Enter` → `ModeResume` (direct, replaces process)
   - `t` → `ModeTmux` (background tmux, if mode available)
   - `a` → `ModeAoE` (open in AoE, if mode available)
   - `f` → `ModeFork` (Claude only, add to Claude Modes())

4.3. Dynamic footer based on selected session's tool:
   - Claude: `enter: resume  t: tmux  a: aoe  f: fork  q: quit`
   - Cursor: `enter: resume  t: tmux  a: aoe  q: quit`
   - Other: `a: aoe  q: quit` (only AoE available for tools without CLI resume)

4.4. Add `SelectedMode()` accessor, update `cmd/tui.go` to use it

4.5. Tests: keybinding → mode mapping, footer rendering per tool

### Acceptance Criteria

- `make check` passes
- Footer changes when navigating between Claude/Cursor/other sessions
- Each keybinding sets correct mode
- Unavailable modes are silently ignored (no crash)

---

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Cursor ID mismatch with `cursor agent --resume` | Phase 2 cursor resume unusable | Manual smoke test before implementation |
| AoE duplicate sessions on same project | Confusing UX | Cross-ref check in Phase 3 (aoe list --json) |
| tmux not installed | ModeTmux and ModeAoE both fail | LookPath check with clear error message |
| AoE path normalization (/tmp vs /private/tmp) | Cross-ref fails on macOS | Canonicalize paths with filepath.EvalSymlinks |
| syscall.Exec in tmux helper vs process replacement | Confusing process lifecycle | For ModeTmux: use os/exec.Command (child process), not syscall.Exec |

## Dependency Graph

```
Phase 1 (resumer interface) ── required by all
  |
  +── Phase 2 (tmux + cursor resume) ── requires Phase 1
  +── Phase 3 (AoE integration) ── requires Phase 1
  |
  Phase 2 + Phase 3 ── required by Phase 4 (TUI picker)
```

Phases 2 and 3 can be done in parallel.
