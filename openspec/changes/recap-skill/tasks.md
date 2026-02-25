## 1. Skill File

- [ ] 1.1 Create `skills/omnisess-recap/` directory
- [ ] 1.2 Write `skills/omnisess-recap/SKILL.md` with:
  - Frontmatter: `name: recap`, `description` (trigger phrases: "recap", "what did I work on", "daily summary", "session summary"), `allowed-tools: Bash`, `argument-hint: "[today|yesterday|Nd]"`, `version: 1.0.0`
  - Binary guard: `command -v omnisess` check with install hint
  - Argument handling: default to `today` if no argument provided; map `today`→`24h`, `yesterday`→`48h`, `Nd`→`<N*24>h`
  - Data fetch: `omnisess list --since=<window> --json`
  - Filter step: instruct Claude to discard sessions with empty `preview` and sessions where `preview` starts with `/`
  - Group step: instruct Claude to group remaining sessions by `project` field
  - Summarize step: for each group, produce intent / outcome / status per session
  - Output step: render structured markdown briefing (header, per-project sections, optional Pending section)
  - Empty-result guard: if no sessions remain after filtering, print "No sessions found for <window>." and exit 0

## 2. Command File

- [ ] 2.1 Write `commands/omnisess/recap.md` with:
  - Frontmatter: `description`, `argument-hint: "[today|yesterday|Nd]"`, `allowed-tools: Bash`
  - Body: single instruction to invoke `omnisess:recap` skill via the Skill tool

## 3. Plugin Manifest

- [ ] 3.1 Add `{ "name": "recap", "file": "commands/omnisess/recap.md" }` to the `commands` array in `.claude-plugin/plugin.json`
- [ ] 3.2 Add `{ "name": "recap", "file": "skills/omnisess-recap/SKILL.md" }` to the `skills` array in `.claude-plugin/plugin.json`
- [ ] 3.3 Bump `version` from `1.0.0` to `1.1.0` in `.claude-plugin/plugin.json`
- [ ] 3.4 Verify JSON is valid: `jq . .claude-plugin/plugin.json`

## 4. OpenSpec Global Spec Update

- [ ] 4.1 Update `openspec/specs/omnisess-skills/spec.md` to add the `recap` skill requirements and update the skill count from 4 to 5

## 5. Verification

- [ ] 5.1 Confirm `skills/omnisess-recap/SKILL.md` frontmatter is valid YAML with all required fields
- [ ] 5.2 Confirm binary guard (`command -v omnisess`) is present
- [ ] 5.3 Confirm filter logic (empty preview, `/`-prefix filter) is documented in skill body
- [ ] 5.4 Confirm `.claude-plugin/plugin.json` `recap` entries resolve to existing files
- [ ] 5.5 Read the skill body end-to-end and verify it covers: binary check, argument default, data fetch, filter, group, summarize, output, empty-result guard
