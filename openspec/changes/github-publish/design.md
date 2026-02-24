## Context

`sessions` is a private Go CLI tool with module path `github.com/psacconier/sessions`. It has no public presence, no README, no LICENSE, and no CI. The goal is to publish it as an open-source project under `github.com/psacc/omnisess` with the standard scaffolding a Go open-source project is expected to have.

The current module path appears in 61 locations across Go source files, `go.mod`, and documentation. No external consumers exist, so this is a clean cut — no deprecation notice or import alias bridge is needed.

## Goals / Non-Goals

**Goals:**
- Rename the Go module path from `github.com/psacconier/sessions` to `github.com/psacc/omnisess` across all files
- Add `README.md` covering installation, quick-start usage, supported sources, and contributing
- Add `LICENSE` (MIT)
- Add `.github/workflows/ci.yml` running build, vet, and test on push and PRs
- Add `.github/ISSUE_TEMPLATE/` with bug report and feature request templates
- Update `CLAUDE.md` and `ARCHITECTURE.md` to reflect the new module path and repo URL

**Non-Goals:**
- No changes to CLI behavior, commands, or supported sources
- No new Go packages
- No versioning strategy beyond the initial public push (no `v1.0.0` tag decisions here)
- No Go module proxy warm-up or pkg.go.dev badge automation
- No change to the binary name (`sessions` stays as the binary name)

## Decisions

### D1: Module rename via `find` + `sed`, not `go mod` tooling

**Decision**: Use `find . -type f \( -name "*.go" -o -name "*.mod" -o -name "*.md" \) | xargs sed -i '' 's|github.com/psacconier/sessions|github.com/psacc/omnisess|g'` to perform the rename in one pass.

**Why**: `go mod edit -module` only updates `go.mod`. Every import path in every `.go` file and every documentation reference must also change. A `sed` sweep touches all of them in one command. The pattern is unambiguous — the old path does not appear as a substring of any other string in the codebase.

**Alternative considered**: `gorename` or `gofmt`-based tooling. Rejected — these operate on symbol names, not import strings. They would not update `go.mod`, documentation, or non-Go files.

**Verification**: After the `sed` pass, run `grep -r "psacconier" .` to confirm zero residual occurrences.

---

### D2: README structure — install-first, code-second

**Decision**: README sections in this order:
1. One-line description + badge row (CI status)
2. **Install** (`go install github.com/psacc/omnisess@latest`)
3. **Quick start** (3-4 command examples with output)
4. **Supported sources** (table: Claude Code, Cursor, Codex, Gemini — with status notes)
5. **Commands** (brief table: `list`, `search`, `active`, `show`, `tui`)
6. **Contributing** (fork → branch → `make check` → PR)
7. **License**

**Why**: Developers scanning a new tool on GitHub decide within 10 seconds. Install command and one working example are the highest-signal content. Putting them above any architecture description maximizes conversion from visitor to user.

**Alternative considered**: Architecture-first README similar to internal `ARCHITECTURE.md`. Rejected — that document targets contributors who already know the tool. A public README targets strangers.

---

### D3: MIT License

**Decision**: MIT, no modification.

**Why**: Maximally permissive. No copyleft concerns for users who embed the tool in scripts or enterprise workflows. Consistent with the Go ecosystem default for CLI tools. No legal review required.

**Alternative considered**: Apache 2.0 for explicit patent grant. Rejected — this is a CLI tool with no novel algorithms; patent exposure is not a realistic concern, and MIT is simpler.

---

### D4: CI workflow — two jobs, no matrix

**Decision**: Single workflow file `.github/workflows/ci.yml` triggered on `push` (all branches) and `pull_request` (targeting `main`). Two jobs:

- **`build`**: `go build ./...` — fast gate, catches compilation errors
- **`test`**: `go vet ./...` + `go test -race ./...` — correctness gate

Both jobs use `ubuntu-latest` and `go-version: "stable"` (not pinned to a minor, since `go.mod` already specifies `go 1.25.0` as the minimum and GitHub Actions' `stable` tracks the current release).

**Why two jobs instead of one**: Build failures are diagnosable faster if they are isolated from test failures. The CI log is clearer for first-time contributors.

**Why no matrix (multi-Go-version)**: The tool uses no deprecated APIs and has no platform-specific code that warrants a matrix. A matrix adds CI minutes and noise without meaningful coverage benefit at this stage.

**Alternative considered**: `golangci-lint` as a third job. Deferred — the project already has `.golangci.yml` and runs lint locally via `make check`. Adding it to CI is a good follow-up once the repo is public and PRs start coming in, but it is not required for the initial publish.

**Trigger note**: `push` on all branches ensures branches are validated before merge, not just PRs from forks.

---

### D5: Issue templates — structured YAML, not plain Markdown

**Decision**: Use GitHub's YAML-based issue forms (`.github/ISSUE_TEMPLATE/*.yml`) rather than Markdown templates.

Bug report fields: `description` (textarea), `reproduction steps` (textarea), `expected vs. actual` (textarea), `OS and Go version` (input), `sessions version` (input).

Feature request fields: `problem` (textarea), `proposed solution` (textarea), `alternatives considered` (textarea).

**Why**: YAML forms render as structured web forms in the GitHub UI, reducing low-quality issues with missing reproduction info. The additional authoring cost over Markdown templates is under 30 minutes.

**Alternative considered**: Plain Markdown templates. They are simpler to write but produce free-form issues that consistently omit version and OS info, increasing triage cost.

## Risks / Trade-offs

- **`sed` sweep is irreversible without git** → Mitigation: run on a clean branch (`feat/openspec-init` already exists); commit immediately after the sweep so `git diff` confirms the exact set of changes before merge.
- **`go install @latest` depends on the module being pushed to GitHub** → The README install command will not work until the repo is public and tagged. Mitigation: note in the Contributing section that `go build -o omnisess .` is the local development path.
- **`go-version: stable` in CI may silently upgrade** → Acceptable risk. Toolchain version in `go.mod` (`go 1.25.0`) enforces the minimum; stable channel upgrades are not breaking for this codebase.
- **Binary name stays `sessions`** → First-time users running `omnisess` after `go install` will get a binary named `sessions` (from `main` package, no `// go:build` name override). This is a potential UX confusion. Mitigation: document the binary name in the README install section. Renaming the binary is a follow-up task.

## Open Questions

- Should the initial public push include a `v0.1.0` tag so `go install @latest` resolves immediately, or defer tagging to a separate release step?
- Should the CI workflow also upload a `coverage.out` artifact for visibility, or keep it minimal for now?
