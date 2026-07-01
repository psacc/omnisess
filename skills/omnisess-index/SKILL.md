---
name: index
description: |
  Bulk-populate the omnisess transcript SQLite cache by walking every session
  of every registered (non-stub) source. This is the index that `omnisess stats`
  reads from. Without it, stats will still work but lazy-indexes on first
  query, which is slow on a large corpus.
  Use this skill when asked to build/rebuild the transcript index, seed the
  cache, prepare for stats queries, or capture full tool arguments and result
  payloads for inspection.
  Trigger phrases: "index sessions", "build the index", "populate the cache",
  "seed the transcript index", "rebuild stats", "prepare stats", "index --all",
  "capture tool arguments".
license: MIT
allowed-tools: Bash
argument-hint: "--all [--full] [--rebuild]"
metadata:
  version: 0.11.0
---

# omnisess index

Bulk-populate the transcript SQLite cache used by `omnisess stats`.

## Usage

```bash
omnisess index --all [--full] [--rebuild]
```

- `--all` is required (other modes are not yet implemented).
- `--full` captures tool-use arguments and result payloads. This is
  privacy-sensitive: Bash command strings and Write/Edit file content may
  contain secrets. Treat the cache file as user-private data.
- `--rebuild` drops existing rows for each session before re-indexing.
  Use this after a schema change or to recover from a corrupt cache.

The index file lives at `os.UserCacheDir() + /omnisess/index.sqlite` by
default; override via `OMNISESS_INDEX_PATH`.

## When you need this

You don't, strictly — `omnisess stats` lazy-indexes on first query. But:

- One bulk run up front is much faster than indexing during interactive use.
- `--full` only makes per-session data available *if it was captured at
  index time*. `omnisess stats --full` cannot retroactively populate
  arguments/results that weren't recorded.

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
omnisess index "$@"
```

## Examples

```bash
# Seed the cache with metadata-only rows (default)
omnisess index --all

# Capture full arguments and result payloads (privacy-sensitive)
omnisess index --all --full

# Drop existing rows and re-index
omnisess index --all --rebuild
```
