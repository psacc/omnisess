---
name: recap
description: |
  Produce a structured markdown briefing of AI coding sessions for a given time window.
  Groups sessions by project and summarizes each group with intent, outcome, and status.
  Use this skill when asked to recap recent coding work, summarize today's sessions,
  show what was worked on, or produce a daily/weekly session summary.
  Trigger phrases: "recap", "what did I work on", "daily summary", "session summary",
  "summarize my sessions", "what sessions did I have today", "coding recap".
  This is the sessions-only public version — no Slack, Jira, Granola, or Todoist integration.
allowed-tools: Bash
argument-hint: "[today|yesterday|Nd]"
version: 1.0.0
---

# omnisess recap

Produce a structured markdown briefing of AI coding sessions for a given time window.

## Usage

```
/omnisess:recap [today|yesterday|Nd]
```

- Default window: `today` (last 24 hours)
- `yesterday` — last 48 hours (rolling approximation)
- `Nd` — last N days (e.g., `3d` = last 72 hours)

## Instructions

### Step 1: Binary guard

Check that the `omnisess` binary is available before proceeding:

```bash
if ! command -v omnisess &>/dev/null; then
  echo "omnisess not found. Install it with:"
  echo "  go install github.com/psacc/omnisess@latest"
  exit 1
fi
```

### Step 2: Resolve the time window

Determine the `--since` duration from the argument (default to `today` if none provided):

- `today` (or no argument) → `--since=24h`
- `yesterday` → `--since=48h`
- `Nd` where N is an integer → `--since=<N*24>h` (e.g., `3d` → `--since=72h`)

If the argument does not match any of the above patterns, print:

```
Unknown window: <arg>. Use: today, yesterday, or Nd (e.g., 3d).
```

and exit 1.

### Step 3: Fetch sessions as JSON

Run:

```bash
omnisess list --since=<resolved-duration> --json
```

Capture the JSON output. If the command fails or returns no output, print:

```
No sessions found for <window>.
```

and exit 0.

### Step 4: Filter sessions

From the JSON array, discard:

1. Sessions where the `preview` field is empty or missing
2. Sessions where the `preview` field starts with `/` (these are Claude Code slash-command invocations — operational noise, not coding work)

If the filtered list is empty, print:

```
No sessions found for <window>.
```

and exit 0.

### Step 5: Group by project

Group the remaining sessions by their `project` field (the project path). Use the basename of the path for display (e.g., `~/prj/myapp` → `myapp`), but preserve the full path for the session detail lines.

### Step 6: Summarize each group

For each project group, read the session previews and reason about:

- **Intent**: What the user was trying to accomplish in this session
- **Outcome**: What was achieved or produced (be specific if the preview contains enough detail)
- **Status**: One of `complete`, `in-progress`, or `unclear`

Keep summaries concise (1–2 sentences per session).

### Step 7: Render the markdown briefing

Output the following structure:

```markdown
# Recap: <date range>

_<N> sessions across <M> projects_

## <project-basename>

- **<tool>:<short-id>** (`<full-project-path>`): <intent> — <outcome> · _<status>_

## Pending

- **<tool>:<short-id>** in `<project-basename>`: <reason why in-progress or unclear>
```

Rules:
- The date range header uses the resolved window (e.g., "last 24h", "last 3 days")
- The `## Pending` section is omitted entirely if all sessions have status `complete`
- List sessions within each project group in reverse chronological order (most recent first)
- If a project has only one session, keep the section header but do not add a sub-list — use a single bullet

### Notes

This skill is the public sessions-only version of `/recap`. It does not integrate with Slack, Jira, Granola, or Todoist. Those integrations belong in a private local skill.
