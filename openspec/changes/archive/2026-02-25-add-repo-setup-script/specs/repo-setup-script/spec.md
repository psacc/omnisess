## ADDED Requirements

### Requirement: Idempotent repo settings bootstrap
`scripts/setup_repo.sh` SHALL configure GitHub repository settings (squash-only merges, auto-merge enabled, delete-branch-on-merge) and branch protection on `main` (require PRs, linear history enforced, enforce_admins=false, no required status checks) in a single idempotent invocation.

#### Scenario: First-time run with no existing protection
- **WHEN** the script is run against a repo with no branch protection on `main`
- **THEN** protection is created with require_pull_request_reviews, required_linear_history=true, enforce_admins=false, and no required_status_checks

#### Scenario: Re-run does not weaken settings
- **WHEN** the script is run a second time without `--force`
- **THEN** existing branch protection settings are preserved and no API PATCH/PUT degrades them

#### Scenario: --force flag overwrites existing protection
- **WHEN** the script is run with `--force`
- **THEN** the full branch protection payload is applied, overwriting any existing configuration

### Requirement: Script accepts owner/repo argument with default
`scripts/setup_repo.sh` SHALL accept an optional `[owner/repo]` positional argument. When omitted, it SHALL resolve the repo from `gh repo view --json nameWithOwner`.

#### Scenario: Explicit owner/repo
- **WHEN** `./scripts/setup_repo.sh psacc/omnisess` is called
- **THEN** all API calls target the `psacc/omnisess` repo

#### Scenario: Default repo resolution
- **WHEN** the script is called with no arguments from inside a git repo
- **THEN** owner/repo is resolved via `gh repo view` and used for all API calls

### Requirement: Prerequisite validation
The script SHALL validate that `gh` is installed and authenticated before making any API calls, and exit with a non-zero status and a human-readable error if not.

#### Scenario: gh not installed
- **WHEN** the script is run and `gh` is not found in PATH
- **THEN** it exits with status 1 and prints "Error: gh CLI is required"

#### Scenario: gh not authenticated
- **WHEN** the script is run and `gh auth status` returns non-zero
- **THEN** it exits with status 1 and prints "Error: gh CLI is not authenticated"

### Requirement: Labels created idempotently
The script SHALL create labels `ai-consensus` and `human-review-required` using `gh label create --force` so that re-runs update color/description without failing.

#### Scenario: Label does not exist
- **WHEN** the label `ai-consensus` does not exist on the repo
- **THEN** it is created with the defined color and description

#### Scenario: Label already exists
- **WHEN** the label `ai-consensus` already exists
- **THEN** the script updates it (via `--force`) and exits without error

### Requirement: Makefile repo-setup target
The `Makefile` SHALL expose a `repo-setup` target that calls `scripts/setup_repo.sh` and is listed in `make help` output.

#### Scenario: make repo-setup invocation
- **WHEN** `make repo-setup` is run from the repo root
- **THEN** `scripts/setup_repo.sh` is executed with no arguments (defaulting to current repo)
