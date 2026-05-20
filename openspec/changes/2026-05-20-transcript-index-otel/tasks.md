## 1. Extend the model

Rationale: the model carries all data the index needs. Parser does the heavy extraction once; index just persists. No new exported types from `internal/source/claude/` for the index — `model.*` is the contract.

- [ ] 1.1 Add to `model.ToolCall` in `internal/model/session.go`: `ID string`, `IsError bool`, `FilePath string`, `FileOp string` (`"read"|"write"|"edit"|""`), `FileLinesAdded int`, `FileLinesRemoved int`, `FileContentSize int`. Remove the truncation comments on `Input` and `Output` (now full).
- [ ] 1.2 Add `UsageInputTokens, UsageOutputTokens, UsageCacheCreationInputTokens, UsageCacheReadInputTokens int` fields to `model.Message` in `internal/model/session.go`
- [ ] 1.3 Update existing tests in `internal/source/claude/parser_test.go` to set/accept the new fields. Cursor/Codex/Copilot/Gemini parsers leave the new fields empty — verify no breakage

## 2. Extend the Claude parser

- [ ] 2.1 In `extractToolCalls` (`internal/source/claude/parser.go`): also capture the `id` field from each `tool_use` block; populate `ToolCall.ID`; remove the 200-character truncation of `Input` — store full JSON
- [ ] 2.2 Add `extractToolResults(content interface{}) map[string]string` that walks user-message content blocks and returns `tool_use_id → tool_result.content-as-JSON`
- [ ] 2.3 Modify `parseSessionFile` to walk lines in pairs: on a user line that follows an assistant line carrying tool_use blocks, extract `tool_result`s and assign their content back to the preceding assistant message's `ToolCall.Output` (matched by ID)
- [ ] 2.4 Add a `toolUseResult json.RawMessage \`json:"toolUseResult"\`` field to `sessionLine`. On user-typed lines that carry `toolUseResult`, parse it (it can be either a JSON object or a raw string) and prefer it over `tool_result.content` for `ToolCall.Output`
- [ ] 2.5 Parse `message.usage.{input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens}` from assistant messages and populate the new `Message.Usage*` fields
- [ ] 2.6 Add testdata fixture `internal/source/claude/testdata/tool_use_result.jsonl` exercising: Read + tool_result pair, Edit + toolUseResult with structuredPatch, Bash + toolUseResult with stdout/stderr, error tool_result (`is_error: true`), orphan tool_use (no result), assistant with `usage` field

## 3. Index package skeleton

- [ ] 3.1 Create `internal/index/index.go` — define the `Index` interface: `Open(path string) error`, `Close() error`, `EnsureSession(claudeFilePath, conversationID string, includeFullPayloads bool) error` (lazy-populate one session), `QuerySession(conversationID string) (*SessionStats, error)`, `QueryWindow(start, end time.Time, tool string) (*AggregateStats, error)`
- [ ] 3.2 Create `internal/index/schema.go` with the embedded SQL schema string (see design.md) and a `migrate(db *sql.DB) error` function
- [ ] 3.3 Create `internal/index/sqlite.go` — `sqliteIndex` struct implementing `Index`; opens `os.UserCacheDir() + /omnisess/index.sqlite` or `$OMNISESS_INDEX_PATH`; creates parent directory with `0o700`; runs `migrate` on open
- [ ] 3.4 Add `SessionStats` and `AggregateStats` model types in `internal/index/types.go` — represent the rows returned to consumers

## 4. Index writer (Claude only in this PR)

- [ ] 4.1 In `internal/index/writer_claude.go`, implement `indexClaudeSession(db, claudeFilePath, includeFullPayloads)`: parse via existing claude source `Get(id)` → `model.Session`; INSERT both `session_metadata` and `tool_calls` rows in a single transaction. One SQLite transaction per session — no global batching.
- [ ] 4.2 Move file-I/O and tool-type extraction INTO `internal/source/claude/parser.go` (populating the new `model.ToolCall` fields directly). The index writer reads from `model.ToolCall` — no new exported types from the source package.
- [ ] 4.3 Add `toolType(name string) string` → `'mcp'` if `strings.HasPrefix(name, "mcp__")`, `'skill'` if `name == "Skill"`, `'agent'` if `name == "Agent"` or `name == "TaskCreate"`, `'function'` otherwise. Cover with table-driven tests.
- [ ] 4.4 Implement file-I/O extraction in the parser: for `Read`, set `FilePath` from `input.file_path` and prefer `toolUseResult.file.filePath` (nested object) when present. For `Write`/`Edit`, set `FilePath` from `input.file_path` and prefer top-level `toolUseResult.filePath`. `Edit`: parse `toolUseResult.structuredPatch[].lines` for `+`/`-` counts (lines starting with `+` count to `FileLinesAdded`, `-` to `FileLinesRemoved`; context lines starting with ` ` ignored). `Write`: set `FileContentSize = len(input.content)`.
- [ ] 4.5 Implement `is_error` rule per design.md three-branch logic: (a) `tool_result.is_error: true`, (b) `toolUseResult` is a string starting with `"Error:"`, (c) `toolUseResult.success: false`. Populate `model.ToolCall.IsError`.
- [ ] 4.6 Implement invalidation: `EnsureSession` reads current file `(mtime, size, has_full_payloads)` from session_metadata, compares to current file stat + `wantFull`, deletes + re-inserts if `fileChanged OR payloadDowngrade` (per design.md algorithm).
- [ ] 4.7 Tests: `internal/index/writer_claude_test.go` covering the fixture matrix below (per the unbiased reviewer's coverage gate concern). One fixture per branch class — multi-fixture or sub-cases inside one fixture, your call.

### Fixture matrix (writer tests)

Each row must be covered by at least one test (table-driven or per-fixture). 100% per-package coverage is required.

| Class | Variant | Source data |
|---|---|---|
| Tool type | `mcp__*` → 'mcp' | tool_use name `mcp__claude-in-chrome__tabs_create_mcp` |
| Tool type | `Skill` → 'skill' | tool_use name `Skill` |
| Tool type | `Agent` → 'agent' | tool_use name `Agent` |
| Tool type | `TaskCreate` → 'agent' | tool_use name `TaskCreate` |
| Tool type | other → 'function' | tool_use name `Grep` |
| File I/O | Read happy | `Read` with input.file_path + toolUseResult.file.filePath (nested) |
| File I/O | Write happy | `Write` with input.{file_path, content} + toolUseResult top-level filePath |
| File I/O | Edit happy | `Edit` with input + toolUseResult with structuredPatch (mix of +/-/context) |
| File I/O | Bash (no file extraction in PR1) | `Bash` with input.command — FilePath stays empty |
| is_error | tool_result.is_error=true | Read with is_error result |
| is_error | toolUseResult bare string "Error:..." | Bash with stderr/exit-1 case |
| is_error | toolUseResult.success=false | Skill with success:false |
| is_error | no error | normal happy path |
| Cache | Hit (mtime+size unchanged) | second EnsureSession call returns 0 rows touched |
| Cache | Miss on size change | append to fixture, expect full re-index |
| Cache | Miss on mtime change | touch file, expect full re-index |
| Cache | Miss on `--full` upgrade | re-index without file change, `wantFull=true`, existing row has `has_full_payloads=0` |
| Cache | No-op on `--full` downgrade | existing row has `has_full_payloads=1`, call with `wantFull=false` → no rows touched |
| Orphan | tool_use without tool_result | last assistant block in fixture has tool_use with no following user line |
| Tokens | usage sums | 3 assistant messages with usage fields → session_metadata totals are sums |

## 5. `omnisess index` command

- [ ] 5.1 Create `cmd/index.go` — cobra command, flags: `--all`, `--tool`, `--full`, `--rebuild`
- [ ] 5.2 Implement `runIndex`: enumerate sessions via existing `source.Source.List` (filter by `--tool`), call `EnsureSession` per session (with `--rebuild` doing a delete-first)
- [ ] 5.3 Stderr progress: print every 50 sessions (for very small or very large corpora this is acceptable; progress-bar enhancement deferred)
- [ ] 5.4 Register the command in `cmd/root.go`
- [ ] 5.5 Tests: `cmd/index_test.go` — mock `Index` interface, verify `EnsureSession` is called per session; verify `--rebuild` and `--full` flags propagate; verify exit codes

## 6. `omnisess stats` command

- [ ] 6.1 Create `cmd/stats.go` — cobra command, flags: `--session`, `--window` (default `7d`), `--tool`, `--full`, `--json`
- [ ] 6.2 Validate: `--session` and `--window` mutually exclusive; default `--window 7d` if neither given
- [ ] 6.3 Implement `runStats`:
  - If `--session`: ensure that session is indexed (lazy populate via `EnsureSession`); query `Index.QuerySession`; render
  - If `--window`: parse duration (`1d`, `7d`, `24h`, …); enumerate sessions in window via `source.Source.List`; ensure each is indexed; query `Index.QueryWindow`; render aggregate
- [ ] 6.4 Implement text renderer in `cmd/stats_format.go`: header + tool-counts table + file-I/O table (using `tablewriter`)
- [ ] 6.5 Implement JSON renderer (encoding/json marshal of `SessionStats`/`AggregateStats`)
- [ ] 6.6 Register the command in `cmd/root.go`
- [ ] 6.7 Tests: `cmd/stats_test.go` — happy paths for `--session` and `--window`, error paths (unknown session, mutex conflict, invalid duration), JSON renderer output verification

## 7. Docs

- [ ] 7.1 Update `docs/references/claude-code-format.md` to document `tool_use_id`, `toolUseResult` (per-tool typed shapes: Read nested `file.filePath`, Write/Edit top-level `filePath`+`structuredPatch`, Bash `stdout`/`stderr`/`interrupted`, Skill `success`/`commandName`, Agent rich payload, bare-string error case), `usage.cache_creation_input_tokens`, `usage.cache_read_input_tokens`
- [ ] 7.2 Add a "Stats" section to `README.md` with `omnisess stats` examples and a note about `--full` opt-in for sensitive data. Include a caveat: "the index is currently used only by `omnisess stats`; future versions will route `list`/`active`/`digest` through it (issue #54)."
- [ ] 7.3 Update `CLAUDE.md` Key Invariants #4: replace "No indexing — brute-force scan" with "Index is opt-in derived cache for analytics (`omnisess stats`); core list/search/active paths remain JSONL-scan in PR1; perf routing arrives in PR2 (#54)." Update `ARCHITECTURE.md` codemap to include `internal/index/`, `cmd/stats.go`, `cmd/index.go`.

## 8. Validation gate

- [ ] 8.1 `make check` passes (fmt + vet + lint + test)
- [ ] 8.2 `make cover-check` passes (100% per-package coverage)
- [ ] 8.3 `make smoke` passes
- [ ] 8.4 Manual smoke: `omnisess index --all` followed by `omnisess stats --window 7d`; both complete with exit 0 and reasonable output on real `~/.claude/projects/` data
