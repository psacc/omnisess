## Context

No CI exists today. The `Makefile` already encapsulates the full quality gate (`make check`: `gofmt`, `go vet`, `golangci-lint`, `go test`). GitHub Actions is the natural host — free for public repos, zero infra to manage.

The existing `ci-workflow` spec defined two jobs (`build` + `test`) with raw `go` commands. This design replaces that with a single `check` job delegating to `make check`, aligning CI with local developer workflow.

## Goals / Non-Goals

**Goals:**
- Run `make check` automatically on every push and on PRs to `main`
- Single job named `check` so branch protection can reference it by name
- Cache Go modules to keep runs fast
- Go 1.22 pinned to match `go.mod`

**Non-Goals:**
- No build artifact upload
- No deployment or release steps
- No matrix (multi-OS, multi-Go-version)
- No secrets or environment-specific config

## Decisions

**Single job over two jobs (build + test)**
- `make check` already sequences fmt → vet → lint → test. Splitting into two jobs adds overhead with no isolation benefit at this repo's scale. Two-way door — can split later if parallel feedback matters.

**`go-version: "1.22"` over `"stable"`**
- Matches `go.mod`. `"stable"` could silently pick up a newer minor and surface unexpected breakage. Explicit pin is safer; update deliberately with `go.mod` bumps.

**`cache: true` on `actions/setup-go`**
- Module cache is built-in to `actions/setup-go@v5`. Zero extra config, meaningful speedup (~30s on cold run for this module size).

**`actions/checkout@v4` and `actions/setup-go@v5`**
- Current major versions. No reason to deviate.

## Risks / Trade-offs

- [Risk] Makefile dependency (`golangci-lint` must be installed in CI) → Mitigation: `make check` calls `go run github.com/golangci/golangci-lint/cmd/golangci-lint` (or similar); verify `Makefile` lint target works without a pre-installed binary before merging.
- [Risk] Go 1.22 pin becomes stale → Mitigation: update alongside `go.mod` version bumps; low-frequency event.
