---
name: stats
description: |
  Show tool-call counts and file-I/O activity from the transcript SQLite index.
  Aggregates Read/Write/Edit file paths, line counts, token usage, and error flags
  across a time window (default 7d) or for a single session.
  Use this skill when asked which tools were used most, which files saw the most
  activity, how many tokens were spent, or to inspect a single session's
  tool-call summary.
  Trigger phrases: "session stats", "tool stats", "what tools did I use",
  "tool usage", "files touched", "token usage", "stats for last week",
  "summarize tool calls", "session tool breakdown".
license: MIT
allowed-tools: Bash
argument-hint: "[--session <id>] [--window <duration>] [--tool <name>] [--full] [--json]"
metadata:
  version: 0.12.0
---

# omnisess stats

Aggregate tool counts and file-I/O activity from the transcript SQLite index.

## Usage

```bash
omnisess stats [--session <id>] [--window <duration>] [--tool <name>] [--full] [--json]
```

- `--session` and `--window` are mutually exclusive.
- Without `--session`, aggregates over a time window (default `7d`).
- `--tool` filters to one source (currently only `claude` is supported).
- `--full` is required to surface argument/result payloads — and only works if
  the index was previously populated with `omnisess index --all --full`.
- `--json` emits machine-readable output suitable for `jq`.

## Prerequisites

The transcript index is populated lazily on first use, but for bulk runs it is
faster to seed it once:

```bash
omnisess index --all
```

The index lives at `os.UserCacheDir() + /omnisess/index.sqlite` by default;
override via `OMNISESS_INDEX_PATH`.

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
omnisess stats "$@"
```

## Examples

```bash
# Last 7 days, default window
omnisess stats

# Last 30 days
omnisess stats --window 30d

# Single session detail
omnisess stats --session claude:5c3f2742

# JSON for scripting
omnisess stats --window 7d --json | jq .

# Capture full arguments + result payloads (privacy-sensitive)
omnisess index --all --full
omnisess stats --session claude:5c3f2742 --full
```
