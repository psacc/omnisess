# pi — Local Data Format

## Paths

- **Agent dir**: `$PI_CODING_AGENT_DIR` when set, else `~/.pi/agent`
- **Sessions**: `<agent-dir>/sessions/<encoded-cwd>/<timestamp>_<session-id>.jsonl`
  - `<encoded-cwd>` is the absolute project path with the leading slash
    removed and `/`, `\` and `:` replaced by `-`, wrapped in `--`
    (e.g. `/Users/foo/prj/demo` → `--Users-foo-prj-demo--`).
  - `<timestamp>` is the session start, ISO 8601 with `:` and `.` replaced by
    `-` (e.g. `2026-01-02T03-04-05-000Z`).
  - `<session-id>` is a UUIDv7.
  - A custom `--session-dir` may place `*.jsonl` files directly under the
    sessions root; both layouts are read.

Each file is append-only JSONL.

## Header

The first line is the session header:

```json
{"type":"session","version":3,"id":"01900000-0000-7000-8000-000000000001",
 "timestamp":"2026-01-02T03:04:05.000Z","cwd":"/Users/foo/prj/demo"}
```

`parentSession` is present on forked sessions.

## Entries

Every following line has `type` plus `id` / `parentId` / `timestamp`
(ISO 8601). Entry types consumed by omnisess:

```json
{"type":"model_change","provider":"anthropic","modelId":"claude-sonnet-5-5"}
{"type":"session_info","name":"Parser work"}
{"type":"message","message":{…AgentMessage…}}
```

Other entry types (`thinking_level_change`, `usage`, `compaction`,
`branch_summary`, `custom`, `custom_message`, `context_edit`, `label`) are
ignored.

## AgentMessage

`message.message` is one of pi's message shapes. Fields used by omnisess:

```json
// user — content is a string or an array of {type:"text"} blocks
{"role":"user","content":[{"type":"text","text":"…"}],"timestamp":1767323048000}

// assistant — content mixes text, thinking and tool calls
{"role":"assistant",
 "content":[{"type":"thinking","thinking":"…"},
            {"type":"text","text":"…"},
            {"type":"toolCall","id":"call_1","name":"read",
             "arguments":{"path":"…"}}],
 "provider":"anthropic","model":"claude-sonnet-5-5",
 "usage":{"input":10,"output":20,"cacheRead":3,"cacheWrite":4},
 "timestamp":1767323049000}

// toolResult — answers the toolCall with the matching toolCallId
{"role":"toolResult","toolCallId":"call_1","toolName":"read",
 "content":[{"type":"text","text":"…"}],"isError":false,"timestamp":…}

// bashExecution — a user-initiated shell command
{"role":"bashExecution","command":"git status","output":"clean",
 "exitCode":0,"cancelled":false,"truncated":false,"timestamp":…}
```

- `timestamp` inside a message is unix milliseconds; the entry timestamp is
  ISO 8601 and is used when the message one is absent.
- `system`, `custom`, `branchSummary` and `compactionSummary` messages are not
  rendered.
- Tool results are attached to the originating `ToolCall` (matched by
  `toolCallId`) instead of becoming their own message.
- `bashExecution` is rendered as a `tool` message: `$ <command>` followed by
  the output.

## Session metadata

- Title: the first `session_info` name when the session was named
  (`pi --name`), else the first user message.
- Model: the last `model_change`, else the assistant message's
  `provider/model`.
- Updated time: the newest message timestamp, refined by the file mtime.

## Notes

- Session IDs are displayed as `pi:<id>`. `omnisess show` accepts a unique ID
  prefix.
- `List` scans at most the first 200 lines of each file; full transcripts are
  only read by `show` and `search`.
- Active detection is the shared fallback: a `pi` process is running **and**
  the transcript was written within the last 10 minutes. pi runs under Node
  (or as a bun-compiled binary), so the process probe matches the full command
  line (`pi-coding-agent` or `/bin/pi`).
