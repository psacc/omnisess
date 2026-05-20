## Why

Issue #54 surfaced that `list`, `active`, `digest`, `search` take 27–46s on a real multi-thousand-session corpus because every command brute-force re-parses every JSONL across all sources. The root invariant — "no indexing, brute-force scan, optimize when slow" (CLAUDE.md Key Invariants #4) — explicitly anticipated this point.

Independently, today there is no way to ask "what tools did Claude call in this session" or "how many files were read/written this week" — `Message.ToolCall.Output` exists in the model but is never populated by the Claude parser, and the rich `toolUseResult` payload from the JSONL is ignored entirely.

These two needs converge on the same architectural change: build a derived index over transcript files. This proposal scopes that index, defines the OpenTelemetry GenAI–aligned schema, and ships a new `omnisess stats` command. A follow-up PR routes existing commands through the index (closing #54).

## What Changes

### Parser (Claude only in this PR)

- Extend `model.ToolCall` with `ID` field (stable join key to tool_result / toolUseResult)
- Lift the 200-character truncation of `tool_use.input` (file paths can exceed 200ch under deep worktrees)
- Walk `user` lines for `tool_result` blocks; pair with `tool_use_id`
- Capture the sibling `toolUseResult` JSONL field — the rich typed payload with `structuredPatch`, `filePath`, `success`, `is_error`, `stdout`/`stderr`
- Capture `usage.input_tokens`, `usage.output_tokens`, `usage.cache_creation_input_tokens`, `usage.cache_read_input_tokens` from assistant messages
- Add fixture files under `internal/source/claude/testdata/` exercising tool_use/tool_result pairing, structured patches, error results

### New `internal/index/` package

- SQLite database at `os.UserCacheDir() + /omnisess/index.sqlite`; override via env `OMNISESS_INDEX_PATH`
- Schema: `tool_calls` (per-tool-call row) + `session_metadata` (per-session aggregates) — OTel GenAI attribute names (`tool_name`, `tool_call_id`, `provider_name`, `request_model`, `usage_input_tokens`, …) with `omnisess.file.*` extensions for file I/O not covered by OTel
- Invalidation key: `(file_mtime, file_size)`. Full reparse on mismatch — no incremental append parsing in v1
- Lazy populate: index is built on first read for a session in the query window
- Bulk pre-warm: `omnisess index --all` walks every Claude session
- Metadata-only payload by default; opt-in to full `arguments_json`/`result_json` via `--full` flag on both `index` and `stats`

### New `omnisess stats` command

- `omnisess stats --session <id>` — single session detail: tool counts table, file I/O list (path, op, lines added/removed where structured patch is present), token totals
- `omnisess stats --window 7d` — aggregate across sessions whose `UpdatedAt` falls in the window
- `omnisess stats --tool claude` — restrict to one source (Claude only is the only meaningful filter in this PR)
- `omnisess stats --json` — machine-readable output for piping
- All flags compose: `omnisess stats --window 7d --tool claude --json`

## Capabilities

### New Capabilities
- `transcript-index`: Derived SQLite index over AI session transcripts. Defines schema with OTel GenAI–aligned field naming, invalidation strategy (`mtime + size`), lazy populate + explicit bulk command, privacy default (metadata-only), and the `omnisess stats` query surface.

### Modified Capabilities
None in this change. PR2 (separate proposal) will modify `list`, `active`, `digest` to read metadata from the index.

## Impact

- **`internal/model/session.go`**: `ToolCall` gains `ID` field. `Message` gains `UsageInputTokens`, `UsageOutputTokens`, `UsageCacheCreationInputTokens`, `UsageCacheReadInputTokens` fields. Backward-compatible additions; no field removed.
- **`internal/source/claude/parser.go`**: significant rewrite of `extractToolCalls`; new `extractToolResults` function that walks user messages; reads `toolUseResult` from the raw JSONL line; reads `usage` from assistant messages.
- **`internal/source/claude/testdata/`**: new fixtures for tool_use/tool_result pairing and rich payloads.
- **`internal/index/` (new)**: package containing `Index` interface, SQLite implementation, schema migrations, lazy populate logic, bulk-index command implementation.
- **`cmd/stats.go` (new)**: cobra command wiring.
- **`cmd/root.go`**: register new `stats` and `index` subcommands.
- **`go.mod`**: no new dependencies (`modernc.org/sqlite` already present).
- **`docs/references/claude-code-format.md`**: document the canonical `tool_use_id`, `toolUseResult` (per-tool typed shapes), `usage` fields. Currently stale.
- **`README.md`**: new section for `omnisess stats` examples.
- **No backwards-incompatible changes.** The existing `Message.ToolCall.{Name, Input, Output}` triplet remains; `Input` no longer truncates (no consumer relies on the 200ch limit — verified: only string-rendered in `cmd/show.go` which already line-wraps).

## Out of scope (PR2 or later)

- `list`/`active`/`digest`/`search` routing through index → **PR2** (closes #54)
- Bash command file-path extraction heuristics
- Cursor / Codex / Copilot / Gemini indexing
- Background indexing goroutines
- `omnisess export --otlp` (OTLP wire export)
- Incremental append parsing for active sessions
