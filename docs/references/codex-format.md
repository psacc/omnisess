# Codex — Local Data Format (Reference for Later)

## Paths

- **Global history**: `~/.codex/history.jsonl`
- **Session files**: `~/.codex/sessions/YYYY/MM/DD/rollout-<datetime>-<session-id>.jsonl`
- **SQLite metadata**: `~/.codex/sqlite/codex-dev.db`
- **Config**: `~/.codex/config.toml`

## history.jsonl Format

```json
{"session_id":"019c41d9-...","ts":1739091671,"text":"compare AGENTS.md with..."}
```

- `ts`: Unix epoch seconds
- `text`: user prompt
- `session_id`: UUID embedded in session filename

## Session JSONL Format

First line is session metadata:
```json
{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"id":"019c41d9-...","cwd":"/Users/paolo/prj/gd"}}
```

Subsequent lines are response items:
```json
{"timestamp":"2026-02-09T10:01:11.966Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"user prompt"}]}}
```

## CLI Support

```bash
codex resume              # Interactive session picker
codex resume --last       # Resume most recent
codex resume <session-id> # Resume by ID
```

## Status: DEFERRED

Implementation deferred. Stub source returns empty results.
Same JSONL pattern as Claude — straightforward to add later.
