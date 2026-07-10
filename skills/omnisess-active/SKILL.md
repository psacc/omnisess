---
name: active
description: |
  Show currently active AI coding sessions (Claude Code, Cursor, Codex, GitHub Copilot CLI).
  A session is active if a live process is attributable to it: Claude Code via the
  ~/.claude/sessions registry (with per-session status: busy/idle/waiting/...), Codex via
  its open rollout file — the same correlation `omnisess ps` uses, so the two reconcile.
  Cursor/Copilot fall back to process-alive + recent transcript activity.
  Use this skill when asked about active sessions, running sessions, what is currently open,
  or which AI tools are in use right now.
  Trigger phrases: "active sessions", "running sessions", "what's open", "current sessions",
  "is claude running", "what sessions are active", "show active".
license: MIT
allowed-tools: Bash
argument-hint: ""
metadata:
  version: 0.12.0
---

# omnisess active

Show currently active AI coding sessions.

## Usage

```bash
omnisess active

# Agent-ergonomic output: minimal fields + aggregate + next-step hints (compact JSON)
omnisess active --axi
```

## Instructions

1. Check that the `omnisess` binary is available:

```bash
if ! command -v omnisess &>/dev/null; then
  echo "omnisess not found. Install it with:"
  echo "  go install github.com/psacc/omnisess@latest"
  exit 1
fi
```

2. Run the command:

```bash
omnisess active "$@"
```

## Notes

A session is considered active when a live process is attributable to it:
- Claude Code: a `~/.claude/sessions/<PID>.json` registry entry with a live PID; the
  registry status (busy/idle/waiting/...) is shown in the STATUS column
- Codex: a live codex process holding the session's rollout file open
- Cursor / Copilot (fallback): the tool's process is alive AND the transcript was
  modified within the last 10 minutes

The same correlation backs `omnisess ps`, so every claude/codex row here maps to a
process in the `ps` tree (cross-reference via the ID column).

## Example output

```
TOOL     ID         PROJECT      BRANCH  PREVIEW       UPDATED            STATUS
claude   5c3f2742   prj/myapp    main    fix the bug   2026-06-10 15:14   ACTIVE (busy)
```
