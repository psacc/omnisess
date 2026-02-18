# 006 — Session Recap Skill

**Status**: Not started
**Priority**: Next (after JSON control-char fix lands on main)
**Estimated effort**: 3-4 hours

## Purpose

Enable Claude Code to answer contextual questions about recent work:

- "What are we currently working on?"
- "What did we work on today?"
- "What did we work on yesterday?"

Claude Code has zero non-interactive session listing capability. The `sessions` CLI fills this gap for metadata, but raw transcripts are too large (240K tokens across 28 sessions in 24h) for a single LLM context. A Claude Code skill bridges the two: CLI for data retrieval, LLM for summarization.

## User Stories

1. **Daily standup prep**: Developer invokes `/recap today` before standup. Gets a project-grouped summary of all sessions, with what was accomplished and what's still in progress.
2. **Context recovery**: Developer starts a new Claude Code session. Asks "what were we working on in this project?" and gets a summary of recent sessions scoped to the current project.
3. **Yesterday recap**: Developer asks "what did we do yesterday?" and gets a time-windowed summary grouped by project.
4. **Active work**: Developer asks "what's currently in progress?" and gets only sessions with active indicators or very recent updates.

## Why a Skill, Not a Go Subcommand

The `sessions summary` subcommand planned in 004-improvements.md targets **heuristic** per-session summaries (first user message + last assistant message + stats). That's useful for CLI output but insufficient for the recap use case.

Recap requires:

1. **Semantic compression** — A 52K-token session cannot be meaningfully represented by first/last message. The LLM must read the full transcript and extract what actually happened (files changed, bugs fixed, features added, decisions made).
2. **Cross-session aggregation** — Grouping 17 sessions by project and synthesizing a coherent narrative across them is an LLM task, not a formatting task.
3. **Natural language interface** — Users ask freeform questions ("what did we work on?", "any progress on the auth refactor?"). A skill handles this natively; a subcommand would need flag gymnastics.
4. **Composability** — A skill can call the CLI for data, use an LLM for summarization, and return structured prose. This keeps the Go binary simple (data retrieval) and the skill smart (interpretation).

The heuristic `sessions summary` subcommand (004) remains valuable as a fast, zero-cost fallback. The skill can use it as a first pass, then drill into full transcripts only for sessions that need deeper summarization.

## Technical Approach

### Architecture

```
User question ("what did we work on today?")
        |
        v
Claude Code skill (.claude/commands/recap.md)
        |
        v
sessions list --since=<window> --json    (metadata, ~60ms)
        |
        v
Filter: drop sessions with empty/slash-command previews
        |
        v
For each substantive session:
    sessions show <tool:id> --json        (~60ms each)
        |
        v
    Per-session summarization (haiku-class, ~3s each)
        |
        v
Aggregate summaries by project
        |
        v
Structured recap response to user
```

### Session Filtering Strategy

From validation data: 28 sessions in 24h, only 17 substantive. Filter criteria:

1. **Empty preview** — Sessions where `Preview` is empty string → skip
2. **Slash-command-only** — Sessions where `Preview` starts with `/` (e.g., `/mcp`, `/memory`, `/slack`) → skip
3. **Tiny sessions** — After fetching, sessions with < 3 messages or < 500 chars total content → skip (these are typically abandoned sessions)

Expected reduction: 28 → ~15-17 sessions needing summarization.

### Token Budget Analysis

From real data (last 24h):

| Metric | Value |
|--------|-------|
| Total sessions | 28 |
| Substantive sessions | ~17 |
| Total transcript tokens | ~240K |
| Avg per session | ~8.5K |
| Max per session | ~52K |
| Median per session | ~4K (estimated) |

**Why per-session summarization is required**: 240K input tokens exceeds Claude's context window for a single pass. Even after filtering to ~17 sessions, the total is ~200K tokens — still too large.

**Per-session summarization approach**:
- Each session (avg 8.5K tokens) is independently summarized to ~200-400 tokens
- 17 sessions * ~300 tokens avg = ~5K tokens of summaries → fits easily in final aggregation context
- Use `haiku`-class model for per-session summarization (fast, cheap)

**Cost estimate per recap**:
- Input: ~145K tokens via haiku = ~$0.004
- Output: 17 * 300 tokens = ~5K tokens = ~$0.001
- Total: ~$0.005 per recap (negligible)

### Wall Time Estimate

- `sessions list --json`: ~3.8s (current, includes filesystem scan)
- `sessions show` per session: ~60ms * 17 = ~1s
- Haiku summarization: ~3s per session (parallelizable to ~5-6s with batching)
- Final aggregation: ~3s
- **Total: ~15-20s** (dominated by LLM calls)

Optimization path: parallelize `show` + summarize calls. With 4-way parallelism, LLM phase drops to ~15s. With 8-way: ~8s. Total could reach ~12s.

### Output Format

Grouped by project, with status indicators:

```
## Session Recap — Today (Feb 18, 2026)

### psacc/sessions (5 sessions)
- [ACTIVE] Implementing session recap skill — wrote exec plan, designed
  filtering strategy and per-session summarization approach
- Fixed JSON control character bug in Claude parser — unicode escaping
  for \x00-\x1f chars in session content
- Added agent-to-agent review process — decision classification rules,
  reviewer checklist, escalation format

### finn/b2b-orders-api (3 sessions)
- Refactored HubSpot webhook handler — split into separate handlers per
  event type, added idempotency checks
- Debugged order sync timeout — root cause was missing index on
  external_id column

### finn/backoffice (2 sessions)
- Updated user permissions UI — added role-based access control matrix
  to settings page
```

Each entry: one-line summary of what was accomplished, prefixed with `[ACTIVE]` if the session is still running.

### Skill File Structure

```
.claude/commands/recap.md
```

The skill is a single Markdown file containing:
1. A system prompt defining the recap behavior
2. Instructions to call `sessions list` and `sessions show`
3. The summarization prompt template for per-session compression
4. The aggregation prompt template for final output

The skill accepts a natural language argument (e.g., `/recap today`, `/recap yesterday`, `/recap this week`) and maps it to `--since` flags:
- "today" / no arg → `--since=1d`
- "yesterday" → `--since=2d` (then filter to only yesterday's sessions)
- "this week" → `--since=7d`
- "current project" → `--since=7d --project=<cwd basename>`

## Relationship to 004-improvements.md

The `sessions summary` subcommand in 004 is **complementary**, not replaced:

| Aspect | `sessions summary` (004) | `/recap` skill (006) |
|--------|--------------------------|----------------------|
| Implementation | Go subcommand | Claude Code skill |
| Summarization | Heuristic (first/last message + stats) | LLM-powered (full transcript) |
| Cost | Zero (no API calls) | ~$0.005 per invocation |
| Latency | <100ms | 15-20s |
| Use case | Quick glance at what a session was about | Deep understanding of what was accomplished |
| Output | Per-session, CLI table | Cross-session, grouped by project |

The skill can use `sessions summary` output (once built) as a **pre-filter**: scan heuristic summaries first, only fetch full transcripts for sessions that seem relevant to the user's question. This would cut token usage significantly.

## Implementation Steps

### Step 1: Create the skill file
**File**: `.claude/commands/recap.md`

Write the skill prompt that:
- Parses the user's time window from the argument
- Runs `sessions list --since=<window> --json`
- Filters out empty/stub sessions
- For remaining sessions, runs `sessions show <id> --json`
- Summarizes each session individually
- Aggregates by project and returns formatted output

### Step 2: Design the per-session summarization prompt

The prompt must instruct the LLM to extract from a raw transcript:
- What the user was trying to accomplish (intent)
- What was actually done (files modified, decisions made, bugs fixed)
- Current status (completed, in-progress, blocked)
- Key artifacts (file paths, PR URLs, error messages)

Target: 200-400 tokens per session summary.

### Step 3: Design the aggregation prompt

Takes all per-session summaries + the user's original question and produces:
- Project-grouped output
- Status indicators (active, completed today, in-progress)
- Answer to the specific question asked (not just a dump)

### Step 4: Handle edge cases

- **No sessions in window**: Return "No sessions found in the last <window>."
- **All sessions are stubs**: Return "Found N sessions but all were empty (slash commands or abandoned)."
- **Single project**: Skip project grouping, just list session summaries.
- **Very long window (7d+)**: Warn about cost/time, suggest narrowing.
- **Binary not found**: Clear error message about installing `sessions` CLI.

### Step 5: Smoke test against real data

```bash
# Ensure sessions binary is built and on PATH
go build -o ~/bin/sessions .

# Test the skill
claude /recap today
claude /recap yesterday
claude /recap "this week"
```

Verify:
- Correct time window interpretation
- Empty sessions filtered out
- Summaries are accurate (spot-check against `sessions show`)
- Project grouping is correct
- Active indicators match `sessions active` output
- Wall time is within 15-20s budget

### Step 6: Iterate on prompt quality

Based on smoke test results:
- Tune the per-session summarization prompt for better signal extraction
- Adjust filtering thresholds (min messages, min content length)
- Refine aggregation prompt for more actionable output

## Testing Strategy

### Manual testing (primary)

This is a skill, not Go code — no unit tests apply. Testing is:

1. **Accuracy**: Run `/recap today`, then manually verify 3-4 session summaries against `sessions show <id>` output. Summaries should capture the main intent and outcome.
2. **Filtering**: Verify empty/stub sessions are excluded. Count sessions in recap output vs. `sessions list --since=1d | wc -l`.
3. **Time windows**: Test "today", "yesterday", "this week" — verify correct `--since` mapping and date filtering.
4. **Edge cases**: Test with `--project` filter, test when no sessions exist in window, test with a single session.
5. **Performance**: Time the full recap. Should be 15-20s for ~17 sessions.

### Regression boundary

The skill depends on `sessions list --json` and `sessions show --json` output format. If these change, the skill breaks. Document this coupling.

**Contract**: The JSON output from `list` must include: `ID`, `Tool`, `Project`, `Preview`, `StartedAt`, `UpdatedAt`, `Active`. The JSON output from `show` must include the `Messages` array with `Role`, `Content`, `Timestamp`.

### Future: automated validation

Once `sessions summary` (004) exists, we could cross-check: skill summaries should be semantically consistent with heuristic summaries. Not automatable yet.

## Decisions

None requiring escalation. The key architectural choice (skill vs. subcommand) is a two-way door — the skill file can be deleted or replaced at any time with zero impact on the Go codebase.

## Open Questions

1. **Parallelism strategy**: Should the skill issue `sessions show` calls sequentially or instruct Claude to batch them? Claude Code skills can run multiple bash commands, but the summarization happens in the LLM context, not as separate API calls. Need to test whether Claude Code handles many sequential `sessions show` calls efficiently.
2. **Token counting**: The skill has no way to count tokens before fetching. If a session is 52K tokens, the summarization step for that session may need to truncate. Should we add a `--max-tokens` flag to `sessions show` that truncates output? Or handle truncation in the skill prompt?
3. **Cache invalidation**: If the user runs `/recap today` twice in 10 minutes, should we cache summaries? Probably not for v1 — the cost is negligible and sessions may have progressed.
