# Development Harness

Local development tooling for the `sessions` CLI: tests, linting, and pre-commit hooks.

## Quick Start

```bash
make setup    # install git pre-commit hook
make check    # full validation: fmt + vet + lint + test
```

## Make Targets

| Target         | What it does                                           |
|----------------|--------------------------------------------------------|
| `make`         | Build + vet + lint + test (default)                    |
| `make build`   | `go build -o sessions .`                               |
| `make test`    | `go test -race -count=1 ./...`                         |
| `make cover`   | Run tests with coverage, print per-function report     |
| `make cover-html` | Generate and open HTML coverage report              |
| `make lint`    | Run `golangci-lint run` (installs if missing)          |
| `make vet`     | `go vet ./...`                                         |
| `make fmt`     | `gofmt -w .`                                           |
| `make check`   | Full pre-commit: fmt + vet + lint + test               |
| `make clean`   | Remove binary and coverage files                       |
| `make setup`   | Install git hooks                                      |

## Running a Single Test or Package

```bash
# Single package
go test -v ./internal/model/...

# Single test function
go test -v -run TestShortID ./internal/model/...

# Single package with coverage
go test -coverprofile=coverage.out -covermode=atomic ./internal/source/claude/...
go tool cover -func=coverage.out
```

## Coverage for One Package

```bash
go test -coverprofile=coverage.out ./internal/output/...
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

## Conventions

### Table-driven tests

All tests use table-driven style with named subtests:

```go
tests := []struct {
    name  string
    input string
    want  string
}{
    {name: "empty", input: "", want: ""},
    {name: "normal", input: "hello", want: "hello"},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got := myFunc(tt.input)
        if got != tt.want {
            t.Errorf("myFunc(%q) = %q, want %q", tt.input, got, tt.want)
        }
    })
}
```

### Testdata fixtures

Parser tests use JSONL fixtures in `testdata/` directories co-located with the test file.
See `internal/source/claude/testdata/` for examples.

### Golden files

For complex output validation, compare against golden files:
1. Store expected output in `testdata/golden/<name>.txt`
2. Use `os.ReadFile` to load and compare
3. Pass `-update` flag to regenerate: `go test -run TestFoo -update`

(Not yet implemented -- use string matching for now.)

### TUI rendering tests

TUI features require boundary-aware testing beyond structural checks:

1. **Width budget test**: Assert that rendered row width <= `m.width` at 80 and 120 columns
2. **Empty field tests**: Every displayed field must be tested with `""` — previews, projects, tools
3. **External CLI integration**: When exec'ing external tools, test CWD/env assumptions (e.g., `claude --resume` scopes to CWD)

See [`docs/process/tui-quality-gates.md`](process/tui-quality-gates.md) for full post-mortem.

### Adding tests for a new source

1. Create `internal/source/<name>/testdata/` with fixture files
2. Create `internal/source/<name>/parser_test.go` with table-driven tests
3. Test pure parsing functions directly (avoid filesystem dependencies)
4. Run `make cover` to verify coverage meets target
