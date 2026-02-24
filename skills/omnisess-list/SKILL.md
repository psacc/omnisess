---
name: list
description: |
  List AI coding sessions across all sources (Claude Code, Cursor, Codex, Gemini).
  Use this skill when asked to list sessions, show recent sessions, filter sessions by tool,
  or get an overview of coding activity.
  Trigger phrases: "list sessions", "show sessions", "recent sessions", "sessions from cursor",
  "sessions today", "how many sessions", "show me my sessions".
allowed-tools: Bash
argument-hint: "[--tool <name>] [--since <duration>] [--limit N] [--json]"
version: 1.0.0
---

# omnisess list

List AI coding sessions across all sources.

## Usage

```bash
omnisess list [--tool <name>] [--since <duration>] [--limit N] [--json]
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

2. Run the command, passing through all arguments:

```bash
omnisess list "$@"
```

## Examples

```bash
# List all sessions, most recent first
omnisess list

# Filter by tool
omnisess list --tool claude

# Limit output
omnisess list --limit 10

# JSON output for scripting
omnisess list --json
```
