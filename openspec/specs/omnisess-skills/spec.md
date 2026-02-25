# Capability: omnisess-skills

## Requirements

### Requirement: Each skill has a SKILL.md file with valid frontmatter
Each of the 5 skills SHALL have a `SKILL.md` file at `skills/<name>/SKILL.md`. The frontmatter SHALL include: `name`, `description`, `allowed-tools`, and `argument-hint`.

#### Scenario: SKILL.md frontmatter is parseable
- **WHEN** Claude Code loads a skill file
- **THEN** the YAML frontmatter is valid and all required fields are present

#### Scenario: allowed-tools is restricted to Bash
- **WHEN** Claude Code inspects the `allowed-tools` field for any of the 5 skills
- **THEN** only `Bash` is listed, since skills exclusively shell out to the omnisess binary

### Requirement: Each skill checks for the omnisess binary before running
Every skill SHALL begin by checking whether `omnisess` is available in the user's PATH. If the binary is not found, the skill SHALL print a human-readable install instruction and exit with a non-zero status code, without attempting to run the CLI command.

#### Scenario: Binary is present — command runs normally
- **WHEN** the user invokes a skill and `omnisess` is in PATH
- **THEN** the skill executes the corresponding CLI subcommand and returns its output

#### Scenario: Binary is absent — clear error is shown
- **WHEN** the user invokes a skill and `omnisess` is NOT in PATH
- **THEN** the skill prints "omnisess not found. Install it with: go install github.com/psacc/omnisess@latest" and exits with status 1

### Requirement: list skill wraps `omnisess list`
The `list` skill at `skills/list/SKILL.md` SHALL invoke `omnisess list` via Bash, passing any user-supplied arguments (e.g., `--tool`, `--limit`, `--json`) through to the CLI.

#### Scenario: List with no arguments
- **WHEN** the user invokes `/omnisess:list` with no arguments
- **THEN** the skill runs `omnisess list` and returns the tabular session output

#### Scenario: List with tool filter
- **WHEN** the user invokes `/omnisess:list --tool claude`
- **THEN** the skill runs `omnisess list --tool claude` and returns filtered output

### Requirement: search skill wraps `omnisess search`
The `search` skill at `skills/search/SKILL.md` SHALL invoke `omnisess search "<query>"` via Bash. The query is the argument passed by the user to the slash command.

#### Scenario: Search with a query string
- **WHEN** the user invokes `/omnisess:search refactor auth`
- **THEN** the skill runs `omnisess search "refactor auth"` and returns matched session snippets

#### Scenario: Search with no query
- **WHEN** the user invokes `/omnisess:search` with no argument
- **THEN** the skill either prompts for a query or passes the empty argument to the CLI, which returns its own usage error

### Requirement: active skill wraps `omnisess active`
The `active` skill at `skills/active/SKILL.md` SHALL invoke `omnisess active` via Bash and return the list of currently running sessions.

#### Scenario: Active sessions detected
- **WHEN** the user invokes `/omnisess:active` and there are live sessions
- **THEN** the skill runs `omnisess active` and returns the active session rows

#### Scenario: No active sessions
- **WHEN** the user invokes `/omnisess:active` and no sessions are active
- **THEN** the skill runs `omnisess active` and returns the CLI's "no active sessions" output

### Requirement: show skill wraps `omnisess show`
The `show` skill at `skills/show/SKILL.md` SHALL invoke `omnisess show <id>` via Bash, where `<id>` is the argument passed by the user in the format `<tool>:<session-id>` (e.g., `claude:5c3f2742`).

#### Scenario: Show a specific session
- **WHEN** the user invokes `/omnisess:show claude:5c3f2742`
- **THEN** the skill runs `omnisess show claude:5c3f2742` and returns the full session transcript

#### Scenario: Show with missing argument
- **WHEN** the user invokes `/omnisess:show` with no argument
- **THEN** the skill passes the empty argument to the CLI, which returns its own usage error

### Requirement: recap skill fetches sessions, filters noise, groups by project, and summarizes
The `recap` skill at `skills/omnisess-recap/SKILL.md` SHALL:
1. Accept a time window argument (`today`, `yesterday`, or `Nd`) defaulting to `today`
2. Map the window to a `--since` duration and run `omnisess list --since=<duration> --json`
3. Filter the JSON output: discard sessions with empty `preview`; discard sessions where `preview` starts with `/`
4. Group remaining sessions by project path
5. For each project group, summarize with Claude: intent, outcome, and status (complete / in-progress / unclear) per session
6. Output a structured markdown briefing with a date-range header, one section per project, and an optional Pending section for in-progress sessions
7. If no sessions remain after filtering, print "No sessions found for <window>." and exit 0
8. This skill is sessions-only — no Slack, Jira, Granola, or Todoist integration

#### Scenario: Recap with default window (today)
- **WHEN** the user invokes `/omnisess:recap` with no argument
- **THEN** the skill runs `omnisess list --since=24h --json`, filters and groups sessions, and renders a markdown briefing covering the last 24 hours

#### Scenario: Recap with explicit window
- **WHEN** the user invokes `/omnisess:recap 3d`
- **THEN** the skill runs `omnisess list --since=72h --json` and renders a briefing for the last 3 days

#### Scenario: Empty previews are filtered out
- **WHEN** the session list contains entries with an empty `preview` field
- **THEN** those sessions are excluded from the briefing

#### Scenario: Slash-command sessions are filtered out
- **WHEN** the session list contains entries whose `preview` starts with `/`
- **THEN** those sessions are excluded as operational noise

#### Scenario: No sessions after filtering
- **WHEN** all sessions are filtered out (empty previews or slash commands only)
- **THEN** the skill prints "No sessions found for <window>." and exits 0 without rendering a briefing

#### Scenario: Binary is absent — clear error is shown
- **WHEN** the user invokes `/omnisess:recap` and `omnisess` is NOT in PATH
- **THEN** the skill prints "omnisess not found. Install it with: go install github.com/psacc/omnisess@latest" and exits with status 1
