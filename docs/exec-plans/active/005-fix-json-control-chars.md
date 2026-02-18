# 005 — Fix JSON Control Characters in `--json` Output

**Status**: In progress
**Priority**: Bugfix (affects downstream consumers)
**Estimated effort**: 1-2 hours

## Problem

`sessions show <id> --json` can emit JSON containing unescaped control characters
(U+0000 through U+001F) in message content, causing Python's `json.load()` to
fail with "Invalid control character" errors.

## Investigation

Go's `encoding/json` encoder correctly escapes all control characters (U+0000-U+001F)
in `string` fields. However, message content can contain control characters from
multiple sources:

1. **Claude JSONL files**: Content parsed via `json.Unmarshal` which rejects literal
   control chars. Safe in theory, but edge cases exist with malformed source data
   that Go may handle leniently across versions.
2. **Cursor transcript files**: Plain text files read line-by-line. Can contain ANSI
   escape sequences (ESC = 0x1B), null bytes, form feeds, and other control characters
   from terminal output captured in transcripts.
3. **ToolCall.Input truncation** (`extractToolCalls` in claude/parser.go): Byte-level
   truncation at position 200 can split multi-byte UTF-8 characters, producing invalid
   UTF-8 sequences.

The defensive fix is to sanitize control characters at the output layer before JSON
serialization, ensuring all sources benefit regardless of how content was ingested.

## Solution

Add a `sanitizeForJSON` function in `internal/output/render.go` that strips non-printable
control characters (U+0000-U+001F) except `\n`, `\r`, `\t` (which Go's encoder
handles correctly and which carry semantic meaning). Apply this sanitization to a
deep copy of the session before JSON encoding.

**Why strip instead of escape?**
- Control chars like NUL, ESC, BEL, BS have no semantic value in session content.
- ANSI escape sequences (ESC + `[...m`) are terminal-only formatting — noise in JSON.
- Stripping is safer than escaping: no risk of producing malformed escape sequences.

**Why at the output layer?**
- All sources benefit without modifying each parser.
- Raw content is preserved for table display where control chars are harmless.
- Single point of control for JSON output correctness.

## Concrete Steps

1. Add `sanitizeStringForJSON(s string) string` to `internal/output/render.go`
   - Strip bytes 0x00-0x08, 0x0B, 0x0C, 0x0E-0x1F (keep \t=0x09, \n=0x0A, \r=0x0D)
2. Add `sanitizeSessionForJSON(s *model.Session) model.Session` that returns a
   sanitized shallow copy — sanitize `Title`, `Summary`, `Preview`, and all
   `Messages[].Content`, `Messages[].ToolCalls[].Input`, `Messages[].ToolCalls[].Output`
3. Update `RenderSession` JSON path to sanitize before encoding
4. Update `RenderSessions` / `renderJSON` and `RenderSearchResults` JSON paths similarly
5. Add tests: table-driven test for `sanitizeStringForJSON` with control chars,
   and a round-trip test that encodes a session with control chars and verifies
   Python-compatible JSON output via `json.Valid()`

## Testing

```bash
make check
go build -o sessions .
./sessions show claude:bb80f802 --json 2>/dev/null | python3 -c "import json,sys; json.load(sys.stdin); print('OK')"
```

## Acceptance Criteria

- `sessions show <id> --json` output is always valid JSON parseable by Python's `json.load()`
- Control characters in message content are stripped, not corrupted
- Table output is unchanged (no sanitization applied to non-JSON paths)
- All existing tests pass
