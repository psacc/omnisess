## 1. Module Rename

- [ ] 1.1 Create a clean branch for the rename (e.g., `feat/github-publish`) and verify working tree is clean before proceeding
- [ ] 1.2 Run the `sed` sweep to replace all occurrences of the old module path: `find . -not -path './.git/*' -type f \( -name "*.go" -o -name "*.mod" -o -name "*.md" -o -name "*.yml" -o -name "*.yaml" \) | xargs sed -i '' 's|github.com/psacconier/sessions|github.com/psacc/omnisess|g'`
- [ ] 1.3 Verify zero residual occurrences: `grep -r "psacconier" . --exclude-dir=.git` must return no output
- [ ] 1.4 Update `Makefile` build target: change `go build -o sessions .` to `go build -o omnisess .` and update the `clean` target to remove `omnisess` instead of `sessions`
- [ ] 1.5 Update `CLAUDE.md`: replace all references to `sessions` binary with `omnisess`, and confirm no `github.com/psacconier/sessions` string remains
- [ ] 1.6 Verify `go.mod` declares `module github.com/psacc/omnisess` and contains no trace of the old path
- [ ] 1.7 Run `go build ./...` — must exit 0
- [ ] 1.8 Run `go test -race ./...` — must exit 0
- [ ] 1.9 Run `grep -r "psacconier" docs/` — must return no output

## 2. LICENSE

- [ ] 2.1 Create `LICENSE` at the repo root with standard MIT License text, copyright year 2026, copyright holder `psacc`
- [ ] 2.2 Verify the file contains the phrase "MIT License" and the standard permission notice ("Permission is hereby granted, free of charge...")
- [ ] 2.3 Verify `go.mod` contains no license-related directive (run `grep -i "license" go.mod` — must return no output)

## 3. CI Workflow

- [ ] 3.1 Create directory `.github/workflows/` if it does not exist
- [ ] 3.2 Create `.github/workflows/ci.yml` with `on:` block triggering on `push` (all branches) and `pull_request` targeting `main` only
- [ ] 3.3 Add `build` job: `runs-on: ubuntu-latest`, `actions/checkout@v4`, `actions/setup-go@v5` with `go-version: "stable"`, then `go build ./...`
- [ ] 3.4 Add `test` job: same runner and setup steps, then `go vet ./...` followed by `go test -race ./...`
- [ ] 3.5 Verify no `matrix:` key appears anywhere in the workflow file: `grep "matrix:" .github/workflows/ci.yml` must return no output
- [ ] 3.6 Validate YAML syntax locally: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` must exit 0

## 4. GitHub Issue Templates

- [ ] 4.1 Create directory `.github/ISSUE_TEMPLATE/`
- [ ] 4.2 Create `.github/ISSUE_TEMPLATE/bug_report.yml` as a YAML issue form with top-level keys `name:`, `description:`, `labels: [bug]`, and `body:` containing five fields: description (textarea), steps to reproduce (textarea), expected vs. actual behavior (textarea), OS and Go version (input), `omnisess` version (input)
- [ ] 4.3 Create `.github/ISSUE_TEMPLATE/feature_request.yml` as a YAML issue form with top-level keys `name:`, `description:`, `labels: [enhancement]`, and `body:` containing three fields: problem statement (textarea), proposed solution (textarea), alternatives considered (textarea)
- [ ] 4.4 Create `.github/ISSUE_TEMPLATE/config.yml` with `blank_issues_enabled: false`
- [ ] 4.5 Verify all three template files contain valid YAML: `python3 -c "import yaml, glob; [yaml.safe_load(open(f)) for f in glob.glob('.github/ISSUE_TEMPLATE/*.yml')]"` must exit 0
- [ ] 4.6 Verify `bug_report.yml` contains `name:`, `description:`, `body:`, and `labels:` with value `bug`: `grep -E "^(name|description|body|labels):" .github/ISSUE_TEMPLATE/bug_report.yml`
- [ ] 4.7 Verify `feature_request.yml` contains `name:`, `description:`, `body:`, and `labels:` with value `enhancement`: `grep -E "^(name|description|body|labels):" .github/ISSUE_TEMPLATE/feature_request.yml`

## 5. Public README

- [ ] 5.1 Create `README.md` at the repo root — this step comes after the module rename so all references use `github.com/psacc/omnisess` and `omnisess` binary name
- [ ] 5.2 Add header: project name `omnisess`, one-sentence description, and CI badge linking to `.github/workflows/ci.yml` on `main` (badge URL format: `https://github.com/psacc/omnisess/actions/workflows/ci.yml/badge.svg?branch=main`)
- [ ] 5.3 Add **Install** section containing `go install github.com/psacc/omnisess@latest`, a note that the binary is named `omnisess`, and the local alternative `go build -o omnisess .`
- [ ] 5.4 Add **Quick start** section with runnable examples and representative output blocks for `omnisess list`, `omnisess search "<query>"`, and `omnisess active`
- [ ] 5.5 Add **Supported sources** section with a Markdown table listing Claude Code, Cursor, Codex, and Gemini with their current implementation status (full / stub)
- [ ] 5.6 Add **Commands** section with a table covering all five subcommands: `list`, `search`, `active`, `show`, `tui` — each with a one-line description
- [ ] 5.7 Add **Contributing** section documenting the workflow: fork → branch → `make check` → open PR
- [ ] 5.8 Add **License** section stating MIT License and linking to the `LICENSE` file
- [ ] 5.9 Verify `README.md` contains `go install github.com/psacc/omnisess@latest`: `grep "go install github.com/psacc/omnisess@latest" README.md`
- [ ] 5.10 Verify `README.md` does not contain any old module path: `grep "psacconier" README.md` must return no output

## 6. Final Verification

- [ ] 6.1 Run full `make check` (fmt + vet + lint + test) — must exit 0
- [ ] 6.2 Run `grep -r "psacconier" . --exclude-dir=.git` — must return no output (repo-wide final check)
- [ ] 6.3 Run `make build` and confirm binary named `omnisess` is created: `ls -la omnisess`
- [ ] 6.4 Run `make clean` and confirm `omnisess` is removed: `ls omnisess` must return "No such file or directory"
- [ ] 6.5 Smoke test the binary: `./omnisess list` and `./omnisess active` must run without panics or import errors
