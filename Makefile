.PHONY: all build test test-integ cover cover-check cover-html lint vet fmt check clean setup pr install smoke tag release repo-setup help

# Default: build + vet + lint + test
all: build vet lint test

build: ## Build the omnisess binary
	go build -o omnisess .

test: ## Run unit tests only (skips integration tests that read real local data)
	go test -race -count=1 -short ./...

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

cover-html: cover ## Run tests and open HTML coverage report
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

cover-check: ## Enforce 100% per-package coverage (exempt: gemini, main)
	go test -coverprofile=coverage.out ./...
	go run ./tools/covercheck -threshold 100 -exempt "gemini,github.com/psacc/omnisess,tools/covercheck" coverage.out

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install: https://golangci-lint.run/welcome/install/"; \
		echo "  brew install golangci-lint"; \
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

release: tag ## Create a GitHub release (usage: make release VERSION=v1.2.3)
	@command -v gh >/dev/null 2>&1 || { \
		echo "gh CLI not found. Install: https://cli.github.com/"; \
		echo "  brew install gh"; \
		exit 1; \
	}
	gh release create "$(VERSION)" --generate-notes --title "$(VERSION)"
	@echo ""
	@echo "Release $(VERSION) published."

clean: ## Remove build artifacts
	rm -f omnisess coverage.out coverage.html

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

setup: ## Install git hooks (one-time setup)
	@echo "Installing git hooks..."
	@bash scripts/install-hooks.sh
	@echo "Done. Run 'make check' to verify your setup."

repo-setup: ## Apply GitHub repo settings + branch protection (idempotent; use FORCE=1 to overwrite stricter settings)
	@if [ "$(FORCE)" = "1" ]; then \
		./scripts/setup_repo.sh --force; \
	else \
		./scripts/setup_repo.sh; \
	fi
