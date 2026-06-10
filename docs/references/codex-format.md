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
The reliable correlation primitive: **every live codex session holds its
rollout JSONL open for writing for the session's lifetime.**

`internal/procsnap` uses this as follows (macOS only):

1. Candidate PIDs = processes in the `ps` snapshot whose executable basename
   is `codex` — matches the CLI TUI (`comm` = `codex`) and Codex.app's
   app-server (`comm` = full bundle path ending in `/codex`).
2. One `lsof -n -P -a -p <pid,...> -F pfn` call maps each PID to its cwd and any
   open `.jsonl` files under `~/.codex/sessions/`.
3. Each held rollout = one live session. The `session_meta` first line
   supplies id/cwd/version/originator/start; the filename is the fallback
   for id/start, lsof cwd for cwd. A held `.jsonl` is skipped (with a
   stderr warning) only when both the meta parse and the filename parse
   fail — either alone is enough to identify a session.

Notes:
- An idle Codex.app app-server holds no rollout open → correctly not listed.
- An app-server with N live threads holds N rollouts → N sessions, one PID.
- lsof failure degrades gracefully: stderr warning, claude-only output.

## Status

Transcript source: implemented (`internal/source/codex`).
Live detection in `ps`: implemented (`internal/procsnap`, macOS only).
Session names (`session_index.jsonl` lookup): not consumed — deferred.
