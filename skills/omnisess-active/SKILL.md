---
name: active
description: |
  Show currently active AI coding sessions (Claude Code, Cursor, Codex, Gemini).
  A session is active if the underlying process is alive and the session file was modified
  within the last 2 minutes.
  Use this skill when asked about active sessions, running sessions, what is currently open,
  or which AI tools are in use right now.
  Trigger phrases: "active sessions", "running sessions", "what's open", "current sessions",
  "is claude running", "what sessions are active", "show active".
allowed-tools: Bash
argument-hint: ""
version: 1.0.0
---

# omnisess active

Show currently active AI coding sessions.

## Usage

```bash
omnisess active
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

A session is considered active when:
- The underlying process (claude, cursor, etc.) is alive in the process table
- The session file was modified less than 2 minutes ago

## Example output

```
claude:5c3f2742  ~/prj/myapp  (process alive, modified 47s ago)
```
