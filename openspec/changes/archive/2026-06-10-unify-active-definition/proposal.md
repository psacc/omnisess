## Why

`omnisess active` and `omnisess ps` answer "what is running?" with two unrelated mechanisms that disagree on both count and membership (#74). `active` flags a session when *any* `claude` process exists globally (`pgrep -x claude`) AND its transcript tree was modified within `ActiveThreshold` (10 minutes) — so a freshly-written transcript passes even when no process is attributable to it, and on a host with an always-on agent fleet the global pgrep is permanently true. `ps` instead correlates exact PIDs: the `~/.claude/sessions/<PID>.json` registry for Claude Code, lsof-held rollout files for Codex. Observed in practice: `active` reported 9 sessions, `ps` reported ~19 live processes, and the two sets were not reconcilable by eye because `active` shows no session ID.

The reliable signal already exists and `ps` already consumes it. Empirical check of the live registry (2026-06-10, 25 entries): every entry carries `pid`, `sessionId`, `cwd`, `status`, `kind`, `entrypoint`, `startedAt`, `updatedAt`, `version`; all 25 PIDs were alive (the registry is well garbage-collected); observed `status` values were `busy`, `idle`, and `shell` — an **open enum** (issue #74 had also observed `waiting`).

There must be one definition of "active", not two.

## What Changes

### One definition, one code path

A session is **active** iff a live OS process is attributable to that exact session ID:

- **Claude Code**: a `~/.claude/sessions/<PID>.json` registry entry with that `sessionId` whose PID answers `kill(pid, 0)`.
- **Codex**: a live codex process holding the session's rollout JSONL open (the v0.9.0 lsof correlation).

This is exactly the `procsnap` correlation `ps` uses, so `active ⊆ ps` holds by construction. Registry `status` (`busy`/`idle`/`shell`/…) does **not** gate activeness — a live idle session is active; status is surfaced as data instead (open enum, filtering on it would be fragile).

- New `procsnap.Cached()` memoizes one `Enumerate()` per CLI run; `active`, `list`, `ps`, and the TUI all share that single snapshot.
- `internal/source/claude` and `internal/source/codex` set `Session.Active` from the snapshot instead of `detect.IsSessionActive`.
- `procsnap.Session` gains `Status` (from the registry); `model.Session` gains `Status` so it reaches output.

### Documented fallback where no per-process signal exists

Cursor and Copilot have no registry or held-file signal; non-macOS platforms have no `procsnap`. These keep the existing heuristic (tool process running AND transcript tree mtime < `ActiveThreshold`), now documented as the *fallback* definition. Claude/Codex also fall back when `procsnap` errors or is unsupported.

### Output cleanups (from #74)

- The sessions table gains an `ID` column (short session ID) so `active` and `ps` can be cross-referenced.
- The `STARTED` column becomes `UPDATED` (last activity) — the table was already *sorted* by `UpdatedAt` while *displaying* `StartedAt`, which is how two-month-old sessions appeared "ACTIVE, started 2 months ago".
- `STATUS` shows the registry status when known: `ACTIVE (busy)`.

### Doc drift fixes

- `CLAUDE.md` glossary said "modified < 2 min ago"; code says 10 minutes. The glossary now states the registry-based definition with the mtime fallback.
- `ARCHITECTURE.md`, `TESTING.md` §4.1, and `ROADMAP.md` updated accordingly.

## Impact

- Affected packages: `internal/procsnap`, `internal/source/claude`, `internal/source/codex`, `internal/model`, `internal/output`, docs.
- `model.Session` gains a field (additive; JSON `omitempty`) — public-interface change, escalated per `docs/process/git-workflow.md` §6.
- Behavior change: `active` counts will change (more truthful, reconcilable with `ps`); table columns change (`ID` added, `STARTED`→`UPDATED`).
- Closes #74.
