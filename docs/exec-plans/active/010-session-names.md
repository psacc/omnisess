# 010 — Session Names

**Status**: Not started
**Priority**: Medium
**Estimated effort**: 2–4 hours
**Depends on**: nothing (standalone)
**Blocks**: 011-tui-enhancements

## Problem

Every session is identified by a raw qualified ID (`claude:5c3f2742`). That ID is stable but tells you nothing about what the session was about. When you have 30 sessions in a list, you scan project paths and timestamps to orient — which is slow and error-prone.

A human-readable name derived from the session's own content eliminates that scanning cost with no new dependencies and no API key required.

## Proposed Approach

Introduce a `Name()` method on `model.Session` (or a standalone resolver function in `internal/model/` or `internal/session/`) that applies the following cascade:

1. **First user message** — extract up to ~60 chars from the first user-role message body; strip leading whitespace, slash-commands, and inline code fences. If the result is non-empty, use it.
2. **Project basename** — `filepath.Base(session.ProjectPath)` if `ProjectPath` is non-empty. Short and always available.
3. **Qualified ID fallback** — `session.QualifiedID()` if everything above fails.

Rules:
- Result is always <= 60 chars, truncated at a word boundary with `"..."` suffix if needed.
- No LLM calls. No network. Pure string processing.
- `Name()` is deterministic and cheap — safe to call on every render cycle.

### Where it surfaces

- `omnisess list` — replace the raw ID column header with `Name` (or add a `Name` column alongside ID).
- `omnisess show` — show name in the session header.
- TUI sidebar (blocked on 011) — sidebar rows use `Name()` instead of qualified ID.

### Implementation sketch

```go
// internal/model/session.go (or internal/model/name.go)

func (s Session) Name() string {
    if name := nameFromFirstMessage(s.Messages); name != "" {
        return truncate(name, 60)
    }
    if s.ProjectPath != "" {
        return filepath.Base(s.ProjectPath)
    }
    return s.QualifiedID()
}
```

Parser packages (`internal/source/*/`) do not change. `Name()` is computed on demand from already-parsed data.

## Open Questions

1. Should `Name()` live on `model.Session` directly, or in a separate `internal/model/name.go` to keep the struct lean? Leaning toward same file, since it's pure data access.
2. First-message extraction: strip only leading slash-commands, or also trailing tool-call JSON? Trailing JSON can be verbose. Propose: strip anything after first blank line if length > 60.
3. Does the `list` table need a breaking column change? Could add `Name` as a new column and keep `ID` — or replace. TBD at implementation time; should not be treated as breaking since `--json` output is additive.

## Scope Estimate

- `internal/model/name.go` (or addition to `session.go`) — ~50 lines
- `internal/model/name_test.go` — table-driven: empty messages, slash-command-only first message, long first message, no project path, all fallbacks
- Minor updates to `internal/output/` table renderers to surface the name
- `make check` must pass; no new dependencies

## Out of Scope

- LLM-generated names (Wave 2, tracked in 009-visual-dashboard Wave 2 out-of-scope table)
- Persistent name caching (that is 012-lifecycle-store's job if needed)
- Name editing by user
