.PHONY: all build test cover cover-html lint vet fmt check clean setup merge

# Default: build + vet + lint + test
all: build vet lint test

build:
	go build -o sessions .

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

# Squash-merge current branch into main (keeps linear history)
# Usage: make merge
merge:
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" = "main" ]; then \
		echo "error: already on main"; exit 1; \
	fi; \
	echo "Squash-merging $$branch into main..."; \
	git checkout main && \
	git merge --squash "$$branch" && \
	git commit && \
	git branch -D "$$branch" && \
	echo "Done. $$branch squash-merged into main."

clean:
	rm -f sessions coverage.out coverage.html

setup:
	@echo "Installing git hooks..."
	@bash scripts/install-hooks.sh
	@echo "Done. Run 'make check' to verify your setup."
