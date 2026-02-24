# omnisess — AI Session Aggregator

CLI tool that aggregates AI coding sessions across Claude Code, Cursor, Codex, and Gemini. Search, list, and detect active sessions from one place.

## Tech Stack

- **Language**: Go 1.22+
- **CLI**: `github.com/spf13/cobra`
- **SQLite**: `modernc.org/sqlite` (pure Go, no CGO)
- **Output**: `github.com/olekukonez/tablewriter`

## Domain Glossary

- **Source**: A tool that produces AI coding sessions (Claude Code, Cursor, Codex, Gemini). Each implements the `Source` interface in `internal/source/source.go`.
- **Session**: A single conversation between a user and an AI coding assistant. Has an ID, tool name, project path, messages, and timestamps.
- **Message**: A single turn in a session (user, assistant, or tool role).
- **SearchResult**: A Session + matched message snippets.
- **Active Detection**: Heuristic to determine if a session is currently running (process alive AND file modified < 2 min ago).

## Repository Layout

```
main.go                     Entry point, calls cmd.Execute()
cmd/                        Cobra command definitions (no business logic)
internal/model/             Session, Message, SearchResult types
internal/source/            Source interface + per-tool implementations
internal/source/claude/     Claude Code JSONL parser
internal/source/cursor/     Cursor transcript + SQLite parser
internal/source/codex/      Codex JSONL parser (stub)
internal/source/gemini/     Gemini session parser (stub)
internal/detect/            Process detection utilities
internal/output/            Table and JSON formatters
internal/search/            Cross-source search orchestration
docs/references/            File format specs for each tool's local data
docs/exec-plans/            Implementation plans (active/ and completed/)
docs/design-docs/           Design decisions
```

## Key Invariants

1. Each source is a self-contained `Source` interface implementation in its own package
2. Source packages NEVER import each other
3. No CGO — pure Go only (`modernc.org/sqlite`, not `mattn/go-sqlite3`)
4. No indexing — brute-force scan (optimize when slow)
5. Session IDs are always displayed as `<tool>:<id>` (e.g., `claude:5c3f2742`)
6. Home directory resolved at runtime via `os.UserHomeDir()`
7. All local data is READ-ONLY — this tool never modifies source files

## Build & Test

```bash
make setup    # install git hooks (one-time)
make check    # full pre-commit: fmt + vet + lint + test
make cover    # test with per-function coverage report
```

Or manually:

```bash
go build -o omnisess .
go test ./...
./omnisess list
./omnisess search "query"
./omnisess active
./omnisess show claude:<session-id>
```

## Development Process

See [`docs/dev-harness.md`](docs/dev-harness.md) for the full development harness guide.

### Session loop

1. **Orient** -- Read this file + `docs/exec-plans/active/` to find the next task
2. **Plan** -- If the task is non-trivial, write or update an exec plan in `docs/exec-plans/active/`
3. **Branch** -- Create a branch per [`docs/process/git-workflow.md`](docs/process/git-workflow.md) (doc-only changes may go directly to main)
4. **Implement** -- Write code. Follow the invariants above. Keep source packages isolated.
5. **Verify** -- `make check` (or: `go build -o omnisess . && go vet ./... && go test ./...`)
6. **Smoke test** -- Run the relevant `omnisess` subcommand against real local data
7. **Commit & merge** -- Follow the review and merge rules in [`docs/process/git-workflow.md`](docs/process/git-workflow.md)
8. **Complete** -- Move finished exec plans to `docs/exec-plans/completed/`, update `ARCHITECTURE.md` if the codemap changed

### Adding a new Source

1. Create package `internal/source/<name>/`
2. Implement `Source` interface (see `docs/design-docs/source-interface.md`)
3. Call `source.Register()` in `init()`
4. Add blank import to `cmd/root.go`
5. Add `model.Tool<Name>` constant to `internal/model/session.go`
6. Add file format spec to `docs/references/<name>-format.md`
7. Add tests in `internal/source/<name>/parser_test.go` with `testdata/` fixtures

### Conventions

- **Tests**: Table-driven tests with `testdata/` fixtures for parser packages. See `docs/dev-harness.md`.
- **Errors**: return `nil, nil` for "not found" (empty result), `nil, error` for actual failures. Log warnings to stderr for non-fatal issues (corrupt entries, missing files).
- **Exec plans** live in `docs/exec-plans/active/` (numbered `NNN-slug.md`). Move to `completed/` when done.
- **Decision rules**: See [`docs/process/agent-review.md`](docs/process/agent-review.md) for agent-to-agent review and autonomous decision-making rules.

## Key Files

- `internal/source/source.go` -- Source interface contract
- `ARCHITECTURE.md` -- System codemap
- `docs/dev-harness.md` -- Development harness guide (tests, linting, hooks)
- `docs/references/` -- File format specs for each tool
- `docs/exec-plans/active/` -- Current implementation plans
- `Makefile` -- Build, test, lint, coverage targets
- `.golangci.yml` -- Linter configuration
