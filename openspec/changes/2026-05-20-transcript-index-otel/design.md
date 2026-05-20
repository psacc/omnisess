# Design — transcript-index (OTel-aligned)

## Decision summary

| Decision | Choice | Rationale |
|---|---|---|
| Naming standard | OpenTelemetry GenAI semantic conventions | Two-way door, zero runtime cost. Maps 1:1 to Claude `tool_use.{id,name}`. Future-free OTLP export. |
| Storage backend | SQLite via `modernc.org/sqlite` | Already a project dep (no CGO). Single file = trivial backup/wipe. Query expressiveness covers `--window`, `--tool`, aggregates. |
| Index location | `os.UserCacheDir() + /omnisess/index.sqlite`; env override `OMNISESS_INDEX_PATH` | Platform-correct (macOS `~/Library/Caches/omnisess/`, Linux `$XDG_CACHE_HOME/omnisess/`). Derived data → cache, not config/data. |
| Invalidation | `(file_mtime, file_size)`; full reparse on any mismatch | Simple. Claude JSONL is functionally append-only; mtime+size catches both append and rewrite. Incremental append parsing deferred. |
| Populate | Lazy on `stats` calls; `omnisess index --all` for bulk pre-warm | Good UX (no separate setup step). Synchronous (no goroutines) → no race conditions. |
| Payload privacy | Metadata only by default; `--full` opt-in stores raw `arguments_json`/`result_json` | Bash commands and Write/Edit payloads can contain secrets and grow SQLite 10–100x. Privacy by default is the OSS norm. |
| Cross-source extensibility | OTel `provider_name` column from day 1, but only Claude populates it in this PR | Schema doesn't fork later; Copilot/Cursor are additive. |

## Schema (SQL)

```sql
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS session_metadata (
    conversation_id            TEXT PRIMARY KEY,    -- gen_ai.conversation.id (session UUID)
    provider_name              TEXT NOT NULL,       -- gen_ai.provider.name ('anthropic', …)
    request_model              TEXT,                -- gen_ai.request.model
    response_model             TEXT,                -- gen_ai.response.model
    started_at                 INTEGER,             -- unix seconds
    updated_at                 INTEGER,             -- unix seconds
    total_input_tokens         INTEGER NOT NULL DEFAULT 0,
    total_output_tokens        INTEGER NOT NULL DEFAULT 0,
    total_cache_create_tokens  INTEGER NOT NULL DEFAULT 0,
    total_cache_read_tokens    INTEGER NOT NULL DEFAULT 0,
    tool_call_count            INTEGER NOT NULL DEFAULT 0,
    error_count                INTEGER NOT NULL DEFAULT 0,
    -- invalidation keys
    source_file_path           TEXT NOT NULL,
    source_file_mtime          INTEGER NOT NULL,    -- unix seconds
    source_file_size           INTEGER NOT NULL,    -- bytes
    has_full_payloads          INTEGER NOT NULL DEFAULT 0,  -- 0 = metadata only, 1 = arguments_json/result_json populated
    indexed_at                 INTEGER NOT NULL     -- unix seconds when this row was last written
);

-- Note: tool_call_count and error_count are denormalized derivations of the tool_calls table.
-- They drift if any writer bypasses EnsureSession. Tracked as tech debt; revisit if a second writer ever exists.

CREATE INDEX IF NOT EXISTS idx_session_updated ON session_metadata(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_provider ON session_metadata(provider_name);

CREATE TABLE IF NOT EXISTS tool_calls (
    conversation_id   TEXT NOT NULL,
    tool_call_id      TEXT NOT NULL,                -- gen_ai.tool.call.id (Claude tool_use_id)
    tool_name         TEXT NOT NULL,                -- gen_ai.tool.name
    tool_type         TEXT,                         -- gen_ai.tool.type ('function'|'mcp'|'skill'|'agent')
    operation_name    TEXT NOT NULL DEFAULT 'execute_tool',  -- gen_ai.operation.name
    provider_name     TEXT NOT NULL,                -- gen_ai.provider.name
    is_error          INTEGER NOT NULL DEFAULT 0,
    ts                INTEGER NOT NULL,             -- unix seconds
    -- omnisess.file.* extensions
    file_path         TEXT,                         -- omnisess.file.path (Read/Write/Edit)
    file_op           TEXT,                         -- omnisess.file.op ('read'|'write'|'edit')
    file_lines_added  INTEGER,                      -- omnisess.file.lines_added (Edit/Write structuredPatch)
    file_lines_removed INTEGER,                     -- omnisess.file.lines_removed
    file_content_size INTEGER,                      -- omnisess.file.content_size (Write content length)
    -- payload (only populated with --full)
    arguments_json    TEXT,                         -- gen_ai.tool.call.arguments
    result_json       TEXT,                         -- gen_ai.tool.call.result
    PRIMARY KEY (conversation_id, tool_call_id),
    FOREIGN KEY (conversation_id) REFERENCES session_metadata(conversation_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_conv ON tool_calls(conversation_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_name ON tool_calls(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_calls_file ON tool_calls(file_path) WHERE file_path IS NOT NULL;
```

## Claude JSONL → schema mapping

| JSONL field | Schema column | Notes |
|---|---|---|
| filename UUID | `session_metadata.conversation_id` | |
| _(constant)_ | `session_metadata.provider_name = 'anthropic'` | |
| `message.model` (first assistant) | `session_metadata.request_model` | |
| `message.model` (any assistant) | `session_metadata.response_model` | Falls back to request_model |
| `message.usage.input_tokens` (sum) | `session_metadata.total_input_tokens` | |
| `message.usage.output_tokens` (sum) | `session_metadata.total_output_tokens` | |
| `message.usage.cache_creation_input_tokens` (sum) | `session_metadata.total_cache_create_tokens` | |
| `message.usage.cache_read_input_tokens` (sum) | `session_metadata.total_cache_read_tokens` | |
| First non-summary line `timestamp` | `session_metadata.started_at` | |
| Last non-summary line `timestamp` | `session_metadata.updated_at` | |
| `tool_use.id` | `tool_calls.tool_call_id` | |
| `tool_use.name` | `tool_calls.tool_name` | |
| `tool_use.name` classification | `tool_calls.tool_type` | `mcp__*` → `'mcp'`; `Skill` → `'skill'`; `Agent`/`TaskCreate` → `'agent'`; everything else → `'function'` |
| `tool_result.is_error` OR `toolUseResult` shape | `tool_calls.is_error` | `1` if any of: (a) `tool_result.is_error: true`; (b) `toolUseResult` is a string starting with `Error:` (observed for Bash exit-code≠0); (c) `toolUseResult.success: false` (observed only for `Skill` tool). Otherwise `0`. |
| Assistant line `timestamp` | `tool_calls.ts` | |
| `tool_use.input.file_path` (Read/Write/Edit) | `tool_calls.file_path` | Primary source. For Read prefer `toolUseResult.file.filePath` (nested object) when present; for Write/Edit prefer top-level `toolUseResult.filePath` when present |
| Tool name → op | `tool_calls.file_op` | `Read`→`read`, `Write`→`write`, `Edit`→`edit`; null otherwise |
| `toolUseResult.structuredPatch[].lines` filter `+` | `tool_calls.file_lines_added` | Edit only |
| `toolUseResult.structuredPatch[].lines` filter `-` | `tool_calls.file_lines_removed` | Edit only |
| `len(tool_use.input.content)` (Write) | `tool_calls.file_content_size` | byte length |
| `tool_use.input` (`--full`) | `tool_calls.arguments_json` | NULL by default |
| `tool_result.content` OR `toolUseResult` (`--full`) | `tool_calls.result_json` | NULL by default; prefer `toolUseResult`. When `toolUseResult` is a bare string (Bash error case), stored as JSON-quoted string |

## File path extraction (in-scope tools)

In this PR, file paths are extracted **only** from `Read`, `Write`, `Edit`. Their inputs have explicit `file_path` keys; the `toolUseResult` also exposes `filePath`. No heuristics, no false positives.

**Explicitly deferred:** `Bash` command parsing (would need a tokenizer for `cat/head/tail/vim/code/`, `>` redirects, `-o` flags — accept ~80% recall). Tracked as a follow-up.

## Invalidation algorithm

Adds a `has_full_payloads INTEGER NOT NULL DEFAULT 0` column to `session_metadata` to make `--full`-ness part of the cache key alongside `(mtime, size)`.

```
EnsureSession(file, conversation_id, wantFull):
  st <- stat(file)
  row <- SELECT source_file_mtime, source_file_size, has_full_payloads
         FROM session_metadata WHERE conversation_id = id
  fileChanged    = row IS NULL OR row.mtime != st.mtime OR row.size != st.size
  payloadDowngrade = wantFull AND row IS NOT NULL AND row.has_full_payloads = 0
  IF fileChanged OR payloadDowngrade:
    BEGIN TRANSACTION
      DELETE FROM tool_calls WHERE conversation_id = id
      DELETE FROM session_metadata WHERE conversation_id = id
      parse file end-to-end
      INSERT session_metadata (including has_full_payloads = wantFull)
      INSERT tool_calls rows (arguments_json/result_json populated only if wantFull)
    COMMIT
  ELSE:
    cache hit, no work
```

**`--full` durability:** `--full` is sticky across cache hits but NOT across file changes. If a session's JSONL is modified after a `--full` index, the next non-`--full` call will re-index without payloads (silent payload loss). This is intentional and documented; users who want durable `--full` data must re-run with `--full` after the source changes.

**`--full` upgrade:** A user invoking `omnisess stats --full --session X` against a session already indexed without `--full` triggers a transparent re-index with `--full`. No error, no surprise data loss — the existing row is replaced atomically. The `--full` flag-name itself is the warning that payloads will be persisted.

**`--full` downgrade is a no-op.** Running `omnisess stats --session X` (no `--full`) against a session already indexed with `--full` does NOT delete payloads. Payloads persist until the source file changes.

Active sessions (mtime moving forward) will re-index on every `stats` call. They are few; acceptable cost. One SQLite transaction per session — no global batching across sessions (correctness + crash-safety per session boundary).

## CLI surface (cmd/stats.go)

```
omnisess stats [flags]

Flags:
  --session string   Single session ID (with or without tool prefix)
  --window string    Time window: 1d, 7d, 30d, etc. (default if no --session)
  --tool string      Filter to one source ('claude' is the only meaningful value in PR1)
  --full             Include arguments and results payloads (requires --full also on prior index runs)
  --json             Machine-readable JSON output
```

Default with no flags = `--window 7d`. `--session` and `--window` are mutually exclusive.

## CLI surface (cmd/index.go)

```
omnisess index --all [flags]

Flags:
  --all              Walk every session of every (registered, non-stub) source and index it
  --tool string      Restrict to one source
  --full             Capture full arguments/result payloads
  --rebuild          Drop existing rows for each session before re-indexing (default uses mtime+size cache)
```

## What we do NOT do

- Do NOT import `go.opentelemetry.io/*`. No SDK, no runtime, no exporter.
- Do NOT parse `tool_result.content` MIME types — store raw, let the renderer decide.
- Do NOT redact `--full` payloads. The flag name is the warning.
- Do NOT touch `Search` in this PR — its index integration is non-trivial (snippet ranking) and out of scope.

## Open questions answered during grilling

- **Q:** Bundle perf fix with this PR? **A:** No — sequence. PR1 ships index + stats; PR2 routes existing commands.
- **Q:** Bash file paths? **A:** Deferred. Best-effort heuristics are scope creep for PR1.
- **Q:** Sensitive data default? **A:** Metadata only. `--full` is opt-in everywhere it appears.
- **Q:** Cache invalidation? **A:** `(mtime, size)` full reparse. Incremental deferred.
