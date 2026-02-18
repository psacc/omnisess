# 004 — v0.2 Improvements

**Status**: Not started
**Priority**: After 002-codex

## Cleanup

### Strip more Cursor markup tags
**File**: `internal/source/cursor/transcript.go` → `cleanCursorMarkup()`

Currently only strips `<user_query>` and `[Thinking]`. Also strip:
- `<external_links>...</external_links>`
- `<cursor_commands>...</cursor_commands>`
- `<manually_attached_skills>...</manually_attached_skills>`
- `<image_files>...</image_files>`
- `[Image]` markers

Use regex: `<[a-z_]+>.*?</[a-z_]+>` or explicit list.

### Fix Claude path encoding for non-home-dir projects
**File**: `internal/source/claude/parser.go` → `projectPathFromDir()`

Currently naive: `-Users-foo-bar` → `/Users/foo/bar`. Should use same greedy filesystem walk as Cursor source.

### Filter slash-command-only sessions
Sessions where the only user input is `/mcp`, `/memory`, `/slack` etc. are noise. Option: `--no-commands` flag to filter these out, or show them with a `[cmd]` tag.

## Features

### `sessions summary <tool:id>`
Generate a summary of a session. Options:
1. **Heuristic**: first user message + last assistant message + count of messages/tool calls
2. **LLM-powered**: pipe session content to an LLM for summarization (requires API key)

Start with heuristic.

### `sessions stats`
Show aggregate stats: sessions per tool, sessions per project, sessions per day/week. Data already available from list results.

### Slack MCP bridge
**Deferred**. Would shell out to Claude Code MCP tools for Slack search. Couples to Claude Code running. Consider only if local sources prove insufficient.

## Performance

### Search indexing
Current: brute-force scan (~5s for Claude, ~3s for Cursor).
If this becomes a bottleneck with more sessions:
- Build `~/.sessions/index.db` (SQLite FTS5)
- Incremental updates based on file mtime
- Not needed yet at ~200 total sessions
