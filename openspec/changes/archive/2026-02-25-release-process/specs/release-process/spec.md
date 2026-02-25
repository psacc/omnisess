## ADDED Requirements

### Requirement: make tag creates and pushes an annotated git tag
The Makefile SHALL provide a `tag` target that accepts a `VERSION` variable, creates an annotated git tag named `$(VERSION)` with message `Release $(VERSION)`, and pushes the tag to origin.

#### Scenario: tag created successfully
- **WHEN** the developer runs `make tag VERSION=v0.1.0`
- **THEN** an annotated git tag `v0.1.0` is created on the current commit and pushed to `origin`

#### Scenario: VERSION not provided
- **WHEN** the developer runs `make tag` without a `VERSION` variable
- **THEN** the target exits non-zero and prints a usage error indicating that `VERSION` is required (e.g., `make tag VERSION=v0.1.0`)

### Requirement: make release creates a GitHub Release
The Makefile SHALL provide a `release` target that depends on `tag`, checks that the `gh` CLI is installed, and runs `gh release create $(VERSION) --generate-notes` to publish a GitHub Release.

#### Scenario: release published successfully
- **WHEN** the developer runs `make release VERSION=v0.1.0` with `gh` installed and authenticated
- **THEN** the annotated tag `v0.1.0` is created and pushed, a GitHub Release is published with auto-generated notes, and a reminder to sync `plugin.json` is printed

#### Scenario: gh CLI not installed
- **WHEN** the developer runs `make release` and `gh` is not on PATH
- **THEN** the target exits non-zero and prints an install hint for the `gh` CLI

#### Scenario: VERSION not provided
- **WHEN** the developer runs `make release` without a `VERSION` variable
- **THEN** the target exits non-zero with a usage error (inherited from the `tag` dependency)

### Requirement: release.md documents the release process
The repository SHALL contain `docs/process/release.md` that documents when to cut a release, semver rules, step-by-step release instructions using `make tag` and `make release`, and the manual `plugin.json` version sync procedure.

#### Scenario: developer follows release.md to cut a release
- **WHEN** a developer follows the steps in `docs/process/release.md`
- **THEN** they can cut a versioned release with a tagged commit, a GitHub Release, and a synced `plugin.json`

#### Scenario: semver rules are unambiguous
- **WHEN** a developer reads the semver section of `release.md`
- **THEN** they can determine whether a change is a patch, minor, or major bump based on the documented criteria

### Requirement: plugin.json version sync is documented
The `docs/process/release.md` SHALL include an explicit step to manually update the `version` field in `plugin.json` to match the new release tag after tagging.

#### Scenario: developer follows plugin.json sync step
- **WHEN** a developer follows the `plugin.json` sync step in `release.md`
- **THEN** they update the `version` field in `.claude-plugin/plugin.json` to match the released version and commit the change

### Requirement: README.md Releases section exists
The `README.md` SHALL include a "Releases" section that references the GitHub releases page at `https://github.com/psacc/omnisess/releases`.

#### Scenario: user looks for versioned releases
- **WHEN** a user reads the README.md
- **THEN** they find a Releases section that links to the GitHub releases page for version history and changelogs
