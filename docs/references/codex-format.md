# Codex — Local Data Format

## Paths

- **Global history**: `~/.codex/history.jsonl`
- **Session files**: `~/.codex/sessions/YYYY/MM/DD/rollout-<datetime>-<session-id>.jsonl`
- **Session name index**: `~/.codex/session_index.jsonl` (`{id, thread_name, updated_at}`; written on rename/save only — live sessions are usually absent)
- **SQLite metadata**: `~/.codex/sqlite/codex-dev.db`
- **Config**: `~/.codex/config.toml`

## history.jsonl Format

```json
{"session_id":"01900000-...","ts":1700000000,"text":"user prompt text"}
```

- `ts`: Unix epoch seconds
- `text`: user prompt
- `session_id`: UUID embedded in session filename

## Session JSONL Format

First line is session metadata (can run tens of KB — the payload embeds the
full base instructions). Fields used by `omnisess ps`:
```json
{"timestamp":"2026-01-01T10:00:00.000Z","type":"session_meta","payload":{"id":"01900000-0000-7000-8000-000000000000","timestamp":"2026-01-01T10:00:00.000Z","cwd":"/Users/example/prj/myapp","originator":"codex-tui","cli_version":"0.138.0"}}
```

The filename encodes the same session id plus the start time in **local**
clock time (`rollout-YYYY-MM-DDTHH-MM-SS-<uuid>.jsonl`); `payload.timestamp`
is UTC.

Subsequent lines are response items:
```json
{"timestamp":"2026-01-01T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"user prompt"}]}}
```

## CLI Support

```bash
codex resume              # Interactive session picker
codex resume --last       # Resume most recent
codex resume <session-id> # Resume by ID
```

## Live Process Detection (`omnisess ps`)

Codex keeps no PID registry (unlike Claude's `~/.claude/sessions/<PID>.json`).
An active thread owns an open `~/.codex/thread-writer-locks/<session-id>.lock`.
Some process shapes also hold the matching rollout JSONL open.

`internal/procsnap` uses this as follows (macOS only):

1. Candidate PIDs = processes in the `ps` snapshot whose executable basename
   is `codex` — matches the CLI TUI (`comm` = `codex`) and Codex.app's
   app-server (`comm` = full bundle path ending in `/codex`).
2. One `lsof -n -P -a -p <pid,...> -F pfn` call maps each PID to its cwd, open
   thread-writer locks, and open rollout JSONLs.
3. Each distinct lock or rollout identifies one live session. A lock-only
   session locates its rollout by exact ID for metadata. The `session_meta`
   first line supplies cwd/version/originator/start when available.

Notes:
- An idle Codex.app app-server holds no thread lock or rollout → correctly not listed.
- An app-server with N live threads holds N locks → N sessions, one PID.
- lsof failure degrades gracefully: stderr warning, claude-only output.

## Status

Transcript source: implemented (`internal/source/codex`).
Live detection in `ps`: implemented (`internal/procsnap`, macOS only).
Session names (`session_index.jsonl` lookup): not consumed — deferred.
