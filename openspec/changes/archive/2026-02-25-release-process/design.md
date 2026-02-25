## Context

omnisess is a public Go module (`github.com/psacc/omnisess`) with a Claude Code plugin (`plugin.json`). Users install via `go install github.com/psacc/omnisess@latest` or via the plugin marketplace. Without tagged releases, `@latest` resolves to the HEAD pseudo-version, making it impossible to pin stable versions, reference changelogs, or communicate breaking changes. The GitHub Releases page is empty. There is no documented procedure for cutting a release.

The repo already has `make pr` and `make merge` for branch workflow automation. Adding `make release` and `make tag` follows the same pattern.

## Goals / Non-Goals

**Goals:**
- Add `make tag VERSION=vX.Y.Z`: creates an annotated git tag and pushes it to origin
- Add `make release VERSION=vX.Y.Z`: runs `make tag`, then creates a GitHub Release via `gh release create`
- Add `docs/process/release.md`: semver guidance, when-to-release criteria, step-by-step instructions, and `plugin.json` sync procedure
- Update `README.md` with a Releases section pointing to GitHub releases
- Tag `v0.1.0` as first release representing current working state
- Document the manual `plugin.json` version sync step

**Non-Goals:**
- Automated `plugin.json` version bumping
- CHANGELOG generation or release notes automation
- Cross-platform binary packaging or distribution beyond `go install`
- CI-triggered releases
- Release candidates or pre-release versioning conventions (can add later)

## Decisions

**`make tag` is a prerequisite of `make release`, not inlined**
Splitting tag creation from GitHub Release creation lets developers create a tag for internal testing or rollback anchoring without publishing a GitHub Release. This is the minimum useful split. The alternative (a single `make release` that does everything) would remove this flexibility for no savings.

**VERSION is a required Makefile variable, not auto-derived**
Auto-deriving the next version (e.g., from the latest tag + bump type) requires deciding bump logic and handling edge cases (dirty tree, pre-release tags). Explicit `VERSION=vX.Y.Z` at the call site is unambiguous, matches the mental model of the release author, and avoids a bump-logic dependency. Enforcement: the targets check `$(VERSION)` is non-empty and exits with a usage error if not.

**Annotated tags, not lightweight tags**
Annotated tags carry a tagger identity, timestamp, and message — they are what `go list -m -versions` and `pkg.go.dev` use to resolve module versions. Lightweight tags work for local bookmarks but are not appropriate for public releases. `git tag -a` with `-m "Release vX.Y.Z"` is the minimum viable annotated tag.

**`gh release create` with `--generate-notes`**
`gh release create --generate-notes` produces release notes from commit messages since the previous tag. This is good enough for early releases and adds zero overhead. A human can edit the notes on GitHub after creation if needed.

**`plugin.json` sync is a manual documented step, not automated**
`plugin.json` is not a Go artifact — it is not read by `go build`. Automating its version bump would require either a separate script (parsing and rewriting JSON) or a `sed` one-liner that is brittle on version field format changes. The manual step is a two-minute task documented in `release.md`. When the pain of doing it manually exceeds the cost of automation, automate it.

**v0.1.0 as first release**
The repo has working Claude and Cursor sources, a public module path, and a plugin. The codebase is useful in its current state. Semver 0.x signals unstable API — appropriate since the Source interface and CLI flags may still change. v0.1.0 anchors the first useful state without implying stability.

## Risks / Trade-offs

- [Risk] Developer forgets to sync `plugin.json` after tagging → Mitigation: `release.md` step 6 is explicit; `make release` prints a reminder after success
- [Risk] `gh` CLI not installed or not authenticated → Mitigation: `make release` checks for `gh` with `command -v` and exits with an install hint, same pattern as `golangci-lint` check
- [Risk] Pushing a tag to a wrong commit → Mitigation: `make tag` prints the commit SHA being tagged and asks for confirmation before pushing (or documents that the developer should confirm SHA before running)
- [Trade-off] `--generate-notes` may produce noisy release notes from chore/docs commits → Acceptable at v0.x; human can edit on GitHub after creation

## Open Questions

_(none)_
