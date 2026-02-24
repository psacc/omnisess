---
name: show
description: |
  Show the full detail of a single AI coding session by its ID.
  Session IDs are in the format <tool>:<id> (e.g., claude:5c3f2742, cursor:a1b2c3d4).
  Use this skill when asked to show a specific session, inspect a session, read a session,
  or view the messages in a session.
  Trigger phrases: "show session", "open session", "inspect session", "read session",
  "show me claude:5c3f2742", "view session details", "what happened in session".
allowed-tools: Bash
argument-hint: "<tool>:<session-id>"
version: 1.0.0
---

# omnisess show

Show full detail for a single AI coding session.

## Usage

```bash
omnisess show <tool>:<session-id>
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

2. Guard for missing argument:

```bash
if [ -z "$1" ]; then
  echo "Usage: omnisess show <tool>:<session-id>"
  echo "Example: omnisess show claude:5c3f2742"
  exit 1
fi
```

3. Run the command with the provided session ID:

```bash
omnisess show "$1"
```

## Session ID format

Session IDs always use the `<tool>:<id>` format:
- `claude:5c3f2742`
- `cursor:a1b2c3d4`
- `codex:e5f6g7h8`

Use `omnisess list` to find available session IDs.

## Example

```bash
omnisess show claude:5c3f2742
```
