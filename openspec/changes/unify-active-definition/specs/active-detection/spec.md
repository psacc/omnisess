# active-detection

One definition of "active" across all active-aware paths (`active`, `list --active`, `digest`, TUI): a session is active iff a live OS process is attributable to that exact session ID. Where no per-process signal exists, a documented mtime-based fallback applies.

## ADDED Requirements

### Requirement: Claude activeness comes from the session registry

A Claude Code session SHALL be reported active iff a `~/.claude/sessions/<PID>.json` registry entry exists whose `sessionId` matches the session AND whose `pid` is a live process (`kill(pid, 0)` succeeds; EPERM counts as dead).

#### Scenario: live registry entry

- **GIVEN** a registry entry with `sessionId: S` and a live `pid`
- **WHEN** `list` or `active` evaluates session `S`
- **THEN** session `S` is active, regardless of transcript mtime and regardless of registry `status`

#### Scenario: no registry entry for a recently-written transcript

- **GIVEN** session `S` has no live registry entry
- **AND** its transcript was modified 1 minute ago
- **AND** some unrelated `claude` process is running
- **WHEN** the snapshot is available (macOS, no enumeration error)
- **THEN** session `S` is NOT active

### Requirement: Codex activeness comes from the lsof rollout correlation

A Codex session SHALL be reported active iff a live codex process holds the session's rollout JSONL open (the same correlation `ps` uses).

#### Scenario: held rollout file

- **GIVEN** a codex process holding `rollout-...-<id>.jsonl` open for session `<id>`
- **WHEN** `active` evaluates that session
- **THEN** it is active

### Requirement: One snapshot per CLI invocation

All active-aware paths in one CLI invocation SHALL consume a single memoized process snapshot (`procsnap.Cached()`), so `active`, `list`, and `ps` cannot disagree within a run.

#### Scenario: claude and codex sources share the snapshot

- **GIVEN** an `active` run over the claude and codex sources
- **WHEN** both sources evaluate activeness
- **THEN** process enumeration (`ps`/`lsof`/registry scan) ran at most once

### Requirement: Registry status is surfaced, never used as a gate

The registry `status` field SHALL be treated as an open string enum (observed: `busy`, `idle`, `waiting`, `shell`), carried on `procsnap.Session.Status` and `model.Session.Status`, and shown in output. It SHALL NOT gate activeness.

#### Scenario: idle session is active

- **GIVEN** a live registry entry with `status: "idle"`
- **WHEN** activeness is evaluated
- **THEN** the session is active and its status renders as `ACTIVE (idle)`

#### Scenario: unknown future status value

- **GIVEN** a live registry entry with `status: "some-new-value"`
- **WHEN** activeness is evaluated
- **THEN** the session is active and the status string is passed through unmodified

### Requirement: Fallback heuristic where no per-process signal exists

When the snapshot is unavailable (`procsnap.ErrUnsupported` on non-macOS, or an enumeration error) — and always for cursor and copilot, which have no per-process signal — activeness SHALL fall back to: tool process running AND session transcript tree modified within `detect.ActiveThreshold` (10 minutes).

#### Scenario: non-macOS platform

- **GIVEN** a platform where `procsnap` returns `ErrUnsupported`
- **AND** a claude process is running
- **AND** the session transcript was modified 5 minutes ago
- **WHEN** activeness is evaluated
- **THEN** the session is active via the fallback

#### Scenario: cursor session

- **GIVEN** a cursor session with a transcript modified 5 minutes ago and Cursor running
- **WHEN** activeness is evaluated
- **THEN** the heuristic result is used (unchanged behavior)

### Requirement: active is a subset of ps

On macOS, every claude or codex session reported by `active` SHALL correspond to a live process visible to `ps` in the same run.

#### Scenario: reconcilable by construction

- **GIVEN** `active` and `ps` executed against the same snapshot
- **WHEN** their outputs are compared
- **THEN** each active claude/codex session's ID appears in the `ps` tree

### Requirement: Session tables show ID and last activity

The sessions table (used by `list` and `active`) SHALL include the short session ID and an `UPDATED` (last activity) column in place of `STARTED`, and SHALL render the status as `ACTIVE (<status>)` when a registry status is known, `ACTIVE` when active without status, `-` otherwise.

#### Scenario: cross-referencing active with ps

- **GIVEN** an active claude session
- **WHEN** `active` renders its table row
- **THEN** the row contains the short session ID that `ps` shows for the same process
- **AND** the timestamp column reflects `UpdatedAt`, not `StartedAt`
