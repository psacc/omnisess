# Design — unify-active-definition

## Registry investigation findings (#74 prerequisite)

Empirical characterization of `~/.claude/sessions/` on a live host (2026-06-10, 25 entries, always-on agent fleet):

- **Schema** (all entries): `pid`, `sessionId`, `cwd`, `status`, `kind`, `entrypoint`, `peerProtocol`, `procStart`, `startedAt`, `updatedAt`, `version`. Background sessions (`kind: "bg"`) additionally carry `jobId` and `name`; one entry carried an optional `bridgeSessionId`.
- **`status` is an open enum**: observed `idle` (21), `busy` (3), `shell` (1) in one sample; `waiting` documented in #74. New values can appear with Claude Code releases. Consequence: never gate behavior on a closed status set — carry it as a string.
- **`kind`**: `interactive` | `bg`. **`entrypoint`**: `cli` observed (issue notes `desktop` exists).
- **GC behavior**: all 25 PIDs answered `kill(pid, 0)` — Claude Code cleans up registry files reliably. `procsnap.filterAlive` still guards against post-crash stale files (and treats EPERM as dead — PID reuse).
- **Subagent reconciliation** (open question from #74): with the registry as the source of truth, a session with a busy subagent is active simply because its parent process is alive — subagent transcript mtime no longer matters on the primary path. Whether the parent's `status` flips to `busy` while only a subagent runs is NOT yet characterized; since status does not gate activeness here, nothing depends on the answer. Left open, documented.

## Decisions

### D1 — "Active" = live attributable process; `idle` counts

#74 proposed `alive AND status ∈ {busy, waiting}` and left "is idle active?" open. Decided: **status does not gate activeness**.

- The status enum is open (`shell` was undocumented); a closed-set filter silently misclassifies future values.
- A live idle session is attached, resumable, and holds its cwd/worktree — downstream automation asking "what is still open?" needs it counted.
- Callers that want busy-only get the `status` field and can filter; that is data's job, not the definition's.
- This makes `active` membership ≡ `ps` membership for claude+codex on macOS — the strongest possible reconciliation (`active ⊆ ps` required by #74).

Decision (agent-decided): two-way door — the definition is internal and easily revised once real status semantics demand it.

### D2 — One snapshot per CLI run: `procsnap.Cached()`

`Enumerate()` shells out to `ps` and `lsof`. With both the claude and codex sources consuming it (plus `ps`/TUI), naive calls would run it 2–4× per command. `procsnap.Cached()` wraps `Enumerate` in a `sync.Once` (process-lifetime memo — correct for a one-shot CLI; the TUI keeps its explicit refresh via `Enumerate` directly). A test-only reset hook keeps the once-path coverable.

### D3 — Sources keep owning `Active`; injection via package seam

The `Source` interface is unchanged. Each source package sets `Active` as before, but claude/codex now consult `procsnap` through a package-level `var` seam (`snapshotFn = procsnap.Cached`), overridable in tests on any platform. This preserves invariant #1/#2 (self-contained sources, no cross-source imports — `procsnap` is shared infrastructure, not a source) and keeps the non-darwin fallback testable on Linux CI, where the 100% per-package gate runs.

### D4 — Fallback hierarchy, per source

| Source | Primary signal | Fallback |
|---|---|---|
| claude | registry entry + `kill(pid,0)` | tool running + transcript tree mtime < 10 min |
| codex | lsof-held rollout file | tool running + transcript mtime < 10 min |
| cursor | — (none exists) | heuristic (unchanged) |
| copilot | — (none exists) | heuristic (unchanged) |

Fallback triggers when `procsnap` returns `ErrUnsupported` (non-macOS) or any error. `ActiveThreshold` stays 10 minutes and is now documented as the *fallback* threshold (fixes the 2-min glossary drift).

### D5 — Table shows `ID` and `UPDATED`

`renderTable` was sorting by `UpdatedAt` but displaying `StartedAt` — the root of "ACTIVE, started 2 months ago" confusion. The table now shows the short session ID (cross-reference key with `ps`) and `UPDATED` (last activity). `STATUS` becomes `ACTIVE (busy)` when the registry status is known. JSON output keeps both timestamps and gains `status`.

## Alternatives considered

- **Filter `active` by status busy/waiting**: rejected (D1 — open enum, wrong layer).
- **Pass a snapshot through `ListOptions`**: rejected — changes the `Source` interface for all five sources and leaks process-correlation concerns into the contract; the seam achieves the same testability without interface churn.
- **Drop `detect` heuristics entirely**: rejected — cursor/copilot and non-macOS have no per-process signal; without the fallback they would never show active.
