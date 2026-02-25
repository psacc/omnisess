## ADDED Requirements

### Requirement: recap skill wraps `omnisess list --json` and synthesizes a markdown briefing
The `recap` skill at `skills/omnisess-recap/SKILL.md` SHALL:
1. Accept a time window argument (`today`, `yesterday`, or `Nd`) defaulting to `today`
2. Map the window to a `--since` duration and run `omnisess list --since=<duration> --json`
3. Filter the JSON output: discard sessions with empty `preview`; discard sessions where `preview` starts with `/`
4. Group remaining sessions by project path
5. For each project group, summarize with Claude: intent, outcome, and status (complete / in-progress / unclear) per session
6. Output a structured markdown briefing with a date-range header, one section per project, and an optional Pending section for in-progress sessions
7. If no sessions remain after filtering, print "No sessions found for <window>." and exit 0

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

## MODIFIED Requirements

### Requirement: Each skill has a SKILL.md file with valid frontmatter
Each of the 5 skills SHALL have a `SKILL.md` file at `skills/<name>/SKILL.md`. The frontmatter SHALL include: `name`, `description`, `allowed-tools`, and `argument-hint`.

_(Previously "4 skills"; updated to "5 skills" to include `recap`.)_
