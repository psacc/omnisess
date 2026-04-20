# Active Claude Process Tree

**Date**: 2026-04-20
**Status**: Implemented
**Related**: [`active-detection-investigation.md`](../reports/active-detection-investigation.md), [`../exec-plans/active/010-session-names.md`](../exec-plans/active/010-session-names.md), [`../exec-plans/active/011-tui-enhancements.md`](../exec-plans/active/011-tui-enhancements.md)

## Problem

Today `session.Active` is a global heuristic: `pgrep -x claude` AND session file mtime < 10 min. It tells you "something named claude is running somewhere", not which sessions are actually alive. There is no way to see how a session was launched (terminal, tmux pane, IDE, nested agent).

We want:
1. **Precision** — per-session active flag derived from a live PID for that session.
2. **Context** — a merged process tree of all live Claude processes, grouped by shared ancestors up to `launchd`.

Both goals share the same data source, so we build them together.

## Scope

- **Claude only**. Cursor / Codex / Gemini keep their existing mtime-based `Active` logic.
- **macOS first**. Linux / Windows: return `ErrUnsupported`; TUI falls back to the existing heuristic.

## Non-goals

- Rewriting `internal/detect/process.go` for non-claude tools.
- Tracking process lifecycle history (no persistence, no events) — the snapshot is point-in-time.
- Showing non-claude sibling processes in the tree (would clutter; out of scope).
- Cleaning stale `~/.claude/sessions/<PID>.json` files — source data is read-only.

## Data sources

Claude Code maintains `~/.claude/sessions/<PID>.json` for every live claude process (both CLI-launched and Claude-Desktop-launched). Shape:

```json
{
  "pid": 52333,
  "sessionId": "eaa9deaf-6c97-4233-9629-9c425ebf4457",
  "cwd": "/Users/paolo.sacconier/prj/psacc/ai-assistant-neko",
  "startedAt": 1776680734830,
  "version": "2.1.114",
  "kind": "interactive",
  "entrypoint": "cli"
}
```

A `name` field is added when the user runs `/rename` in the session (upstream persists this to `sessions-index.json` but the feature is buggy — see issues [#25509](https://github.com/anthropics/claude-code/issues/25509), [#47197](https://github.com/anthropics/claude-code/issues/47197)). We read it opportunistically from the PID file and do not rely on the durable store.

Two important properties of this file:

- PID files can be **stale** if a process crashed without cleanup. We verify liveness with `kill(pid, 0)` (`syscall.Kill` in Go returns `ESRCH` for dead PIDs).
- `entrypoint` distinguishes CLI sessions from Desktop-spawned agent-mode sessions. Both write here.

## Architecture

```
internal/procsnap/        ← new package (macOS impl + stub)
internal/source/claude/   ← consumes Snapshot for Active flag
internal/tui/             ← consumes Snapshot for Active flag + lineage overlay
cmd/ps.go                 ← new subcommand, prints merged tree
```

Invariant: `procsnap` imports only `internal/model`, stdlib, and os-exec. It never imports `internal/source/*`. The reverse dependency (sources read a Snapshot) is the only direction.

### Package surface

```go
package procsnap

type Session struct {
    PID        int
    SessionID  string
    Name       string       // from /rename, empty if unset
    CWD        string
    StartedAt  time.Time
    Entrypoint string       // "cli" | "claude-desktop"
    Version    string
    Ancestors  []Ancestor   // index 0 = immediate parent, last = root
}

type Ancestor struct {
    PID     int
    Command string          // comm (short name)
    Args    string          // trimmed, used to label e.g. tmux pane
}

type Snapshot struct {
    Sessions []Session
    Built    time.Time
}

// Enumerate returns the current snapshot. On unsupported platforms
// returns (empty-snapshot, ErrUnsupported).
func Enumerate() (Snapshot, error)

// IsActive returns true if the given session ID has a live PID in the snapshot.
func (s Snapshot) IsActive(sessionID string) bool
```

`ErrUnsupported` is a sentinel callers check to decide whether to fall back.

## Algorithm

```
1. Scan ~/.claude/sessions/*.json
2. For each PID file:
     - parse JSON
     - kill(pid, 0) → drop entry if not alive
3. Run `ps -Ao pid=,ppid=,comm=,args=` once
     → build map[pid]procInfo for the whole system
4. For each live session:
     - walk ancestors via PPID until PPID == 1 or not found
     - attach []Ancestor
5. Return Snapshot{Sessions, Built: time.Now()}
```

One directory scan + one `ps` invocation per snapshot. Expected <50ms on a typical laptop even with 20+ sessions.

## Display

### `omnisess ps` subcommand

Default output: merged tree, ASCII box-drawing, stopping at `launchd` (PID 1). Claude leaves annotated with name / project / entrypoint / age.

```
launchd
├─ iTerm2 (3012)
│  └─ zsh
│     └─ claude  omnisess  (c183a0f2)  cli      2m
└─ Claude.app
   ├─ claude  finn           (84db8f3f)  desktop  5m
   └─ claude  ai-assistant-neko (eaa9deaf)  desktop  3m
```

Name resolution cascade: `/rename` name (from PID file) → `model.Session.Name()` (plan 010) → project basename → qualified short ID.

Merge rule: ancestor chains that share a prefix collapse. Nested `claude → claude` (subagents spawned from an interactive claude) render as parent/child — no special case needed, falls out of the generic merge.

`--json` flag emits the raw `Snapshot` for scripting.

### TUI integration

- `session.Active` for claude rows becomes `snapshot.IsActive(session.ID)`. No more mtime heuristic for claude.
- Name column prefers the `/rename` name from the snapshot when present.
- New keybinding `l` (lineage): overlays the selected row's ancestor chain; `Esc` dismisses.
- Snapshot refreshed every 5s via `tea.Tick`. Refresh is a single `procsnap.Enumerate()` call — well under budget. `r` forces an immediate refresh.

Cursor / Codex / Gemini rows keep today's `Active` behavior.

## Cross-platform

- `procsnap_darwin.go` — real implementation.
- `procsnap_other.go` (build tag `!darwin`) — `Enumerate` returns `(Snapshot{}, ErrUnsupported)`.
- `cmd/ps.go` prints `procsnap: unsupported on this platform` and exits 0 when it sees `ErrUnsupported`.
- TUI silently falls back to the existing `detect.IsSessionActive` when `ErrUnsupported` is returned. Linux/Windows users see the same (imperfect) behavior they have today; no regression.

## Failure modes

| Case | Behavior |
| --- | --- |
| `~/.claude/sessions/` missing | Empty snapshot, no error |
| PID file unreadable / bad JSON | Skip entry, warn to stderr |
| `kill(pid, 0)` returns ESRCH | Drop entry (stale PID file, not our problem) |
| `ps` command fails | Warn to stderr, return empty snapshot |
| Orphaned process (PPID gone) | Chain stops; no error |

Never panic. Never modify source files.

## Testing

Table-driven tests with injection points:

- `psRunnerFn` — replaceable `func() ([]byte, error)` that returns fake `ps` output.
- `killFn` — replaceable `func(pid int) error` for liveness check.
- `sessionsDirFn` — replaceable `func() string` so tests point at a `testdata/` directory.

Fixtures:

- Happy path: CLI session, Desktop session, nested claude→claude.
- Stale PID file (process dead) — dropped from snapshot.
- Corrupt PID json — skipped with warning.
- Orphaned ancestor (PPID missing from ps output) — chain stops cleanly.
- `ErrUnsupported` path via build-tag split.

Coverage target: 100% per-package (matches repo gate).

## Scope estimate

| File | Lines | Kind |
| --- | --- | --- |
| `internal/procsnap/procsnap.go` | ~80 | types + Enumerate dispatch |
| `internal/procsnap/procsnap_darwin.go` | ~120 | darwin impl |
| `internal/procsnap/procsnap_other.go` | ~10 | stub |
| `internal/procsnap/procsnap_test.go` | ~200 | table-driven, fixtures |
| `cmd/ps.go` | ~100 | subcommand + tree renderer |
| `cmd/ps_test.go` | ~80 | render golden tests |
| `internal/tui/model.go` diff | ~60 | snapshot wiring, lineage overlay, tick |
| `internal/source/claude/claude.go` diff | ~20 | swap IsSessionActive for Snapshot.IsActive |
| `docs/design-docs/2026-04-20-active-claude-process-tree.md` | this file | design |

Total ~670 LOC incl. tests. No new `go.mod` dependencies.

## Dependencies and sequencing

- Plan **010-session-names** must be in for the name cascade to have its `Name()` fallback. If 010 is not yet merged at implementation time, the cascade skips that step.
- Plan **011-tui-enhancements** is independent. The lineage overlay and snapshot wiring slot in cleanly either before or after 011 lands.
- No dependency on 012-lifecycle-store.

## Open decisions (resolved)

| Decision | Choice | Note |
| --- | --- | --- |
| Subcommand name | `omnisess ps` | Familiar (docker ps / kubectl get pods analog) |
| Keep mtime-based Active for non-claude tools | Yes | procsnap is claude-only |
| TUI refresh cadence | 5s `tea.Tick` + `r` manual | Performance budget is fine |
| Include non-claude siblings in tree | No | Out of scope; reduces clutter |
| Tree root | `launchd` (PID 1) | Universal stopping point |

## References

- [`docs/reports/active-detection-investigation.md`](../reports/active-detection-investigation.md) — prior analysis of the broken heuristic
- [anthropics/claude-code#25509](https://github.com/anthropics/claude-code/issues/25509) — `/rename` doesn't persist without sessions-index
- [anthropics/claude-code#47197](https://github.com/anthropics/claude-code/issues/47197) — `/rename` lost after process exit
- [anthropics/claude-code#26240](https://github.com/anthropics/claude-code/issues/26240) — names lost on resume
