## ADDED Requirements

### Requirement: Plugin manifest file exists at canonical path
The repository SHALL contain a plugin manifest at `.claude-plugin/plugin.json` relative to the repo root. This file is the entry point for Claude Code's plugin loader.

#### Scenario: Manifest file is present at the expected path
- **WHEN** a user installs the plugin from `github.com/psacc/omnisess`
- **THEN** Claude Code finds `.claude-plugin/plugin.json` and loads the plugin

### Requirement: Manifest contains required metadata fields
The manifest SHALL include the following top-level fields: `name`, `version`, `description`, `author`, `license`, `repository`, `keywords`, `skills`.

#### Scenario: Manifest is valid and complete
- **WHEN** Claude Code parses `.claude-plugin/plugin.json`
- **THEN** all required fields are present and non-empty

#### Scenario: Name matches the plugin namespace
- **WHEN** Claude Code reads the `name` field
- **THEN** its value is `"omnisess"`, which becomes the slash command namespace prefix

### Requirement: Manifest version is a valid semver string
The `version` field SHALL be a valid semver string (e.g., `"1.0.0"`). It SHALL be kept in sync with the Go module release tag when cutting a new release.

#### Scenario: Version field is semver-compliant
- **WHEN** Claude Code reads the `version` field
- **THEN** it matches the pattern `MAJOR.MINOR.PATCH`

### Requirement: Manifest registers all four skills
The `skills` array SHALL contain exactly 4 entries: `list`, `search`, `active`, and `show`. Each entry SHALL specify a `name` and a `file` path pointing to the corresponding `SKILL.md`.

#### Scenario: All skills are registered in the manifest
- **WHEN** Claude Code reads the `skills` array
- **THEN** it finds entries for `list`, `search`, `active`, and `show`

#### Scenario: Skill file paths resolve correctly
- **WHEN** Claude Code resolves a skill's `file` path (e.g., `skills/list/SKILL.md`)
- **THEN** the file exists at that path relative to the plugin root

### Requirement: Repository field points to the canonical GitHub URL
The `repository` object SHALL have `type: "git"` and `url: "https://github.com/psacc/omnisess"`.

#### Scenario: Repository URL is correct
- **WHEN** Claude Code reads the `repository` field
- **THEN** the URL resolves to the public GitHub repository for the plugin
