## Why

omnisess aggregates AI coding sessions across Claude Code, Cursor, Codex, and Gemini but requires users to leave their editor to query session data. Shipping it as a Claude Code plugin lets developers invoke session commands directly from Claude Code via slash commands, eliminating context switches and making the tool discoverable in the plugin marketplace.

## What Changes

- Add `.claude-plugin/plugin.json` manifest turning the repo into an installable Claude Code plugin
- Add 4 skills under `skills/`: `list`, `search`, `active`, `show` — each wrapping the corresponding `omnisess` CLI subcommand via Bash
- Add command definitions under `commands/` for each skill's slash command entry point
- Add "Install as Claude Code Plugin" section to `README.md`
- Plugin is self-hosted from `github.com/psacc/omnisess` — no separate repo needed

## Capabilities

### New Capabilities

- `plugin-manifest`: `.claude-plugin/plugin.json` with metadata, version, and marketplace entry point
- `omnisess-skills`: 4 skills (list, search, active, show) each wrapping an `omnisess` CLI subcommand via Bash, with a prerequisite check for the binary in PATH
- `plugin-readme-section`: "Install as Claude Code Plugin" section added to `README.md` covering both plugin install and CLI binary prerequisite

### Modified Capabilities

<!-- none — no existing spec requirements change -->

## Impact

- **New files:** `.claude-plugin/plugin.json`, `skills/list/SKILL.md`, `skills/search/SKILL.md`, `skills/active/SKILL.md`, `skills/show/SKILL.md`, `commands/list.md`, `commands/search.md`, `commands/active.md`, `commands/show.md`
- **Modified files:** `README.md`
- **No Go code changes** — plugin layer wraps the existing CLI binary
- **Dependency:** users must have `omnisess` installed separately (`go install github.com/psacc/omnisess@latest`)
- **No breaking changes**
