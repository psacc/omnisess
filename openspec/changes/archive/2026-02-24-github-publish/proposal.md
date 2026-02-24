## Why

The `sessions` CLI is a general-purpose tool useful to any developer working across multiple AI coding assistants, but it currently exists only as a private local project with no public identity or discoverability. Publishing it under the `psacc` GitHub org as an open-source project makes it installable via `go install`, shareable, and ready to receive community contributions.

## What Changes

- Rename the Go module path from `github.com/psacconier/sessions` to `github.com/psacc/<chosen-name>` (name TBD: `aitrail`, `aiscout`, or `omnisess`) — **BREAKING** (import paths change across all packages)
- Create a public-facing `README.md` covering installation, usage, and supported sources
- Add `LICENSE` (MIT)
- Add `.github/` directory with a CI workflow (`go test ./...` on push/PR) and issue templates (bug report, feature request)
- Update `CLAUDE.md` to reflect the new canonical module path and public repo URL
- Push to `github.com/psacc/<chosen-name>` as a public repository

## Capabilities

### New Capabilities

- `public-readme`: User-facing README covering what the tool does, installation (`go install`), quick-start usage, supported sources, and contributing guidelines
- `license`: MIT `LICENSE` file at the repo root
- `ci-workflow`: GitHub Actions workflow running `go build`, `go vet`, and `go test ./...` on every push and pull request
- `github-issue-templates`: Bug report and feature request issue templates under `.github/ISSUE_TEMPLATE/`
- `module-rename`: Rename the Go module path everywhere it appears (go.mod, all internal import paths, documentation references)

### Modified Capabilities

_(none — no existing spec-level behavior changes; this change is purely about packaging and publication)_

## Impact

- **go.mod / go.sum**: module declaration must change; all consumers of the module (none today, since it is private) would be affected
- **All Go source files**: every `import "github.com/psacconier/sessions/..."` path must be updated to the new path — mechanical but broad
- **CLAUDE.md / ARCHITECTURE.md**: module path and repo URL references updated
- **docs/**: any hardcoded module path references updated
- **No runtime behavior changes**: the CLI feature set is unchanged; this is a packaging-only change plus documentation additions
