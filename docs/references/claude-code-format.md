# Claude Code — Local Data Format

## Paths

- **Global history**: `~/.claude/history.jsonl`
- **Session files**: `~/.claude/projects/<project-name>/<session-id>.jsonl`
- **Subagent files**: `~/.claude/projects/<project-name>/<session-id>/subagents/agent-<id>.jsonl`

## Project Name Encoding

Absolute path with `/` replaced by `-`, leading `-`:
`/Users/paolo/prj/foo` → `-Users-paolo-prj-foo`

## history.jsonl Format

One JSON object per line:
```json
{"display":"user prompt text...","pastedContents":{},"timestamp":1769100950948,"project":"/Users/paolo/prj/foo","sessionId":"5c3f2742-b736-4776-8848-dcaccba554d5"}
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
