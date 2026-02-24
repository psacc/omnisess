---
name: search
description: |
  Full-text search across all AI coding sessions (Claude Code, Cursor, Codex, Gemini).
  Use this skill when asked to search sessions, find a conversation about a topic,
  look up what was discussed in past sessions, or find sessions mentioning specific code or concepts.
  Trigger phrases: "search sessions", "find sessions about", "look up in sessions",
  "what did I discuss about", "find conversation about", "search my history".
allowed-tools: Bash
argument-hint: "<query>"
version: 1.0.0
---

# omnisess search

Full-text search across all AI coding sessions.

## Usage

```bash
omnisess search <query>
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

2. Run the search with the provided query (all arguments joined as the query string):

```bash
omnisess search "$*"
```

## Examples

```bash
# Search for sessions mentioning database migrations
omnisess search "database migration"

# Search for a specific function name
omnisess search "parseConfig"

# Multi-word search
omnisess search "error handling middleware"
```
