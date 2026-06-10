# ps-codex-detection — Delta Spec

## ADDED Requirements

### Requirement: Live codex sessions appear in omnisess ps
`omnisess ps` SHALL list live Codex sessions alongside Claude sessions in the merged process tree. A codex session is live when a running process whose executable basename is `codex` holds a rollout file (`~/.codex/sessions/**/rollout-*-<uuid>.jsonl`) open.

#### Scenario: Codex TUI session is listed
- **WHEN** a codex TUI process is running and holds its rollout JSONL open
- **THEN** `omnisess ps` renders a leaf labeled `codex` with the session's short id, project basename, entrypoint, and age, under the process's ancestor chain

#### Scenario: No codex processes running
- **WHEN** no process with executable basename `codex` exists
- **THEN** `omnisess ps` performs no lsof invocation and lists only claude sessions

#### Scenario: App-server with multiple live threads
- **WHEN** a single codex process holds N rollout files open
- **THEN** the snapshot contains N codex sessions sharing that PID

### Requirement: Session metadata from rollout session_meta with filename fallback
The codex enumerator SHALL parse the first line of each held rollout file as `session_meta` and populate session id, cwd, started-at, entrypoint (from `originator`), and version (from `cli_version`). If the first line cannot be read or parsed, the enumerator SHALL fall back to the session id and start time encoded in the rollout filename and the process cwd reported by lsof.

#### Scenario: Valid session_meta line
- **WHEN** the rollout's first line is a valid `session_meta` record
- **THEN** the session carries the id, cwd, version, entrypoint, and start time from the record

#### Scenario: Malformed first line
- **WHEN** the rollout's first line is not valid `session_meta` JSON
- **THEN** the session is still emitted, with id and start time parsed from the filename and cwd from lsof

### Requirement: Tool-aware sessions and rendering
`procsnap.Session` SHALL carry a `Tool` field (`claude` or `codex`). `omnisess ps` SHALL render the tool name in each leaf label and include the field in `--json` output. Claude sessions SHALL carry `Tool: claude`.

#### Scenario: JSON output includes tool
- **WHEN** `omnisess ps --json` runs with live claude and codex sessions
- **THEN** every session object includes a `Tool` field identifying its source tool

#### Scenario: TUI snapshot contract unchanged
- **WHEN** the TUI applies a snapshot containing codex sessions
- **THEN** only claude model sessions have their Active flag and Title affected

### Requirement: Graceful degradation on codex enumeration failure
A failure to enumerate codex sessions (lsof missing, lsof error, unreadable sessions dir) SHALL NOT fail `omnisess ps`. The command SHALL warn on stderr and return claude sessions only.

#### Scenario: lsof fails
- **WHEN** the lsof invocation returns an error
- **THEN** `omnisess ps` exits 0, prints the claude-only tree, and writes a warning to stderr

### Requirement: Read-only access to codex data
Codex enumeration SHALL only read from `~/.codex` and the process table; it SHALL NOT create, modify, or delete any file (invariant #7).

#### Scenario: Enumeration leaves no trace
- **WHEN** codex enumeration runs
- **THEN** no file under `~/.codex` is written or removed
