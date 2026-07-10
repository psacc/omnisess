---
name: omnisess
description: |
  Aggregate AI coding sessions across Claude Code, Cursor, Codex, and GitHub Copilot CLI.
  Search, list, recap, digest, and detect active sessions from one place.
  Includes skills for listing sessions, full-text search, active session detection,
  session detail view, structured recap/briefing, Obsidian-compatible daily-note digest,
  transcript-index population, tool-call and file-activity stats, live Claude process
  tree (macOS), and Claude Code skills usage audit.
license: MIT
allowed-tools: Bash
compatibility: "Requires Go 1.22+ (for building) or pre-built binary in PATH"
metadata:
  version: 0.13.0
---

# omnisess

Aggregate AI coding sessions across Claude Code, Cursor, Codex, and GitHub Copilot CLI — search, list, and detect active sessions from one place.

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
| `omnisess-ps` | Process tree of live Claude and Codex sessions with ancestor lineage (macOS) |
| `omnisess-show` | Show full detail for a single session by ID |
| `omnisess-recap` | Structured markdown briefing of sessions for a time window |
| `omnisess-digest` | Print a calendar day's sessions as Obsidian-compatible markdown with full Q&A |
| `omnisess-stats` | Tool-call counts and file-I/O activity from the transcript index |
| `omnisess-index` | Bulk-populate the transcript SQLite cache used by `omnisess-stats` |
| `omnisess-skills-audit` | Classify Claude Code skills by usage (Keep / Borderline / Archive) |

## Supported Sources

| Source | Status |
|--------|--------|
| Claude Code | Full |
| Cursor | Full |
| Codex | Full |
| GitHub Copilot CLI | Full |
