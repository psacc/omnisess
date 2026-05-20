# source-list-perf

Performance contract for the session-listing pipeline. Covers the Claude source's per-session peek loop and the `digest` command's per-session full-parse loop.

## ADDED Requirements

### Requirement: Claude List parallelizes per-session peek work

The Claude source's `List` SHALL execute the per-session peek work (`findSessionFile`, `sessionFileUpdatedAt`, `peekSessionMetadata`, active-status detection) concurrently across sessions, bounded by `min(runtime.NumCPU(), 16)`. The result slice SHALL be sorted by `UpdatedAt` descending after the parallel section so that output ordering is deterministic and identical to the sequential implementation.

#### Scenario: Output is identical to sequential implementation

- **GIVEN** a Claude `~/.claude` layout with N session JSONL files referenced by `history.jsonl`
- **WHEN** `claudeSource.List(opts)` is called twice in succession with the same `opts`
- **THEN** both returned slices contain the same session entries
- **AND** both slices are ordered by `UpdatedAt` descending
- **AND** the order is stable across invocations

#### Scenario: One peek failure does not abort List

- **GIVEN** a Claude layout where one session JSONL is truncated such that `peekSessionMetadata` cannot extract a branch
- **WHEN** `List` runs
- **THEN** the offending session is still listed (without branch/model)
- **AND** the other sessions are listed with their branch/model
- **AND** no error is returned to the caller

#### Scenario: pgrep is invoked once per List call

- **GIVEN** N sessions in `history.jsonl`
- **WHEN** `List` runs
- **THEN** the `IsToolRunning("claude")` check is performed at most once for the duration of the call
- **AND** the cached result is reused for every per-session active probe

### Requirement: Orphan-scan peek work is parallelized

The Claude source's orphan-session discovery (`findOrphanSessions`) SHALL execute the per-orphan peek work (`sessionFileUpdatedAt`, `peekSessionMetadata`, `peekFirstUserMessage`) concurrently across orphans, bounded by the same `min(runtime.NumCPU(), 16)` cap as the main `List` fan-out.

#### Scenario: Orphan output is deterministic

- **GIVEN** a Claude layout with M orphan JSONL files (not present in `history.jsonl`)
- **WHEN** `findOrphanSessions` runs
- **THEN** the returned orphan list contains exactly M entries with their per-file metadata populated
- **AND** every entry's `SessionID` matches its filename (basename without `.jsonl`)

### Requirement: Digest parallelizes per-session full-parse

The `omnisess digest` command SHALL execute the per-session full-parse step (`source.Source.Get`) concurrently across the matched sessions, bounded by `min(runtime.NumCPU(), 16)`. The markdown render step SHALL remain sequential to preserve deterministic byte-identical output.

#### Scenario: Digest output is deterministic across runs

- **GIVEN** N matched sessions for a given `digest` invocation
- **WHEN** `digest` runs twice in succession with identical flags against an unchanged on-disk layout
- **THEN** both runs produce byte-identical stdout

#### Scenario: A Get failure for one session does not abort digest

- **GIVEN** N matched sessions where one source's `Get` returns `nil, error` for one session
- **WHEN** `digest` runs
- **THEN** the failing session is silently skipped (matching current behaviour)
- **AND** the other sessions are rendered in order
- **AND** the command exits zero

### Requirement: Race-detector clean

The new concurrent paths in `internal/source/claude` and `cmd/digest.go` SHALL be free of data races as detected by `go test -race ./...`.

#### Scenario: race-detector run passes

- **WHEN** `go test -race ./internal/source/claude/... ./cmd/...` runs
- **THEN** the command exits zero
- **AND** no `DATA RACE` line appears in stderr
