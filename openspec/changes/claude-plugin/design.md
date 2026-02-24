## Context

omnisess is a Go CLI at `github.com/psacc/omnisess` that aggregates AI coding sessions. Claude Code supports plugins via a plugin manifest (`plugin.json`) and skill/command files in a conventional directory layout. The plugin layer is purely declarative — no new Go code is needed. The same repository serves both the Go CLI and the Claude Code plugin.

Existing plugins in the wild (`babysitter`, `ops-central`) inform the manifest schema and skill/command format conventions used here.

## Goals / Non-Goals

**Goals:**
- Make `github.com/psacc/omnisess` installable as a Claude Code plugin via `/plugin install`
- Expose 4 slash commands (`/omnisess:list`, `/omnisess:search`, `/omnisess:active`, `/omnisess:show`) that invoke the CLI binary via Bash
- Guard against missing binary with a clear install hint at invocation time
- Document the install flow in `README.md`

**Non-Goals:**
- Bundling or vendoring the omnisess binary inside the plugin — users install it separately
- Implementing a marketplace listing beyond the plugin.json metadata (that is controlled by Claude's marketplace infra)
- Modifying any Go source code
- Supporting Claude Code versions that predate the plugin system

## Decisions

### D1: Plugin manifest lives at `.claude-plugin/plugin.json`

Claude Code's plugin loader expects the manifest at `.claude-plugin/plugin.json` at repo root. This is the same convention used by `babysitter` and `ops-central`.

**Manifest fields:**
```json
{
  "name": "omnisess",
  "version": "<semver synced to Go module tag>",
  "description": "Aggregate and search AI coding sessions from Claude Code, Cursor, Codex, and Gemini",
  "author": "psacc",
  "license": "MIT",
  "repository": { "type": "git", "url": "https://github.com/psacc/omnisess" },
  "keywords": ["sessions", "claude", "cursor", "ai-tools", "search"],
  "skills": [
    { "name": "list",   "file": "skills/list/SKILL.md" },
    { "name": "search", "file": "skills/search/SKILL.md" },
    { "name": "active", "file": "skills/active/SKILL.md" },
    { "name": "show",   "file": "skills/show/SKILL.md" }
  ],
  "commands": []
}
```

Commands are intentionally left as an empty array — the slash commands are defined via `commands/<name>.md` files and registered through the skills array, consistent with how `babysitter` registers its `call.md` command separately from the manifest's `commands` field.

**Alternative considered:** A top-level `plugin.json`. Rejected — `.claude-plugin/` is the documented convention and keeps plugin metadata separate from repo files.

### D2: Skill files use SKILL.md frontmatter + inline instructions

Each skill file at `skills/<name>/SKILL.md` has a YAML frontmatter block followed by plain-English instructions. The frontmatter includes:
- `name`: matches the skills array entry in plugin.json
- `description`: used by Claude to decide when to invoke the skill
- `allowed-tools`: `Bash` only — skills only shell out to the binary
- `argument-hint`: describes what arguments the slash command accepts

**Why this structure:** It mirrors the existing plugin conventions and is what Claude Code's skill loader parses.

### D3: Binary prerequisite check at invocation time

Every skill begins with a guard that checks whether `omnisess` is in PATH before running the command:

```bash
if ! command -v omnisess &>/dev/null; then
  echo "omnisess not found. Install it with:"
  echo "  go install github.com/psacc/omnisess@latest"
  exit 1
fi
```

**Why:** Skills run in the user's shell environment. The binary is a separate install step. A clear error message is far better than a cryptic "command not found" from Claude Code's Bash tool.

**Alternative considered:** Embedding a `go install` call in the skill itself. Rejected — it silently modifies the user's environment and can take tens of seconds, blocking the slash command.

### D4: Command files are thin wrappers that invoke the skill

`commands/<name>.md` files contain a frontmatter description and a single instruction to invoke the corresponding skill via the `Skill` tool. This matches the pattern in `babysitter`'s `commands/call.md`.

### D5: README section placement

The "Install as Claude Code Plugin" section goes immediately after the existing "Install" section (which covers `go install`). This ensures users see both install paths in the natural reading order. The section covers:
1. Prerequisites: `omnisess` binary must be installed first
2. The two-step install: add the marketplace source, then install the plugin
3. Usage: the 4 slash commands with examples

### D6: Version is not auto-synced from Go module tag

The `version` field in `plugin.json` is updated manually when cutting a release. Automating this is out of scope and adds CI complexity for marginal value.

## Risks / Trade-offs

- **Claude Code plugin API is not publicly documented** → Mitigated by reading 2+ real-world plugin manifests and conforming to their structure. If the format changes, the fix is a manifest edit, not a code change.
- **Skill `allowed-tools: Bash` is the only tool needed** → If a future skill needs richer behavior (e.g., parsing output, prompting), the SKILL.md can be updated without touching the manifest.
- **Binary prerequisite is user's responsibility** → Acceptable: the README and skill error message both state this clearly. No silent failure.
- **Plugin name `omnisess` must be unique in the marketplace** → Low risk; the name matches the binary and repo name. If a collision occurs, the manifest `name` field is a one-line change.
- **`commands` field vs command files** — the exact relationship between `manifest.commands` and `commands/*.md` files in Claude Code is inferred from reference plugins. If the loader ignores `commands/*.md` in favor of the manifest array, commands will need to be moved into the manifest. This is a two-way door.

## Open Questions

- Does Claude Code's marketplace index `github.com/psacc/omnisess` automatically from the `.claude-plugin/plugin.json` or does it require a separate submission? Needs verification against Claude Code docs when available.
- Should `/omnisess:show` accept a bare session ID (e.g., `5c3f2742`) and auto-prefix with `claude:`, or require the full `<tool>:<id>` format? Current design passes the argument as-is to the CLI.
