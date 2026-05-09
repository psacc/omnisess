# GitHub Copilot — Local Data Format

## Paths

- **Per-session directory**: `~/.copilot/session-state/<session-uuid>/`
  - `events.jsonl` — append-only conversation event log
  - `vscode.metadata.json` — workspace metadata (cwd in `workspaceFolder`)

The directory base name is the session ID. There is no global history index;
each session is fully described by its own directory.

## events.jsonl Format

Each line is a JSON object:

```json
{"type":"user.message","timestamp":"2026-04-14T08:10:12.453Z","data":{"content":"…"}}
{"type":"assistant.message","timestamp":"2026-04-14T08:10:14.021Z","data":{"content":"…"}}
{"type":"tool.execution_start","timestamp":"…","data":{…}}
{"type":"tool.execution_complete","timestamp":"…","data":{…}}
```

- `type`: `user.message`, `assistant.message`, `tool.execution_start`,
  `tool.execution_complete` (others are skipped).
- `timestamp`: ISO 8601 string. Not all events carry a parseable timestamp.
- `data.content`: textual content for user / assistant messages.

The CLI streams assistant turns: a single response can be split across
multiple consecutive `assistant.message` lines. The parser merges them into
one message.

## vscode.metadata.json Format

```json
{"workspaceFolder":"/Users/foo/prj/bar", "openedFiles":[…]}
```

Only `workspaceFolder` is consumed.

## VS Code workspaceStorage (darwin)

Two additional locations are also read on darwin:

```
~/Library/Application Support/Code/User/workspaceStorage/<hash>/
  workspace.json          # cwd: {"folder":"file:///..."} or {"workspace":...}
  chatSessions/*.jsonl    # VS Code "classic" chat — one session per file
  state.vscdb             # legacy SQLite store (ItemTable.interactive.sessions)
```

### chatSessions/*.jsonl

Each file holds one or more `{"v": {...}}` snapshots of the same chat
session. The latest line with a non-empty `requests` array is canonical.

```json
{"v":{"sessionId":"…","creationDate":1700000000000,"lastMessageDate":1700000005000,"requests":[{"message":{"text":"…"},"response":[{"kind":"markdownContent","content":{"value":"…"}}]}]}}
```

- Dates are millisecond epochs.
- `response` parts can be `markdownContent`, `markdownVuln`, or a bare
  `{"value":"…"}`. All three are concatenated when reconstructing
  assistant messages.

### state.vscdb

Pure-Go (`modernc.org/sqlite`) read of the `ItemTable` row keyed
`interactive.sessions`. Its `value` is a JSON array of session objects
sharing the same shape as `chatSessions` lines (no `v` wrapper).

### Notes

- Agent-mode sessions aren't always present in these VS Code stores; the
  CLI session-state remains the most reliable source.
- The session ID for a `chatSessions` file is its `sessionId` field if set,
  otherwise the `.jsonl` stem.
- VS Code subsources currently ship for darwin only; Linux / Windows paths
  are a follow-up.
