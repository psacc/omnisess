# Tasks: ps-codex-processes

## 1. Setup

- [x] 1.1 Create branch `feat/ps-codex` off main

## 2. procsnap codex enumeration

- [x] 2.1 Add `Tool` field to `procsnap.Session`; set `"claude"` in existing darwin enumeration; update package doc comment
- [x] 2.2 Add `procsnap_darwin_codex.go`: candidate PID selection from procs map (comm basename `codex`), injectable `codexLsofFn` + `codexSessionsDirFn`, lsof `-F pfn` parser → pid→{cwd, rollout paths}
- [x] 2.3 Add rollout `session_meta` first-line parser with filename fallback (id, started-at) and lsof-cwd fallback
- [x] 2.4 Wire codex sessions into `Enumerate()` with ancestors from the shared procs map; graceful degradation (stderr warn, claude-only) on lsof/dir errors
- [x] 2.5 Table-driven tests + testdata fixtures for all new code paths; maintain 100% per-package coverage (`make cover-check`)

## 3. Command and TUI surface

- [x] 3.1 `cmd/ps.go`: render `s.Tool` in leaf label; update Short/Long help text; update/extend `ps` tests
- [x] 3.2 `internal/tui/model.go`: filter `ApplySnapshot` map to claude-tool entries; test covering codex entries being ignored

## 4. Docs

- [x] 4.1 Update `docs/references/codex-format.md`: live-process detection section; refresh stale DEFERRED status note
- [x] 4.2 Update `CLAUDE.md` (procsnap description), `ARCHITECTURE.md` (codemap), `TESTING.md` §4.1 ps row note

## 5. Verify, QA, PR

- [x] 5.1 `make check` + `make cover-check` clean
- [x] 5.2 Smoke + e2e against live local codex instances: `./omnisess ps` and `ps --json` show running codex TUIs with correct cwd/age; wallclock < 5s
- [x] 5.3 QA pass per TESTING.md (ps golden + risk overlay), report PII-clean per §3
- [ ] 5.4 Reviewer subagent review of branch diff; address findings
- [ ] 5.5 Push branch, open **draft PR** with template fully filled, no PII (one-way door: no self-merge)
