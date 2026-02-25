## Context

The omnisess Claude Code plugin already ships 4 skills (list, search, active, show) that wrap CLI subcommands via Bash. The `recap` skill is different: it does not map 1:1 to a CLI subcommand. Instead, it uses `omnisess list --since --json` as a data source, applies filtering and grouping logic, and then uses Claude's own reasoning to produce a narrative markdown summary. This makes it a first-class "agentic" skill — it reads data and synthesizes, rather than just passing through CLI output.

This is the PUBLIC version. The user's private local `/recap` skill may integrate Slack, Jira, Granola, and Todoist. This skill deliberately excludes those integrations so it can ship as part of the public omnisess plugin without requiring additional tool credentials.

## Goals / Non-Goals

**Goals:**
- Accept a time window argument (today / yesterday / Nd — default: today) and map it to `--since` values omnisess understands
- Retrieve sessions as JSON, filter out noise (empty previews, slash-command sessions where preview starts with `/`)
- Group remaining sessions by project path
- For each project group, prompt Claude to summarize: what was the intent, what was the outcome, and what is the status (complete / in-progress / unclear)
- Output a structured markdown briefing: header with date range, one section per project, a pending-items footer if any sessions are in-progress
- Binary guard: check `command -v omnisess` before running, fail with install hint

**Non-Goals:**
- Integrating external data sources (Slack, Jira, Granola, Todoist) — those belong in the private local skill
- Shipping a dedicated `omnisess recap` CLI subcommand — the skill orchestrates this entirely within Claude Code
- Caching or indexing session data between invocations

## Decisions

### D1: Time window argument maps to `--since` values

The skill accepts `today`, `yesterday`, or `Nd` (e.g., `3d`) as its argument, defaulting to `today`. Mapping:
- `today` → `--since=24h`
- `yesterday` → `--since=48h` with a date-range note that the window is approximate (covers the last 48h, not calendar yesterday strictly)
- `Nd` → `--since=<N*24>h`

**Why:** `omnisess list --since` accepts duration strings. Mapping named windows to durations is simpler than implementing calendar-day filtering in shell. The approximation is acceptable for a daily recap.

**Alternative considered:** Implementing exact calendar-day filtering via `--after`/`--before` flags. Rejected — those flags do not exist in the current CLI; adding them would require Go code changes, violating the no-Go-changes constraint.

### D2: Filtering rules applied to JSON output

Before passing sessions to Claude for summarization, the skill filters out:
1. Sessions with empty `preview` field
2. Sessions where `preview` starts with `/` (slash commands — these are meta-invocations of Claude Code itself, not coding work)

**Why:** Empty-preview sessions carry no summarizable content. Slash-command sessions (e.g., `/recap`, `/omnisess:list`) are operational noise, not work sessions. Filtering them prevents the summary from including self-referential entries.

### D3: Claude synthesizes the summary inline

The skill instructs Claude (via the SKILL.md body) to read the filtered JSON and produce a structured markdown output directly. No external LLM call is needed — Claude Code itself is the summarizer.

**Why:** The skill runs inside Claude Code, so Claude is already available. A bash-only approach (e.g., generating a fixed-format summary via jq templates) would produce lower-quality output than Claude's natural language reasoning.

### D4: Output structure

```markdown
# Recap: <date range>

## <project-path>
- **<session-id>**: <intent> — <outcome> [status: complete|in-progress|unclear]

## Pending
- <session-id> in <project>: <reason>
```

The "Pending" section is omitted if all sessions are complete. Project paths use the basename by default for readability.

### D5: Skill is public-only — no external integrations

The skill body explicitly notes that Slack/Jira/Granola/Todoist integration is out of scope. This is a design boundary, not a limitation. Users who want enriched recaps use a private local skill that imports additional context.

### D6: Command file follows the thin-wrapper pattern

`commands/omnisess/recap.md` follows the same pattern as the 4 existing commands: description frontmatter + single instruction to invoke `omnisess:recap` via the Skill tool.

## Risks / Trade-offs

- **`--since=24h` for "today" is not calendar-accurate** → Acceptable approximation; a strict "since midnight" filter would require Go CLI changes. Users who need exact calendar-day filtering can use `omnisess list --since=<h>` directly.
- **Slash-command filter (`preview` starts with `/`) may miss edge cases** → Low risk; the vast majority of real coding sessions start with a natural-language description. If false positives emerge, the filter can be tightened in the skill body.
- **Claude's summary quality depends on session preview richness** → Mitigated by filtering empty previews. Short previews will produce brief summaries, which is still better than no summary.
- **`plugin.json` version bump to 0.2.0** → Minor semver bump signals a non-breaking feature addition. Existing skill invocations are unaffected.

## Open Questions

- Should the skill emit a warning when the filtered session count is zero, rather than an empty briefing? Current design: emit a "No sessions found for <window>" message and exit cleanly.
- Should `yesterday` map to `--since=48h` (rolling) or attempt a stricter `--since=24h --before=<midnight>` (calendar)? Current design: rolling 48h window with a note.
