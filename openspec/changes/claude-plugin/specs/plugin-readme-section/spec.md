## ADDED Requirements

### Requirement: README contains an "Install as Claude Code Plugin" section
The `README.md` SHALL contain a section titled "Install as Claude Code Plugin". This section SHALL be placed immediately after the existing "Install" section (the `go install` instructions).

#### Scenario: Section is discoverable in the README
- **WHEN** a developer reads `README.md`
- **THEN** they find "Install as Claude Code Plugin" as a top-level or second-level heading immediately following the CLI install section

### Requirement: Section documents the binary prerequisite
The section SHALL state clearly that the `omnisess` binary must be installed separately before the plugin will work, and SHALL provide the `go install` command as the prerequisite step.

#### Scenario: User reads prerequisite before installing plugin
- **WHEN** the user reads the "Install as Claude Code Plugin" section
- **THEN** they see the `go install github.com/psacc/omnisess@latest` command as a prerequisite

### Requirement: Section documents the two-step plugin install flow
The section SHALL document both steps required to install the plugin in Claude Code:
1. Adding the plugin source: `/plugin marketplace add psacc/omnisess`
2. Installing the plugin: `/plugin install omnisess@psacc`

#### Scenario: User follows plugin install steps
- **WHEN** the user runs `/plugin marketplace add psacc/omnisess` followed by `/plugin install omnisess@psacc`
- **THEN** Claude Code installs the plugin and the 4 slash commands become available

### Requirement: Section lists all 4 available slash commands with usage examples
The section SHALL list all 4 slash commands with a brief description and at least one usage example each: `/omnisess:list`, `/omnisess:search`, `/omnisess:active`, `/omnisess:show`.

#### Scenario: User can identify available commands from the README
- **WHEN** the user reads the "Usage" subsection of "Install as Claude Code Plugin"
- **THEN** they see all 4 commands with descriptions and example invocations
