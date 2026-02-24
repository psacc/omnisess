## 1. Plugin Manifest

- [x] 1.1 Create `.claude-plugin/` directory at repo root
- [x] 1.2 Write `.claude-plugin/plugin.json` with `name`, `version`, `description`, `author`, `license`, `repository`, `keywords`, and `skills` array registering all 4 skills
- [x] 1.3 Verify manifest JSON is valid (e.g., `jq . .claude-plugin/plugin.json`)

## 2. Skill Files (list, search, active, show — can be written in parallel)

- [x] 2.1 Create `skills/list/SKILL.md` with frontmatter (`name: list`, `allowed-tools: Bash`, `argument-hint: [--tool <name>] [--limit N] [--json]`) and body: binary check + `omnisess list "$@"`
- [x] 2.2 Create `skills/search/SKILL.md` with frontmatter (`name: search`, `allowed-tools: Bash`, `argument-hint: <query>`) and body: binary check + `omnisess search "$*"`
- [x] 2.3 Create `skills/active/SKILL.md` with frontmatter (`name: active`, `allowed-tools: Bash`, `argument-hint: (none)`) and body: binary check + `omnisess active`
- [x] 2.4 Create `skills/show/SKILL.md` with frontmatter (`name: show`, `allowed-tools: Bash`, `argument-hint: <tool>:<session-id>`) and body: binary check + `omnisess show "$1"`

## 3. Command Files (can be written in parallel with skill files)

- [x] 3.1 Create `commands/list.md` with description frontmatter and a single instruction to invoke the `list` skill
- [x] 3.2 Create `commands/search.md` with description frontmatter and a single instruction to invoke the `search` skill
- [x] 3.3 Create `commands/active.md` with description frontmatter and a single instruction to invoke the `active` skill
- [x] 3.4 Create `commands/show.md` with description frontmatter and a single instruction to invoke the `show` skill

## 4. README Section

- [x] 4.1 Add "Install as Claude Code Plugin" section to `README.md` immediately after the existing "Install" section
- [x] 4.2 Section must include: binary prerequisite (`go install`), two-step plugin install (`/plugin marketplace add psacc/omnisess`, `/plugin install omnisess@psacc`), and usage examples for all 4 slash commands

## 5. Verification

- [x] 5.1 Confirm `.claude-plugin/plugin.json` JSON is valid and all `file` paths in `skills` array resolve to existing `SKILL.md` files
- [x] 5.2 Confirm each `SKILL.md` contains the binary guard (`command -v omnisess`) and the correct CLI subcommand
- [x] 5.3 Read the final README section end-to-end and verify it covers prerequisite, install steps, and all 4 commands
