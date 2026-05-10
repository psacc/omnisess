---
name: omnisess
description: |
  Aggregate AI coding sessions across Claude Code, Cursor, Codex, GitHub Copilot CLI, and Gemini.
  Search, list, recap, and detect active sessions from one place.
  Includes skills for listing sessions, full-text search, active session detection,
  session detail view, and structured recap/briefing.
license: MIT
allowed-tools: Bash
compatibility: "Requires Go 1.22+ (for building) or pre-built binary in PATH"
metadata:
  version: 0.5.0
---

# omnisess

Aggregate AI coding sessions across Claude Code, Cursor, Codex, GitHub Copilot CLI, and Gemini — search, list, and detect active sessions from one place.

## Prerequisites

The `omnisess` binary must be installed and in your PATH:

```bash
go install github.com/psacc/omnisess@latest
```

## Skills

| Skill | Description |
|-------|-------------|
| `omnisess-search` | Full-text search across all sessions |
| `omnisess-list` | List sessions with filtering by tool, time, and limit |
| `omnisess-active` | Show currently running sessions |
| `omnisess-ps` | Process tree of live Claude sessions with ancestor lineage (macOS) |
| `omnisess-show` | Show full detail for a single session by ID |
| `omnisess-recap` | Structured markdown briefing of sessions for a time window |
| `omnisess-skills-audit` | Classify Claude Code skills by usage (Keep / Borderline / Archive) |

## Supported Sources

| Source | Status |
|--------|--------|
| Claude Code | Full |
| Cursor | Full |
| Codex | Full |
| GitHub Copilot CLI | Full |
| Gemini | Stub |
