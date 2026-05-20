## 1. OpenSpec scaffolding

- [ ] 1.1 Create `openspec/changes/2026-05-20-perf-parallel-peek/proposal.md`
- [ ] 1.2 Create `openspec/changes/2026-05-20-perf-parallel-peek/specs/source-list-perf/spec.md`
- [ ] 1.3 Create `openspec/changes/2026-05-20-perf-parallel-peek/tasks.md`
- [ ] 1.4 `openspec validate --changes 2026-05-20-perf-parallel-peek --strict` passes

## 2. Claude source parallelization

- [ ] 2.1 Add `golang.org/x/sync/errgroup` to `go.mod` as a direct require
- [ ] 2.2 Add `peekConcurrency` named constant to `internal/source/claude/claude.go` set to `min(runtime.NumCPU(), 16)`
- [ ] 2.3 Refactor `claudeSource.List` pass 1 (history entries) to fan out the per-session work (`findSessionFile`/`findSessionFileForProject`, `sessionFileUpdatedAt`, `peekSessionMetadata`, `detect.IsSessionActive`) under `errgroup` with bounded concurrency
- [ ] 2.4 Refactor `findOrphanSessions` to fan out the per-orphan work (`sessionFileUpdatedAt`, `peekSessionMetadata`, `peekFirstUserMessage`) under `errgroup` with the same bounded concurrency
- [ ] 2.5 Cache the `detect.IsToolRunning("claude")` result once per `List` invocation, share with all per-session active probes (changes the contract of `detect.IsSessionActive` callers in this file only; the global function is unchanged)
- [ ] 2.6 Preserve sort order (`UpdatedAt` desc) after the parallel section
- [ ] 2.7 Preserve error semantics: a per-session failure logs a stderr warning and produces no result for that slot; it does not abort `List`
- [ ] 2.8 Add tests asserting deterministic output across multiple invocations under concurrency
- [ ] 2.9 Add tests asserting that a per-session peek failure does not crash `List` (synthetic JSONL with a truncated header for one session)
- [ ] 2.10 `go test -race ./internal/source/claude/...` clean
- [ ] 2.11 100% per-package coverage maintained in `internal/source/claude`

## 3. Digest parallelization

- [ ] 3.1 Refactor `cmd/digest.go` `writeDigest` to fan out `src.Get(sess.ID)` calls into an index-aligned slice; render serially in input order
- [ ] 3.2 Preserve existing error semantics: a `src.Get` failure or nil result for a session is skipped silently (matches current behaviour)
- [ ] 3.3 Add test asserting deterministic digest output under concurrency
- [ ] 3.4 `go test -race ./cmd/...` clean
- [ ] 3.5 100% per-package coverage maintained in `cmd`

## 4. Docs

- [ ] 4.1 Update CLAUDE.md invariant #4 to reflect: brute-force scan parallelized via bounded errgroup; index-routing for list/active/digest deferred pending measurement
- [ ] 4.2 No README change (no user-visible flag or output change)

## 5. Validation

- [ ] 5.1 `make check` passes
- [ ] 5.2 `make cover-check` passes
- [ ] 5.3 `make smoke` passes
- [ ] 5.4 A/B perf: baseline (main HEAD) vs branch, median of 3 runs each, for `list --limit 1`, `active`, `digest --tool claude --limit 1`. Target: branch wall-clock < 5s on ~1k sessions
