## Why

Users of the omnisess Claude Code plugin have no fast way to get a narrative summary of their recent AI coding work without leaving Claude Code. A dedicated `recap` skill reads session data via `omnisess list --json`, groups it by project, and produces a structured markdown briefing — answering "what did I work on today?" in a single slash command.

## What Changes

- Add new skill: `skills/omnisess-recap/SKILL.md` — runs `omnisess list --since=<window> --json`, filters out empty and slash-command sessions, groups by project, summarizes each group with Claude (intent, outcome, status), and renders a clean markdown briefing
- Add new command: `commands/omnisess/recap.md` — thin wrapper delegating to the `omnisess:recap` skill
- Register both in `.claude-plugin/plugin.json` (commands and skills arrays)

## Capabilities

### New Capabilities

- `omnisess-recap-skill`: A sessions-only recap skill for the public plugin. Wraps `omnisess list --since --json`, filters noise (empty previews, slash-command sessions), groups by project, summarizes with Claude, outputs a structured markdown briefing.

### Modified Capabilities

- `omnisess-skills`: Extended — count grows from 4 to 5. The existing binary-guard and frontmatter requirements apply to the new skill as well. Updated requirement text reflects 5 skills total.
- `plugin-manifest`: `commands` array gains `recap`; `skills` array gains `recap`. Version bump to `1.1.0` marks the first feature addition to the plugin layer.

## Impact

- **New files:** `skills/omnisess-recap/SKILL.md`, `commands/omnisess/recap.md`
- **Modified files:** `.claude-plugin/plugin.json`
- **No Go code changes** — purely declarative plugin layer
- **No new dependencies** — recap uses `omnisess list --json` (already implemented) and Claude's own reasoning for summarization
- **Scope boundary:** this is the PUBLIC plugin version — no Slack, Jira, Granola, or Todoist integration. Those integrations remain in the user's local `/recap` skill.
- **No breaking changes**
