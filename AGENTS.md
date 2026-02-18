# sessions — AI Session Aggregator

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
go build -o sessions .
go test ./...
./sessions list
./sessions search "query"
./sessions active
./sessions show claude:<session-id>
```

## Key Files

- `internal/source/source.go` — Source interface contract
- `ARCHITECTURE.md` — System codemap
- `docs/references/` — File format specs for each tool
- `docs/exec-plans/active/` — Current implementation plans
