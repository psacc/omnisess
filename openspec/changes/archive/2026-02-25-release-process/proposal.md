## Why

omnisess has no defined release process. Users relying on `go install github.com/psacc/omnisess@latest` get whatever HEAD is — no versioned releases, no changelog anchors, no GitHub Releases page. The module is public and the plugin references it by name, so versioned releases are overdue.

## What Changes

- Add `make release` target: bumps the version, creates an annotated git tag (`vX.Y.Z`), pushes the tag to origin, and creates a GitHub Release via `gh release create`
- Add `make tag` as a lower-level target: creates and pushes an annotated git tag without opening a GitHub Release (useful for release candidates, rollback anchors, or CI-driven flows)
- Keep `plugin.json` version in sync with git tags via a documented manual step — no automation yet
- Add `docs/process/release.md`: when to cut a release, semver rules (patch/minor/major), step-by-step instructions, and `plugin.json` sync procedure
- Update `README.md`: add a "Releases" section pointing to the GitHub releases page
- Tag `v0.1.0` as the first release representing current state: working Claude and Cursor sources, public module, plugin

## Capabilities

### New Capabilities

- `release-process`: Makefile targets (`make release`, `make tag`) and documented process for cutting versioned omnisess releases, with `plugin.json` sync guidance

### Modified Capabilities

_(none — no existing spec-level requirements are changing)_

## Impact

- `Makefile`: two new targets added (`release`, `tag`); `.PHONY` updated
- `docs/process/release.md`: new file
- `README.md`: Releases section added
- `plugin.json`: version field manually set to `0.1.0` (once v0.1.0 tag is created)
- No code changes, no new Go dependencies
