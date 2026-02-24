# Resume Modes Strategy

## Problem

The TUI can only resume Claude Code sessions. It uses `syscall.Exec` (process replacement), hardcodes the `claude` CLI, and silently ignores non-Claude selections. We need a multi-tool resume strategy that handles heterogeneous CLI capabilities gracefully.

## Resume Mode Matrix

| Tool   | Mode           | Viable | Effort | Command                                    | Notes                                      |
|--------|----------------|--------|--------|--------------------------------------------|--------------------------------------------|
| Claude | resume         | Yes    | Done   | `claude --resume <UUID>`                   | Requires CWD = project dir                 |
| Claude | continue       | Yes    | Low    | `claude --continue`                        | Most recent session in CWD, no ID needed   |
| Claude | fork           | Yes    | Low    | `claude --resume <UUID> --fork-session`    | Branch conversation, new session           |
| Cursor | agent-resume   | Yes    | Low    | `cursor agent --resume <chatId>`           | Need to verify parsed IDs match CLI IDs    |
| Cursor | agent-continue | Yes    | Low    | `cursor agent --continue`                  | Most recent chat                           |
| Cursor | open-project   | Yes    | Low    | `cursor <project-path>`                    | Degraded: opens IDE, no session resume     |
| Codex  | TBD            | Maybe  | Med    | Unknown                                    | Stub source; defer until source is done    |
| Gemini | TBD            | Maybe  | Med    | Unknown                                    | Stub source; defer until source is done    |
| Any    | tmux           | Yes    | Low    | `tmux new-session "claude --resume <uuid>"` | Background persistent session              |
| Any    | aoe-new        | Yes    | Low    | `aoe add -c <tool> -l <project>`           | Fresh AoE-managed session (no resume)      |
| Any    | aoe-attach     | Yes    | Low    | `aoe session attach <id>`                  | If AoE session exists for tool+project     |

## Architecture: Resumer Interface (Strategy Pattern)

```go
// internal/resume/resume.go

package resume

import "github.com/psacc/omnisess/internal/model"

// Mode describes how a session will be resumed.
type Mode string

const (
    ModeResume   Mode = "resume"    // Resume existing session (direct exec)
    ModeFork     Mode = "fork"      // Fork into new session
    ModeTmux     Mode = "tmux"      // Resume in background tmux session
    ModeAoE      Mode = "aoe"       // Open in AoE (new or attach existing)
    ModeOpen     Mode = "open"      // Degraded: open project dir only
)

// Resumer builds an exec plan for resuming a session.
type Resumer interface {
    // Tool returns which tool this resumer handles.
    Tool() model.Tool

    // Modes returns the resume modes this tool supports.
    Modes() []Mode

    // Exec replaces the current process to resume the session.
    // Returns error only if exec setup fails (on success, never returns).
    Exec(session *model.Session, mode Mode) error
}
```

**Why strategy pattern:**
- Each tool's CLI flags are different -- encapsulation is natural
- New tools register themselves (same pattern as Source)
- TUI queries available modes per tool to show/hide actions
- No switch-on-tool in calling code

**Registry (mirrors Source pattern):**

```go
var registry = map[model.Tool]Resumer{}

func Register(r Resumer) { registry[r.Tool()] = r }
func Get(tool model.Tool) (Resumer, bool) { r, ok := registry[tool]; return r, ok }
func Modes(tool model.Tool) []Mode { /* ... */ }
```

**Per-tool packages** (e.g., `internal/resume/claude/`, `internal/resume/cursor/`) call `resume.Register()` in `init()`, blank-imported from `cmd/root.go`.

## Key Design Decisions

1. **Keep `syscall.Exec`** -- process replacement is correct for "hand off to another CLI." No reason to shell out and wait.
2. **Resumer returns error, not builds command** -- keeps exec logic testable via interface mock, but the real impl still calls `syscall.Exec`.
3. **Mode is a TUI concern** -- the TUI presents available modes (resume/fork/continue) based on `Resumer.Modes()`. Default is `ModeResume`.
4. **Degraded fallback** -- tools without CLI resume get `ModeOpen` (open project dir). TUI shows a visual indicator that full resume is unavailable.

## AoE Integration Findings (2026-02-19)

AoE (v0.12.3) **cannot resume existing sessions**. Key constraints:
- `aoe add` always starts fresh -- no `--args` passthrough to inject `--resume <UUID>`
- AoE only knows sessions it created (no discovery of externally-started sessions)
- Cross-reference possible via `aoe list --json` matching on `tool` + `project_path`

**Workaround**: Direct tmux (`tmux new-session "claude --resume <uuid>"`) bypasses AoE but delivers the persistence value. AoE integration offers "open in AoE" (new session) and "attach existing AoE session" (if one exists for tool+project).

## Implementation Order

See `docs/exec-plans/active/008-aoe-resume.md` for the full plan.

1. **Extract Resumer interface + Claude impl** -- pure refactor of existing `resumeClaude()`
2. **Tmux background resume + Cursor resume** -- `ModeTmux` for persistent sessions, `cursor agent --resume`
3. **AoE integration** -- `ModeAoE` for new/attach AoE sessions with cross-ref
4. **TUI mode picker** -- keybindings for all modes with contextual footer

## Risks

- **Cursor ID mismatch**: Parsed chat IDs may not match `cursor agent --resume` expectations. Validate manually first.
- **AoE path normalization**: macOS symlinks (`/tmp` vs `/private/tmp`) break cross-reference. Use `filepath.EvalSymlinks`.
- **AoE not installed**: Graceful degradation -- `ModeAoE` hidden if `aoe` not in PATH.
- **tmux not installed**: Both `ModeTmux` and `ModeAoE` require tmux. Clear error on missing dependency.
