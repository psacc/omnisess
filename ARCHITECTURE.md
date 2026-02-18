# Architecture

## Data Flow

```
Local filesystem (~/.claude/, ~/.cursor/, ~/.codex/, ~/.gemini/)
        │
        ▼
  internal/source/*     (per-tool parsers, implement Source interface)
        │
        ▼
  internal/model/       (unified Session, Message types)
        │
        ▼
  cmd/*                 (Cobra commands: list, search, show, active)
        │
        ▼
  internal/output/      (table or JSON rendering)
```

## Package Map

- **cmd/root.go** — Cobra root command. Global flags: `--json`, `--tool`, `--since`, `--limit`. Initializes source registry.
- **cmd/list.go** — Aggregates `Source.List()` from all sources, sorts by `UpdatedAt` desc, renders table.
- **cmd/search.go** — Calls `Source.Search()` in parallel via errgroup, merges results, renders with snippets.
- **cmd/show.go** — Parses `tool:id` argument, calls `Source.Get()`, renders full conversation.
- **cmd/active.go** — Calls `Source.List()` with `Active: true` filter.
- **internal/model/session.go** — Pure data types. No dependencies.
- **internal/source/source.go** — `Source` interface: `Name()`, `List()`, `Get()`, `Search()`.
- **internal/source/registry.go** — Global source registry. Sources self-register via `init()`.
- **internal/source/claude/** — Parses `~/.claude/history.jsonl` + session JSONL files.
- **internal/source/cursor/** — Reads `ai-tracking.db` for metadata, `agent-transcripts/*.txt` for content.
- **internal/source/codex/** — Stub. Returns empty results.
- **internal/source/gemini/** — Stub. Returns empty results.
- **internal/detect/process.go** — `IsProcessRunning(name)` and `IsFileRecentlyModified(path, threshold)`.
- **internal/output/render.go** — `RenderTable()` and `RenderJSON()` dispatched by format flag.
- **internal/search/search.go** — Orchestrates parallel search across sources, merges and ranks results.

## Invariants

- Source packages never import each other.
- `cmd/` never accesses filesystem directly — always through source interface.
- `internal/model/` has zero imports from other internal packages.
- All timestamps are `time.Time` in UTC internally, formatted to local time only in output.
