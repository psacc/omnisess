# transcript-index

A derived SQLite index over AI session transcripts. Stores per-session metadata and per-tool-call rows extracted from source-tool JSONL files. Field naming aligned with OpenTelemetry GenAI semantic conventions.

## ADDED Requirements

### Requirement: Index file location is platform-correct and overridable

The index database SHALL be stored at `os.UserCacheDir() + /omnisess/index.sqlite` by default. The path SHALL be overridable via the `OMNISESS_INDEX_PATH` environment variable.

#### Scenario: macOS default

- **GIVEN** a macOS host with `os.UserCacheDir()` returning `/Users/x/Library/Caches`
- **AND** `OMNISESS_INDEX_PATH` is unset
- **WHEN** the index opens
- **THEN** the database file path is `/Users/x/Library/Caches/omnisess/index.sqlite`
- **AND** the parent directory is created with `0o700` if it does not exist
- **AND** `os.Stat(parentDir).Mode().Perm()` equals `0o700`

#### Scenario: Linux with XDG_CACHE_HOME

- **GIVEN** `XDG_CACHE_HOME=/home/x/.cache`
- **AND** `OMNISESS_INDEX_PATH` is unset
- **WHEN** the index opens
- **THEN** the database file path is `/home/x/.cache/omnisess/index.sqlite`

#### Scenario: Env override

- **GIVEN** `OMNISESS_INDEX_PATH=/tmp/test-index.sqlite`
- **WHEN** the index opens
- **THEN** the database file path is `/tmp/test-index.sqlite`
- **AND** no other directory is created

### Requirement: Schema applies OTel GenAI naming where applicable

Column names in the `session_metadata` and `tool_calls` tables SHALL correspond to OpenTelemetry GenAI attribute paths with dots replaced by underscores, for every concept where an OTel attribute exists. Concepts not covered by OTel (file I/O) SHALL use the `omnisess.file.*` namespace, similarly transformed.

#### Scenario: tool-call columns

- **GIVEN** the `tool_calls` table
- **THEN** it has columns: `tool_call_id`, `tool_name`, `tool_type`, `operation_name`, `provider_name`
- **AND** these correspond to `gen_ai.tool.call.id`, `gen_ai.tool.name`, `gen_ai.tool.type`, `gen_ai.operation.name`, `gen_ai.provider.name`

#### Scenario: file-I/O columns use omnisess namespace

- **GIVEN** the `tool_calls` table
- **THEN** it has columns: `file_path`, `file_op`, `file_lines_added`, `file_lines_removed`, `file_content_size`
- **AND** these are NOT covered by OTel GenAI semconv (custom omnisess attributes)

### Requirement: Index is invalidated by source-file (mtime, size)

For each indexed session, the index SHALL store the source file's modification time (`source_file_mtime`, unix seconds) and size (`source_file_size`, bytes). A query MAY use cached rows IF AND ONLY IF the current `(mtime, size)` of the source file equals the stored pair.

#### Scenario: Cache hit

- **GIVEN** a session previously indexed
- **AND** the source JSONL on disk has unchanged mtime and size
- **WHEN** `omnisess stats --session <id>` runs
- **THEN** the index returns rows from SQLite without re-parsing the JSONL
- **AND** the wallclock time per session is < 50ms (no disk parse)

#### Scenario: Cache miss on append

- **GIVEN** a session previously indexed
- **AND** the source JSONL has grown (size increased and mtime advanced) — e.g. an active session
- **WHEN** `omnisess stats --session <id>` runs
- **THEN** existing rows for that conversation_id are deleted (cascade)
- **AND** the JSONL is fully re-parsed
- **AND** new rows are inserted in a single transaction
- **AND** the new `source_file_mtime` and `source_file_size` are stored

#### Scenario: Cache miss on rewrite

- **GIVEN** a session previously indexed
- **AND** the source JSONL has shrunk OR mtime moved backward
- **WHEN** the index runs
- **THEN** existing rows are deleted and the file is re-parsed end-to-end (same as append case)

### Requirement: Payload privacy by default

By default, `tool_calls.arguments_json` and `tool_calls.result_json` SHALL be `NULL`. They MAY be populated only when the index writer is invoked with `--full`. The `session_metadata.has_full_payloads` column records whether a given row's payloads are present (1) or omitted (0); this column participates in cache invalidation alongside `(mtime, size)`.

#### Scenario: Default index call leaves payloads NULL

- **GIVEN** an unindexed session
- **WHEN** `omnisess stats --session <id>` runs (no `--full`)
- **THEN** the resulting `tool_calls.arguments_json` and `tool_calls.result_json` rows are NULL
- **AND** `tool_calls.file_path`, `file_op`, `file_lines_added`/`removed`, `file_content_size` are populated where applicable
- **AND** `session_metadata.has_full_payloads = 0`

#### Scenario: --full populates payloads

- **GIVEN** an unindexed session
- **WHEN** `omnisess index --all --full` runs
- **THEN** `tool_calls.arguments_json` is populated with the raw JSON of `tool_use.input`
- **AND** `tool_calls.result_json` is populated with the raw JSON of `toolUseResult` (preferred) or `tool_result.content` (fallback)
- **AND** `session_metadata.has_full_payloads = 1`

#### Scenario: --full upgrade transparently re-indexes existing row

- **GIVEN** session X indexed without `--full` (payloads NULL, `has_full_payloads = 0`)
- **AND** the source JSONL has not changed (mtime, size unchanged)
- **WHEN** `omnisess stats --session X --full` runs
- **THEN** the existing rows are deleted and re-inserted in a single transaction
- **AND** `tool_calls.arguments_json` and `result_json` are populated
- **AND** `session_metadata.has_full_payloads = 1`
- **AND** no error is reported

#### Scenario: --full downgrade is a no-op (payloads persist)

- **GIVEN** session X indexed with `--full` (payloads populated, `has_full_payloads = 1`)
- **AND** the source JSONL has not changed
- **WHEN** `omnisess stats --session X` runs (no `--full`)
- **THEN** the existing rows are NOT deleted
- **AND** `tool_calls.arguments_json` and `result_json` remain populated
- **AND** `session_metadata.has_full_payloads` remains `1`

#### Scenario: --full is not preserved across file change

- **GIVEN** session X indexed with `--full` (payloads populated, `has_full_payloads = 1`)
- **AND** the source JSONL has been appended to (size and mtime have advanced)
- **WHEN** `omnisess stats --session X` runs (no `--full`)
- **THEN** existing rows are deleted and re-inserted from the new file content
- **AND** new `tool_calls` rows have `arguments_json` and `result_json` set to NULL (silent payload loss is intentional)
- **AND** `session_metadata.has_full_payloads = 0`

### Requirement: Tool-call rows are populated from Claude JSONL

The Claude source parser SHALL pair `tool_use` blocks (from assistant messages) with their matching `tool_result` (in subsequent user messages) and `toolUseResult` (sibling field on the same JSONL line as the `tool_result`) using `tool_use_id` as the join key. Each pair produces exactly one `tool_calls` row.

#### Scenario: Read tool produces file-path row

- **GIVEN** a Claude JSONL with a `tool_use` block: `{"type":"tool_use","id":"toolu_X","name":"Read","input":{"file_path":"/a/b/c.go"}}`
- **AND** a paired user message with `tool_result` for `toolu_X`
- **WHEN** the file is indexed
- **THEN** a `tool_calls` row exists with `tool_call_id='toolu_X'`, `tool_name='Read'`, `file_path='/a/b/c.go'`, `file_op='read'`, `tool_type='function'`

#### Scenario: Edit tool with structuredPatch produces line counts

- **GIVEN** a Claude JSONL with an `Edit` tool_use
- **AND** a sibling `toolUseResult` containing `{"structuredPatch":[{"lines":["+new","-old","+also-new"]}],"filePath":"/x.go"}`
- **WHEN** the file is indexed
- **THEN** the `tool_calls` row has `file_op='edit'`, `file_path='/x.go'`, `file_lines_added=2`, `file_lines_removed=1`

#### Scenario: Bash tool does NOT populate file columns in PR1

- **GIVEN** a Claude JSONL with a `Bash` tool_use: `{"input":{"command":"cat /etc/hosts"}}`
- **WHEN** the file is indexed
- **THEN** the row has `tool_name='Bash'`, `file_path=NULL`, `file_op=NULL`

#### Scenario: MCP and Skill tools get tool_type classification

- **GIVEN** tool_use blocks with names `mcp__claude-in-chrome__tabs_create_mcp`, `Skill`, `Agent`
- **WHEN** indexed
- **THEN** `tool_type` is `'mcp'`, `'skill'`, `'agent'` respectively

#### Scenario: Orphan tool_use (no result) still produces a row

- **GIVEN** a `tool_use` block with no matching `tool_result` (last call in an active session)
- **WHEN** indexed
- **THEN** a `tool_calls` row exists with `is_error=0` and `result_json=NULL`

#### Scenario: Bash error tool_result (toolUseResult is a string)

- **GIVEN** a Claude JSONL with a `Bash` tool_use
- **AND** a sibling `toolUseResult` that is a bare string starting with `"Error: Exit code 1"`
- **WHEN** indexed
- **THEN** the `tool_calls` row has `tool_name='Bash'`, `is_error=1`

#### Scenario: Skill failure (toolUseResult.success: false)

- **GIVEN** a `Skill` tool_use
- **AND** a `toolUseResult` `{"success": false, "commandName": "x"}`
- **WHEN** indexed
- **THEN** the `tool_calls` row has `tool_name='Skill'`, `tool_type='skill'`, `is_error=1`

#### Scenario: tool_result.is_error=true sets is_error regardless of toolUseResult

- **GIVEN** a tool_use paired with a `tool_result` block where `is_error: true`
- **AND** no `toolUseResult` sibling, or a `toolUseResult` whose shape does not indicate error
- **WHEN** indexed
- **THEN** `tool_calls.is_error = 1`

### Requirement: Session metadata rolls up tokens and counts

The `session_metadata` row for each conversation SHALL contain the sum of `usage.input_tokens`, `usage.output_tokens`, `usage.cache_creation_input_tokens`, `usage.cache_read_input_tokens` over all assistant messages in the session; the count of all `tool_calls` rows; and the count of `tool_calls` rows with `is_error=1`.

#### Scenario: Token totals are summed correctly

- **GIVEN** a JSONL with 3 assistant messages whose `usage.input_tokens` are 100, 200, 50
- **WHEN** indexed
- **THEN** `session_metadata.total_input_tokens = 350`

### Requirement: `omnisess stats --session <id>` returns per-session detail

The `stats --session <id>` subcommand SHALL output (when applied to a Claude session):

1. A header with conversation_id, provider, model, started/updated timestamps, total tokens (input/output/cache-create/cache-read)
2. A tool-counts table: tool_name, count, error_count, sorted by count desc
3. A file-I/O table: file_path, ops (read/write/edit counts), total lines added/removed where applicable, sorted by total ops desc

#### Scenario: Single session text output

- **GIVEN** a Claude session indexed with 5 Read calls (1 erroring), 2 Edit calls (3 lines added, 1 removed total), 1 Bash call
- **WHEN** `omnisess stats --session <id>` runs
- **THEN** stdout contains a "Tools" table with rows `Read 5 1`, `Edit 2 0`, `Bash 1 0`
- **AND** stdout contains a "Files" table only for the files touched by Read/Edit; Bash does not appear (no file_path)
- **AND** exit code is 0

#### Scenario: --json output is machine-parseable

- **GIVEN** the same session
- **WHEN** `omnisess stats --session <id> --json` runs
- **THEN** stdout is a single JSON object with keys `conversation_id`, `provider`, `tokens`, `tools`, `files`
- **AND** stderr is empty
- **AND** the JSON parses without error

#### Scenario: Unknown session ID returns clear error

- **GIVEN** no session with id `nonexistent`
- **WHEN** `omnisess stats --session nonexistent` runs
- **THEN** exit code is non-zero
- **AND** stderr contains `session not found: nonexistent`

### Requirement: `omnisess stats --window <duration>` aggregates over time

The `stats --window <duration>` subcommand SHALL aggregate across all `session_metadata` rows whose `updated_at` falls within `[now - duration, now]`. Default window when neither `--session` nor `--window` is given is `7d`.

#### Scenario: Window aggregate

- **GIVEN** 10 indexed sessions, 7 within the last 7 days
- **WHEN** `omnisess stats --window 7d` runs
- **THEN** stdout shows aggregate tool counts and file counts across the 7 sessions
- **AND** the session count is reported

#### Scenario: Default window is 7d when neither --session nor --window provided

- **GIVEN** 10 indexed sessions, 7 within the last 7 days
- **WHEN** `omnisess stats` runs with no flags
- **THEN** the behavior is identical to `omnisess stats --window 7d`
- **AND** the session count reported equals 7

#### Scenario: --session and --window are mutually exclusive

- **WHEN** `omnisess stats --session X --window 7d` runs
- **THEN** exit code is non-zero
- **AND** stderr contains `--session and --window are mutually exclusive`

### Requirement: `omnisess index --all` bulk-indexes every session

The `index --all` subcommand SHALL walk every registered, non-stub source's sessions and ensure each is indexed (applying the cache-invalidation rule). It SHALL print progress to stderr (one line per source or one progress bar) and a summary on completion.

#### Scenario: Bulk index

- **WHEN** `omnisess index --all` runs on a host with 100 Claude sessions and no prior index
- **THEN** all 100 sessions are inserted
- **AND** stderr indicates progress
- **AND** subsequent `omnisess index --all` is a near-zero-cost no-op (cache hit on every session)

#### Scenario: --rebuild forces full re-index

- **GIVEN** sessions already indexed
- **WHEN** `omnisess index --all --rebuild` runs
- **THEN** all rows are dropped and re-inserted
