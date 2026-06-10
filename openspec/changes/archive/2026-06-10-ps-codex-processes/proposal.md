# Proposal: ps-codex-processes

## Why

`omnisess ps` shows only live Claude Code sessions, but Codex CLI/desktop sessions run side by side on the same machine and are invisible to it. Codex provides no PID registry like `~/.claude/sessions/<PID>.json`, so a different correlation mechanism is needed — and one exists: every live codex session holds its rollout JSONL open for writing, which `lsof` can map back to the owning PID.

## What Changes

- `internal/procsnap` gains codex enumeration: candidate codex PIDs are found in the existing `ps` table snapshot, then a single `lsof` invocation maps each PID to the rollout file(s) it holds open under `~/.codex/sessions/`.
- The rollout's first line (`session_meta`) is parsed for session id, cwd, originator, version, and start time; the filename provides a fallback for id/start time.
- `procsnap.Session` gains a `Tool` field (`"claude"` / `"codex"`). `Snapshot.IsActive` becomes tool-aware in behavior docs (ID match unchanged — UUIDs don't collide across tools).
- `omnisess ps` leaf labels render the tool name instead of hardcoded `claude`; command help text updated.
- `internal/tui.ApplySnapshot` keeps its claude-only contract by filtering snapshot entries by tool (no TUI behavior change).
- Codex enumeration failures (lsof missing/erroring) degrade gracefully: warn on stderr, return claude-only results.
- Docs updated: `docs/references/codex-format.md` (live-process detection section; the file's DEFERRED status note is stale), `CLAUDE.md` procsnap description, `ARCHITECTURE.md` if codemap changes, `TESTING.md` §4.1 ps row note.

## Capabilities

### New Capabilities

- `ps-codex-detection`: live Codex session detection in `omnisess ps` — process discovery, lsof correlation, session_meta parsing, tool-aware rendering, graceful degradation.

### Modified Capabilities

(none — no existing spec covers `ps`/procsnap)

## Impact

- Code: `internal/procsnap` (new darwin codex files), `cmd/ps.go`, `internal/tui/model.go` (one-line filter), docs.
- 3+ packages touched → one-way door per `docs/process/git-workflow.md` §6: push branch + draft PR, no self-merge.
- No new Go dependencies. macOS only (matches existing procsnap scope). All reads remain read-only over `~/.codex`.
- JSON output of `ps --json` gains a `Tool` field per session (additive).
