# GitHub Copilot CLI — Local Data Format

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

## Out of scope

VS Code workspaceStorage formats — `chatSessions/*.jsonl` and
`state.vscdb` (`interactive.sessions` key) — are intentionally NOT supported.
Agent-mode sessions are not written to those stores reliably, and supporting
`state.vscdb` would require a SQLite dependency for negligible coverage gain.
The CLI session-state directory is the authoritative local source.
