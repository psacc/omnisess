.PHONY: all build test test-integ cover cover-check cover-html lint vet fmt check clean setup install-lint pr install smoke tag release bump-skills repo-setup help

GOLANGCI_LINT_VERSION ?= v2.12.1

# Default: build + vet + lint + test
all: build vet lint test

build: ## Build the omnisess binary
	go build -o omnisess .

# Per-test time budget. Any test slower than this fails the build.
# Override with: make test TEST_SLOW_THRESHOLD=5
TEST_SLOW_THRESHOLD ?= 1.0

test: ## Run unit tests; fails if any test exceeds TEST_SLOW_THRESHOLD seconds
	@go test -race -count=1 -short -v ./... 2>&1 | tee .test-output.log
	@awk -v thr=$(TEST_SLOW_THRESHOLD) ' \
	  /^--- (PASS|FAIL):/ { \
	    t=$$3; gsub(/[()s]/,"",t); \
	    if (t+0 > thr) { printf "SLOW (%.2fs > %.1fs): %s\n", t, thr, $$2; slow++ } \
	  } \
	  END { if (slow) { printf "\n%d test(s) exceeded %.1fs threshold\n", slow, thr; exit 1 } }' \
	  .test-output.log
	@rm -f .test-output.log

# test-integ runs all tests including integration tests that read real local
# data from ~/.claude, ~/.cursor, ~/.codex, etc. Only run on a developer machine
# with sessions present. Not suitable for CI.
test-integ: ## Run all tests including integration tests (requires real local data)
	go test -race -count=1 ./...

cover: ## Run tests with per-function coverage report
	go test -short -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

cover-check: ## Enforce 100% per-package statement coverage (skips integration tests)
	go test -short -coverprofile=coverage.out ./...
	go run ./tools/covercheck -threshold 100 -exempt "gemini,github.com/psacc/omnisess,tools/covercheck" coverage.out

cover-pkg: ## Fast per-package coverage (usage: make cover-pkg PKG=./cmd/...)
	## Example: make cover-pkg PKG=./internal/source/cursor/...
	go test -short -coverprofile=coverage.out $${PKG} && go tool cover -func=coverage.out | tail -1

cover-html: cover ## Run tests and open HTML coverage report
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

install-lint: ## Install golangci-lint at the pinned version (one-time bootstrap)
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh \
		| sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	@echo "Installed: $$(golangci-lint version)"

lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Run: make install-lint"; \
		exit 1; \
	}
	golangci-lint run

vet: ## Run go vet
	go vet ./...

fmt: ## Run gofmt
	gofmt -w .

# Full pre-commit check: fmt + vet + lint + test
check: fmt vet lint test ## Full pre-commit check: fmt + vet + lint + test

# Push current branch and open a GitHub PR
# Usage: make pr
pr: ## Push current branch and open a GitHub PR with template
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" = "main" ]; then \
		echo "error: cannot open PR from main"; exit 1; \
	fi; \
	git push -u origin "$$branch" && \
	gh pr create --title "$$(git log -1 --format='%s')" --body-file .github/pull_request_template.md

install: ## Install omnisess to ~/go/bin
	go install .

smoke: ## Smoke test: verify omnisess binary is installed and responsive
	@command -v omnisess >/dev/null 2>&1 || { \
		echo "omnisess not in PATH. Run: make install"; \
		echo "  Then ensure ~/go/bin is on your PATH: export PATH=\"\$$PATH:\$$HOME/go/bin\""; \
		exit 1; \
	}
	omnisess list --limit=1

tag: ## Create and push a git tag (usage: make tag VERSION=v1.2.3)
	@if [ -z "$(VERSION)" ]; then \
		echo "error: VERSION is required. Usage: make tag VERSION=v0.1.0"; exit 1; \
	fi
	git tag -a "$(VERSION)" -m "Release $(VERSION)"
	git push origin "$(VERSION)"

bump-skills: ## Rewrite metadata.version in every SKILL.md (usage: make bump-skills VERSION=v1.2.3)
	@if [ -z "$(VERSION)" ]; then \
		echo "error: VERSION is required. Usage: make bump-skills VERSION=v0.4.1"; exit 1; \
	fi
	@stripped=$$(echo "$(VERSION)" | sed 's/^v//'); \
	for f in SKILL.md skills/*/SKILL.md; do \
		[ -f "$$f" ] || continue; \
		sed -i.bak -E "s/^(  version: ).*/\1$${stripped}/" "$$f" && rm -f "$$f.bak"; \
	done
	@echo "Bumped SKILL.md files to $(VERSION)"

release: ## Tag current HEAD and create GitHub release (usage: make release VERSION=v1.2.3)
	@if [ -z "$(VERSION)" ]; then \
		echo "error: VERSION is required. Usage: make release VERSION=v0.4.1"; exit 1; \
	fi
	@command -v gh >/dev/null 2>&1 || { \
		echo "gh CLI not found. Install: https://cli.github.com/"; \
		echo "  brew install gh"; \
		exit 1; \
	}
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" != "main" ]; then \
		echo "error: release must run from main (currently on $$branch)"; exit 1; \
	fi
	@# Guard: SKILL.md must already be bumped to VERSION via a preceding PR.
	@# Branch protection requires every change on main to go through a PR + CI,
	@# so the SKILL.md bump cannot happen at release time. Run `make bump-skills
	@# VERSION=$(VERSION)` on a release-prep branch, PR it, and merge before releasing.
	@stripped=$$(echo "$(VERSION)" | sed 's/^v//'); \
	actual=$$(awk '/^  version: / {print $$2; exit}' SKILL.md); \
	if [ "$$actual" != "$$stripped" ]; then \
		echo "error: SKILL.md version is $$actual, expected $$stripped."; \
		echo "  Run 'make bump-skills VERSION=$(VERSION)' on a release-prep branch,"; \
		echo "  open a PR, and merge it before releasing."; \
		exit 1; \
	fi
	git tag -a "$(VERSION)" -m "Release $(VERSION)"
	git push origin "$(VERSION)"
	gh release create "$(VERSION)" --generate-notes --title "$(VERSION)"
	@echo ""
	@echo "Release $(VERSION) published."

clean: ## Remove build artifacts
	rm -f omnisess coverage.out coverage.html

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

setup: ## Install git hooks and golangci-lint (one-time setup)
	@echo "Installing git hooks..."
	@bash scripts/install-hooks.sh
	@$(MAKE) install-lint
	@echo "Done. Run 'make check' to verify your setup."

repo-setup: ## Apply GitHub repo settings + branch protection (idempotent; use FORCE=1 to overwrite stricter settings)
	@if [ "$(FORCE)" = "1" ]; then \
		./scripts/setup_repo.sh --force; \
	else \
		./scripts/setup_repo.sh; \
	fi
