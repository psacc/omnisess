# Tasks — unify-active-definition

## 1. procsnap: status + memoized snapshot

- [x] 1.1 Add `Status string` to `procsnap.Session`; parse `status` from the registry JSON in `procsnap_darwin_sessions.go` (open enum, pass through verbatim; codex sessions leave it empty)
- [x] 1.2 Add `Cached() (Snapshot, error)` in `procsnap.go`: `sync.Once`-memoized `Enumerate()` (both the snapshot and the error are memoized); add an unexported test reset helper
- [x] 1.3 Update procsnap fixtures/tests for the new field and for `Cached` (once-semantics + reset), keeping 100% per-package coverage on both darwin and linux

## 2. claude source: registry-routed activeness

- [x] 2.1 Add seam `var snapshotFn = procsnap.Cached` in `internal/source/claude`; helper that returns (snapshot, ok) — ok=false on any error
- [x] 2.2 Replace the three `detect`-based active sites (`peekHistoryEntry`, `peekOrphanFile`, `Get`) with: snapshot lookup by session ID when ok; existing heuristic otherwise
- [x] 2.3 Populate `model.Session.Status` from the snapshot for active sessions
- [x] 2.4 Tests: fake snapshots exercising active-via-registry, inactive-despite-recent-mtime, fallback-on-error, status passthrough

## 3. codex source: lsof-routed activeness

- [x] 3.1 Same seam + replacement at the two `detect.IsSessionActive("codex", …)` sites
- [x] 3.2 Tests mirroring 2.4

## 4. model + output

- [x] 4.1 Add `Status string \`json:"status,omitempty"\`` to `model.Session`
- [x] 4.2 `renderTable`: add `ID` column (short session ID); rename `STARTED` → `UPDATED` rendering `UpdatedAt`; status cell `ACTIVE (busy)` / `ACTIVE` / `-`
- [x] 4.3 `renderSessionDetail`: show status when present; show Updated timestamp
- [x] 4.4 Update output/cmd/tui tests for the new columns and field

## 5. Docs

- [x] 5.1 `CLAUDE.md` glossary: registry-based definition + fallback (fixes the "< 2 min" drift); note `Status` on Session
- [x] 5.2 `ARCHITECTURE.md`: procsnap as the shared activeness authority; detect demoted to fallback
- [x] 5.3 `TESTING.md` §4.1: update `active` expectations (registry-correlated; reconcilable with `ps`)
- [x] 5.4 `ROADMAP.md`: tick the #74 items in Priority 1
- [x] 5.5 `skills/omnisess-active/SKILL.md`: describe the new definition and columns

## 6. Verification

- [x] 6.1 `make check`, `make cover-check`, `make smoke` green
- [x] 6.2 Manual reconciliation on real data: every claude/codex session in `omnisess active` appears in `omnisess ps`; counts explained
- [x] 6.3 `omnisess active --format json` carries `status`
