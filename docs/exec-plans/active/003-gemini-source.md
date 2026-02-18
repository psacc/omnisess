# 003 — Gemini Source Implementation

**Status**: Not started
**Priority**: Low (blocked by encrypted protobuf)
**Estimated effort**: 1-2 hours for CLI-based listing, content search not feasible

## Purpose

Implement best-effort Gemini source. Conversation files are encrypted protobuf — only the `gemini --list-sessions` CLI output is viable for listing.

## Constraints

- `~/.gemini/antigravity/conversations/*.pb` — encrypted, no published schema, not parseable
- `~/.gemini/history/<project>/` — only `.project_root` markers, no conversation data
- **Only viable approach**: shell out to `gemini --list-sessions` and parse text output

## Concrete Steps

1. **`internal/source/gemini/gemini.go`** — `List()` implementation:
   - Run `gemini --list-sessions` via `os/exec`
   - Parse the text output (index, title, date)
   - Return sessions with metadata only (no message content)

2. **`Get()` and `Search()`** — return error "Gemini content search not supported (encrypted storage)"

3. **Active detection**: `pgrep -f gemini`

## Acceptance Criteria

- `sessions list --tool=gemini` shows sessions from `gemini --list-sessions`
- `sessions show gemini:<id>` returns a clear error about encrypted storage
- Graceful fallback if `gemini` CLI is not installed

## Open Questions

- Does `gemini --list-sessions` require active auth? Need to test.
- Does the output format change across gemini CLI versions?
- Monitor for Google adding `--export` or `--show-session` features
