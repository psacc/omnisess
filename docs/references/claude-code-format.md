# Claude Code — Local Data Format

## Paths

- **Global history**: `~/.claude/history.jsonl`
- **Session files**: `~/.claude/projects/<project-name>/<session-id>.jsonl`
- **Subagent files**: `~/.claude/projects/<project-name>/<session-id>/subagents/agent-<id>.jsonl`

## Project Name Encoding

Absolute path with `/` replaced by `-`, leading `-`:
`/Users/example/prj/foo` → `-Users-example-prj-foo`

## history.jsonl Format

One JSON object per line:
```json
{"display":"user prompt text...","pastedContents":{},"timestamp":1769100950948,"project":"/Users/example/prj/foo","sessionId":"5c3f2742-b736-4776-8848-dcaccba554d5"}
```

- `timestamp`: Unix epoch milliseconds
- `display`: user's prompt text
- `project`: absolute path to project directory
- `sessionId`: UUID, matches filename in projects dir

## Session JSONL Format

One JSON object per line. Key fields:

### User message
```json
{"type":"user","message":{"role":"user","content":"the user prompt"},"uuid":"...","timestamp":"2026-02-18T11:31:44.598Z","sessionId":"...","cwd":"/path/to/project","gitBranch":"main","version":"2.1.41"}
```

### Assistant message
```json
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response text"}]},"uuid":"...","timestamp":"2026-02-18T11:31:50.123Z","model":"claude-opus-4-6","costUSD":0.05,"durationMs":5000,"stopReason":"end_turn"}
```

### Notes
- `message.content` can be a string OR an array of content blocks `[{"type":"text","text":"..."}]`
- Assistant messages include `model`, `costUSD`, `durationMs`
- Tool use appears as content blocks with `type: "tool_use"` and `type: "tool_result"`
- Lines may also have `type: "summary"` (context compression markers) — skip these for display

## Tool use, tool results, and `toolUseResult`

Tool calls travel across **two JSONL lines** plus an optional rich sibling field.

### `tool_use` (on assistant lines)

Inside `message.content[]`:

```json
{"type":"tool_use","id":"toolu_01ABC...","name":"Read","input":{"file_path":"/path/to/file"}}
```

Stable fields:

- `id` — `toolu_*`; the join key for matching `tool_result`
- `name` — e.g. `Read`, `Write`, `Edit`, `Bash`, `Grep`, `Glob`, `Skill`, `Agent`, `TaskCreate`, `mcp__<server>__<tool>`
- `input` — tool-specific. `Read`/`Write`/`Edit` carry `file_path`; `Write` also `content`; `Bash` carries `command`.

### `tool_result` (on the following user line)

Inside `message.content[]`:

```json
{"type":"tool_result","tool_use_id":"toolu_01ABC...","content":"...","is_error":false}
```

`content` can be a string or an array of blocks. `is_error: true` flags failures.

### `toolUseResult` (sibling field, same JSONL line as `tool_result`)

The undocumented but stable rich payload. Same line as the matching `tool_result`. Shape depends on the tool:

| Tool | `toolUseResult` shape |
|---|---|
| `Read` | `{"type":"text","file":{"filePath":"…","content":"…","numLines":N,"startLine":1,"totalLines":N}}` |
| `Write` | `{"type":"create","filePath":"…","content":"…","structuredPatch":[...],"originalFile":null,"userModified":false}` |
| `Edit` | `{"filePath":"…","oldString":"…","newString":"…","structuredPatch":[{"oldStart":N,"oldLines":M,"newStart":N,"newLines":M,"lines":[" ctx","+added","-removed"]}],"originalFile":"…","userModified":false,"replaceAll":false}` |
| `Bash` | `{"stdout":"…","stderr":"…","interrupted":false,"isImage":false,"noOutputExpected":false}` |
| `Skill` | `{"success":true|false,"commandName":"…"}` |
| `Agent` | rich object: `{"status":"completed","prompt":"…","agentId":"…","agentType":"…","content":[…],"totalDurationMs":N,"totalTokens":N,"totalToolUseCount":N,"usage":{…},"toolStats":{…}}` |
| Error (Bash exit≠0, permission denial, etc.) | bare JSON string, e.g. `"Error: Exit code 1\nfailed output here"` |

### Error detection (omnisess rule)

omnisess flags a tool call as `is_error=true` when any of:

1. `tool_result.is_error == true`
2. `toolUseResult` is a bare JSON string whose value starts with `"Error:"`
3. `toolUseResult.success == false` (observed for `Skill`)

### Structured patch line counting

For `Edit`, omnisess counts `toolUseResult.structuredPatch[].lines` entries:

- prefix `+` → `file_lines_added`
- prefix `-` → `file_lines_removed`
- prefix ` ` (context) → ignored

## `usage` on assistant messages

```json
"usage": {
    "input_tokens": 6,
    "output_tokens": 924,
    "cache_creation_input_tokens": 39534,
    "cache_read_input_tokens": 17511,
    ...
}
```

omnisess sums these across all assistant messages to populate
`session_metadata.total_*_tokens` in the transcript index.
