# 001 — Scaffold + v0.1 Implementation

**Status**: Completed
**Date**: 2026-02-18

## What Was Built

Go CLI tool `sessions` that aggregates AI coding sessions across tools. Single binary, no daemon, reads local files only.

### Working Commands

```bash
sessions list                          # all sessions, sorted by recency
sessions list --tool=cursor            # filter by tool
sessions list --since=7d               # recent sessions
sessions list --project=orders         # filter by project path substring
sessions list --limit=10               # cap results
sessions active                        # only running sessions
sessions search "hubspot"              # full-text search across content
sessions show claude:5c3f2742          # full conversation (prefix IDs work)
# --json flag works on all commands
```

### Sources Implemented

| Source | Status | Data Location | Notes |
|--------|--------|---------------|-------|
| Claude Code | Working | `~/.claude/history.jsonl` + `~/.claude/projects/<proj>/<session>.jsonl` | JSONL parsing, full content search |
| Cursor | Working | `~/.cursor/projects/*/agent-transcripts/*.txt` + `~/.cursor/ai-tracking/ai-code-tracking.db` + `~/.cursor/chats/*/*/store.db` | Transcript parsing, SQLite metadata, chat store enrichment |
| Codex | Stubbed | `~/.codex/sessions/YYYY/MM/DD/*.jsonl` | Interface implemented, returns empty. Same JSONL pattern as Claude. |
| Gemini | Stubbed | `~/.gemini/antigravity/conversations/*.pb` | Interface implemented, returns empty. Conversations are encrypted protobuf — only `gemini --list-sessions` CLI is viable. |

### Active Detection

Heuristic: `pgrep -f <tool-name>` AND session file modified < 2 minutes ago.

## Architecture

```
main.go → cmd/ → internal/source/ → internal/model/
                → internal/detect/
                → internal/output/
```

- **Source interface** (`internal/source/source.go`): `Name()`, `List()`, `Get()`, `Search()`
- Sources self-register via `init()` and blank imports in `cmd/root.go`
- Source packages never import each other
- Pure Go SQLite (`modernc.org/sqlite`), no CGO

### Dependencies

- `github.com/spf13/cobra` — CLI framework
- `modernc.org/sqlite` — pure Go SQLite (for Cursor DBs)

## Key Implementation Details

### Claude Code Parser (`internal/source/claude/`)

- `history.jsonl` is the session index — one entry per user message with `sessionId`, `project`, `timestamp` (unix ms), `display`
- Session files are per-project: `~/.claude/projects/<project-dashes>/<session-id>.jsonl`
- Project path encoding: `/Users/foo/bar` → `-Users-foo-bar` (leading dash, replace `/` with `-`)
- `message.content` is polymorphic: string OR `[{"type":"text","text":"..."}]` array
- Lines with `type: "summary"` are skipped (context compression markers)
- Deduplicates `history.jsonl` entries by sessionId (keeps earliest for StartedAt, latest for UpdatedAt)

### Cursor Parser (`internal/source/cursor/`)

- **Three data sources merged**: conversation_summaries SQLite table (metadata), agent-transcripts .txt files (content), chat store.db (session names/creation time)
- `conversation_summaries` table was empty in practice — all sessions came from transcript files
- Chat store meta is hex-encoded JSON in the `value` column, key `'0'`
- Transcript format: `user:` / `A:` / `[Tool call: <name>]` / `[Tool result]` markers
- `<user_query>` and `[Thinking]` tags are stripped from content
- **Path decoding** is the hardest part: Cursor encodes `/Users/paolo.sacconier/prj/finn/b2b-orders-api` as `Users-paolo-sacconier-prj-finn-b2b-orders-api` (dots AND slashes both become dashes)
- Solved with greedy filesystem walk (`workspace.go`): encode home dir as anchor, then at each directory level try longest matching child name first

## Known Issues / Rough Edges

1. **Claude `show` project path** uses naive decode for non-home-dir projects (same path encoding issue Cursor had, only fixed for Cursor)
2. **Some Cursor previews** still contain `<external_links>`, `<cursor_commands>`, `<manually_attached_skills>` markup tags — `cleanCursorMarkup()` only strips `<user_query>` and `[Thinking]`
3. **Search performance**: Claude search loads every session file (~5s). No indexing yet. Acceptable for ~200 sessions.
4. **Empty previews**: Some Claude sessions show `.` or `/mcp` or `/memory` as preview (these are the actual user inputs — slash commands)

## File Format References

Detailed file format specs for each tool are in:
- `docs/references/claude-code-format.md`
- `docs/references/cursor-format.md`
- `docs/references/codex-format.md`
- `docs/references/gemini-format.md`

## Build

```bash
go build -o omnisess .
# or install to PATH:
go build -o ~/bin/omnisess .
```
