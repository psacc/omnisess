# 002 — Codex Source Implementation

**Status**: Not started
**Priority**: Next
**Estimated effort**: 2-3 hours (same JSONL pattern as Claude)

## Purpose

Implement the Codex source to parse `~/.codex/` session data. The Source interface and stub already exist at `internal/source/codex/codex.go`.

## Data Sources

### History index
- **Path**: `~/.codex/history.jsonl`
- **Format**: `{"session_id":"019c41d9-...","ts":1739091671,"text":"user prompt"}`
- **Note**: `ts` is unix seconds (not milliseconds like Claude)

### Session files
- **Path**: `~/.codex/sessions/YYYY/MM/DD/rollout-<datetime>-<session-id>.jsonl`
- **First line**: `{"timestamp":"...","type":"session_meta","payload":{"id":"...","cwd":"/path"}}`
- **Messages**: `{"type":"response_item","payload":{"type":"message","role":"developer"|"assistant","content":[{"type":"input_text","text":"..."}]}}`

### Full format spec
See `docs/references/codex-format.md`

## Concrete Steps

1. **Create `internal/source/codex/parser.go`**
   - `parseHistoryLine(line []byte) (*historyEntry, error)` — note: `ts` is unix seconds
   - `parseSessionFile(path string) ([]model.Message, string, string, error)` — returns messages, model, cwd
   - `findSessionFile(homeDir string, sessionID string) string` — glob `~/.codex/sessions/*/*/*-<sessionID>.jsonl`

2. **Update `internal/source/codex/codex.go`**
   - Replace stub methods with real implementations
   - `List()`: parse history.jsonl, deduplicate by session_id, resolve session files
   - `Get()`: find session file by ID (support prefix match), parse full content
   - `Search()`: list sessions, load content, substring match

3. **Active detection**
   - `pgrep -f codex` + session file mtime < 2min
   - Already supported by `detect.IsSessionActive("codex", path)`

## Acceptance Criteria

- `sessions list --tool=codex` shows Codex sessions with correct timestamps and previews
- `sessions search "some query" --tool=codex` finds matches in session content
- `sessions show codex:<id>` displays full conversation
- No regression in Claude or Cursor sources

## Verification

```bash
go build -o sessions . && ./sessions list --tool=codex --limit=10
./sessions search "compare" --tool=codex
```
