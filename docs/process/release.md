# Release Process

How to cut a versioned omnisess release.

## When to Cut a Release

Cut a release when `main` contains a coherent, user-visible improvement:

- A new source is promoted from stub to working (e.g., Codex, Gemini)
- A new CLI command or flag reaches stable behavior
- A bug fix that meaningfully improves reliability for existing users
- The plugin or module interface changes in a way users should pin against

Do not release for doc-only changes, internal refactors, or chore commits unless they accompany a user-visible change.

## Semver Rules

omnisess follows [Semantic Versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`.

| Change type | Bump |
|---|---|
| Backwards-compatible bug fix | PATCH (`v0.1.0` → `v0.1.1`) |
| New feature, backwards-compatible | MINOR (`v0.1.1` → `v0.2.0`) |
| Breaking CLI flag, interface, or module path change | MAJOR (`v0.x` → `v1.0.0`) |

While the version is `v0.x.y`, minor bumps may include breaking changes — this signals an unstable API period. Move to `v1.0.0` when the Source interface and CLI surface are stable.

## Step-by-Step Release Instructions

### Prerequisites

- `gh` CLI installed and authenticated (`gh auth status`)
- `main` branch is clean and `make check` passes
- You are on `main` with the commit you want to tag checked out

### Steps

1. Confirm `main` is ready:

   ```bash
   git checkout main
   git pull origin main
   make check
   ```

2. Decide the version number using the semver rules above. Use the format `vX.Y.Z`.

3. Create the annotated tag and push it:

   ```bash
   make tag VERSION=v0.2.0
   ```

   This creates an annotated git tag on the current commit and pushes it to origin. Verify the commit SHA before proceeding.

4. Publish the GitHub Release:

   ```bash
   make release VERSION=v0.2.0
   ```

   This runs `make tag` (idempotent if already done) then calls `gh release create v0.2.0 --generate-notes`. GitHub auto-generates release notes from commits since the previous tag. You can edit the notes on the GitHub UI after creation.

5. Confirm the release is live:

   ```bash
   git tag --list | sort -V   # confirms local tag
   gh release view v0.2.0     # confirms GitHub release
   ```

## Version History

| Version | Date | Notes |
|---|---|---|
| v0.1.0 | 2026-02-24 | First release: working Claude and Cursor sources, public module, Claude Code plugin |
