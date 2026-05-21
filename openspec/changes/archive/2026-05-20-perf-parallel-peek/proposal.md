## Why

Issue #54 surfaced that `omnisess list`, `active`, and `digest` take tens of seconds on a multi-thousand-session corpus. Per-source measurement on the affected corpus shows the cost is concentrated entirely in the Claude source:

- `omnisess list --tool claude --limit 1`: ~50s
- `omnisess list --tool cursor --limit 1`: <0.1s
- `omnisess list --tool codex --limit 1`: ~3s
- `omnisess list --tool copilot --limit 1`: <0.1s

The Claude `List` enumerates every session in `history.jsonl` plus orphan JSONL files on disk, then for each one sequentially: opens the JSONL to read git branch and model (`peekSessionMetadata`), reads the first user message (`peekFirstUserMessage`, orphans only), stats the file for `UpdatedAt` refinement, and spawns `pgrep` to detect active sessions. That's N × open() + N × pgrep on a single goroutine.

PR2 originally proposed routing through the SQLite transcript index. After investigation the index schema (OTel GenAI–aligned) does not store the omnisess-specific session metadata (`Project`, `Branch`, `Preview`, `Title`) that `list`/`active`/`digest` render. Extending the schema is a one-way door we want to defer until we have measurement-backed evidence that parallelization alone is insufficient.

This proposal scopes the cheapest reversible experiment: parallelize the per-session I/O inside Claude's `List` and the per-session full-parse inside `digest`. No schema change. If wall-clock drops below the 5s target on ~1k sessions, we ship. If not, we revisit the index-routing option with data in hand.

## What Changes

### `internal/source/claude/`

- Parallelize the per-session work inside `claudeSource.List` using `golang.org/x/sync/errgroup` with bounded concurrency (`min(runtime.NumCPU(), 16)`). The work parallelized per session:
  - `findSessionFile` / `findSessionFileForProject`
  - `sessionFileUpdatedAt`
  - `peekSessionMetadata`
  - `detect.IsSessionActive` (the per-session call site)
- Parallelize the per-orphan work inside `findOrphanSessions` (the `peekSessionMetadata` + `peekFirstUserMessage` + `sessionFileUpdatedAt` triple).
- Cache the `detect.IsToolRunning("claude")` result once per `List` invocation (N pgrep spawns → 1).
- Preserve deterministic output: collect into index-aligned slots, then filter and sort by `UpdatedAt` descending — identical to today.
- Preserve existing error semantics: a per-session failure logs a warning to stderr and skips the entry; it never aborts `List`.

### `cmd/digest.go`

- Parallelize the per-session `src.Get(sess.ID)` calls (currently sequential inside `writeDigest`) using the same errgroup pattern. The render-to-writer step remains sequential to preserve deterministic markdown output.

### Constants

- New named constant `peekConcurrency` (Claude source) defining the bounded fan-out.

## Capabilities

### New Capabilities

- `source-list-perf`: Performance contract for the session-listing pipeline — covers the Claude source's per-session peek loop and the `digest` command's per-session full-parse loop.

### Modified Capabilities

None.

## Impact

- `internal/source/claude/claude.go`: `List` body refactored to fan out per-session work. `findOrphanSessions` body refactored to fan out per-orphan work. New `runningCache` helper to memoize the `IsToolRunning` result for the duration of one `List` call.
- `cmd/digest.go`: `writeDigest` refactored to fan out `src.Get` calls into a slice, then render serially.
- `go.mod`: promote `golang.org/x/sync` from indirect to direct. The dependency is already pulled in via the build graph; this only adds an explicit `require`.
- Coverage: new tests for the concurrent paths must keep per-package coverage at 100%. Race-detector clean (`go test -race ./...`).

No on-disk format change. No CLI flag change. No output-format change. Other sources (`cursor`, `codex`, `copilot`, `gemini`) untouched — they are not the bottleneck.

## Out of scope (defer)

- Routing `list`/`active`/`digest` through the transcript index (requires schema extension; defer until parallelization is shown insufficient)
- `omnisess search` parallelization
- Cursor / Codex / Copilot `List` parallelization (codex is ~3s, the rest are <0.1s — not the bottleneck)
- Replacing `pgrep` with a Go-native process scan
- Background indexing
