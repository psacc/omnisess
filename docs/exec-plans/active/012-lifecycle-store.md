# 012 — Lifecycle Store

**Status**: Not started
**Priority**: Low (deferred until TUI baseline is validated)
**Estimated effort**: 4–6 hours
**Depends on**: nothing (fully standalone)
**Blocks**: nothing (optional upgrade to 011-tui-enhancements)

## Problem

Sessions have no persistent state outside their source files. There is no way to:
- Mark a session as worth revisiting (pin)
- Know when a session started and ended
- Compute duration without reading the full message list
- Filter by recency without re-parsing every file
- Archive stale sessions automatically

All of that state lives in the engineer's head today. As session counts grow, that becomes expensive.

## Proposed Approach

A SQLite metadata overlay at `~/.config/omnisess/sessions.db` (pure Go via `modernc.org/sqlite`, already in `go.mod`). Source files remain READ-ONLY at all times — this store is a sidecar, never a replacement.

### Schema

```sql
CREATE TABLE IF NOT EXISTS schema_version (
    version_id INTEGER PRIMARY KEY,
    version    TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_version (version_id, version) VALUES (1, '1.0.0');

CREATE TABLE IF NOT EXISTS sessions_meta (
    session_qualified_id TEXT PRIMARY KEY,
    state                TEXT NOT NULL DEFAULT 'active'
                         CHECK (state IN ('active', 'pinned', 'archived')),
    pinned_at            DATETIME,
    last_viewed_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ttl_hours            INTEGER NOT NULL DEFAULT 12
);

CREATE TABLE IF NOT EXISTS lifecycle_events (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    session_qualified_id TEXT NOT NULL,
    event_type           TEXT NOT NULL CHECK (event_type IN ('start', 'end', 'pin', 'unpin', 'view')),
    occurred_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Note: `lifecycle_events` is new relative to 009's schema. It enables duration computation (`end.occurred_at - start.occurred_at`) and audit/stats without scanning source files.

### Package: `internal/lifecycle/`

Key surface:

```go
type State string
const (
    StateActive   State = "active"
    StatePinned   State = "pinned"
    StateArchived State = "archived"
)

type Store struct { /* db handle */ }

func Open(path string) (*Store, error)
func (s *Store) Close() error
func (s *Store) RunTTLSweep(sessions []model.Session) error
func (s *Store) GetState(qualifiedID string) (State, error)
func (s *Store) SetState(qualifiedID string, state State) error
func (s *Store) TogglePin(qualifiedID string) error
func (s *Store) TouchViewed(qualifiedID string) error
func (s *Store) UpsertIfMissing(qualifiedID string) error
func (s *Store) RecordEvent(qualifiedID string, event EventType) error
func (s *Store) Duration(qualifiedID string) (time.Duration, bool, error)
```

Invariants:
- Never import `internal/source/*`.
- Import only `database/sql`, `modernc.org/sqlite`, `internal/model`.
- If `Open()` fails, callers degrade gracefully — all sessions treated as `active`, no panic.
- DB path: `~/.config/omnisess/sessions.db` (resolved at runtime via `os.UserHomeDir()`).

### Integration with 011-tui-enhancements

011's `tui.New()` accepts `*lifecycle.Store` (nilable). When 012 lands:
- `cmd/tui.go` calls `lifecycle.Open()`, passes store to `tui.New()`.
- TUI buckets sessions: `pinned` → section 0, `active` → section 1, `archived` → section 2.
- `p` key toggles pin; `j/k` navigation fires `TouchViewed` (goroutine, fire-and-forget).

### Integration with `omnisess stats` (future)

`lifecycle_events` makes `stats` trivial: aggregate by `event_type`, join on `session_qualified_id`. No re-parsing required.

## Open Questions

1. DB path: `~/.config/omnisess/sessions.db` or `~/.config/sessions/sessions.db`? The 009 plan used `sessions/sessions.db`. Leaning `omnisess/sessions.db` for namespacing — cleaner if other tools use `~/.config/`.
2. TTL default: 12 hours (from 009 plan). Is this right? 12h means any session you haven't opened today gets auto-archived. Reasonable default but should be configurable via flag or config file eventually.
3. `lifecycle_events` for duration: requires `start` events to be recorded. Who records `start`? Options: (a) record on first `UpsertIfMissing` call, (b) require explicit `RecordEvent(id, EventStart)` from callers. Option (b) is more accurate but requires callers to track it. Defer decision to implementation.
4. Should `RunTTLSweep` archive based on `last_viewed_at` or `sessions_meta.ttl_hours` vs source file mtime? Source file mtime is the ground truth for activity; `last_viewed_at` tracks omnisess access. Propose: archive when source file mtime is older than `ttl_hours` (i.e., tool hasn't written to it in N hours).

## Scope Estimate

- `internal/lifecycle/store.go` — ~150 lines
- `internal/lifecycle/store_test.go` — table-driven: first-run schema creation, TTL sweep boundary (12h, 11h59m, 12h01m), UpsertIfMissing idempotency, TogglePin state machine, TouchViewed timestamp, corrupted DB open failure, Duration happy path
- `cmd/tui.go` — ~20 lines (Open + defer Close + pass to tui.New)
- No new `go.mod` dependencies

## Dependency Graph

```
012-lifecycle-store  (standalone)
       │
       └──►  011-tui-enhancements  (optional wire-in for lifecycle bucketing)
       │
       └──►  future: omnisess stats subcommand
```

## Out of Scope

- Configurable TTL per session (validate 12h default first)
- Archive restore UI action
- LLM summary cache (separate `summaries` table from 009 is intentionally omitted here — add when Wave 2 LLM work begins)
- Cross-machine sync
