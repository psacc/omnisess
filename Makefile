.PHONY: all build test cover cover-html lint vet fmt check clean setup pr install smoke tag release

# Default: build + vet + lint + test
all: build vet lint test

build:
	go build -o omnisess .

test:
	go test -race -count=1 ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

cover-html: cover
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install: https://golangci-lint.run/welcome/install/"; \
		echo "  brew install golangci-lint"; \
		exit 1; \
	}
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -w .

# Full pre-commit check: fmt + vet + lint + test
check: fmt vet lint test

# Push current branch and open a GitHub PR
# Usage: make pr
pr:
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" = "main" ]; then \
		echo "error: cannot open PR from main"; exit 1; \
	fi; \
	git push -u origin "$$branch" && \
	gh pr create --title "$$(git log -1 --format='%s')" --body-file .github/pull_request_template.md

install:
	go install .

smoke:
	@command -v omnisess >/dev/null 2>&1 || { \
		echo "omnisess not in PATH. Run: make install"; \
		echo "  Then ensure ~/go/bin is on your PATH: export PATH=\"\$$PATH:\$$HOME/go/bin\""; \
		exit 1; \
	}
	omnisess list --limit=1

tag:
	@if [ -z "$(VERSION)" ]; then \
		echo "error: VERSION is required. Usage: make tag VERSION=v0.1.0"; exit 1; \
	fi
	git tag -a "$(VERSION)" -m "Release $(VERSION)"
	git push origin "$(VERSION)"

release: tag
	@command -v gh >/dev/null 2>&1 || { \
		echo "gh CLI not found. Install: https://cli.github.com/"; \
		echo "  brew install gh"; \
		exit 1; \
	}
	gh release create "$(VERSION)" --generate-notes --title "$(VERSION)"
	@echo ""
	@echo "Release $(VERSION) published."
	@echo "Next: update .claude-plugin/plugin.json version field to match (see docs/process/release.md)"

clean:
	rm -f omnisess coverage.out coverage.html

setup:
	@echo "Installing git hooks..."
	@bash scripts/install-hooks.sh
	@echo "Done. Run 'make check' to verify your setup."
