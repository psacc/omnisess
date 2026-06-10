# Architecture

> Entry point: [`CLAUDE.md`](CLAUDE.md) — project overview, dev process, conventions.
> Design decisions: [`docs/design-docs/`](docs/design-docs/index.md) | Exec plans: [`docs/exec-plans/`](docs/exec-plans/)

## Data Flow

```
Local filesystem (~/.claude/, ~/.cursor/, ~/.codex/, ~/.copilot/
                  + darwin only: ~/Library/Application Support/Code/User/workspaceStorage/)
        │
        ▼
  internal/source/*     (per-tool parsers, implement Source interface)
        │
        ▼
  internal/model/       (unified Session, Message types)
        │
        ▼
  cmd/*                 (Cobra commands: list, search, show, active, digest, index, stats, ps, tui, skills audit, version)
        │
        ▼
  internal/output/      (table or JSON rendering)
```

## Package Map

- **cmd/root.go** — Cobra root command. Global flags: `--json`, `--tool`, `--since`, `--date`, `--limit`, `--project`, `--exclude-project` (also `OMNISESS_EXCLUDE_PROJECTS` env var). Initializes source registry.
- **cmd/list.go** — Aggregates `Source.List()` from all sources, sorts by `UpdatedAt` desc, renders table.
- **cmd/search.go** — Calls `Source.Search()` in parallel via errgroup, merges results, renders with snippets.
- **cmd/show.go** — Parses `tool:id` argument, calls `Source.Get()`, renders full conversation.
- **cmd/active.go** — Calls `Source.List()` with `Active: true` filter.
- **cmd/digest.go** — `omnisess digest`: prints sessions for a calendar day with full Q&A as Obsidian-compatible markdown. Rune-safe truncation, `$HOME` → `~` substitution. Per-session `Source.Get` full-parse fanned out under errgroup with `min(NumCPU, 16)` concurrency (#54).
- **cmd/index.go** — `omnisess index --all`: bulk-walks Claude sessions and lazy-populates the transcript SQLite cache. Flags: `--full` (capture payloads), `--rebuild` (drop existing).
- **cmd/stats.go** — `omnisess stats`: per-session detail (`--session <id>`) or window aggregate (`--window 7d`); JSON output via `--json`. Reads from the index, lazy-populates on cache miss.
- **cmd/ps.go** — `omnisess ps`: merged ancestor tree of live Claude sessions (macOS only). Renders text tree or JSON via `--json`.
- **cmd/tui.go** — Interactive terminal UI for browsing sessions.
- **cmd/version.go** — `omnisess version` / `omnisess --version`: prints the installed module version.
- **cmd/skills.go** — `omnisess skills` parent command (groups skills sub-commands).
- **cmd/skills_audit.go** — `omnisess skills audit`: classifies Claude Code skills by usage (Keep / Borderline / Archive). See `docs/skills-audit.md`.
- **internal/model/session.go** — Pure data types. No dependencies.
- **internal/source/source.go** — `Source` interface: `Name()`, `List()`, `Get()`, `Search()`.
- **internal/source/registry.go** — Global source registry. Sources self-register via `init()`.
- **internal/source/claude/** — Parses `~/.claude/history.jsonl` + session JSONL files. Per-session peek work (file lookup, mtime stat, header peek for branch+model, active probe) inside `List` fanned out under errgroup with `min(NumCPU, 16)` concurrency. `pgrep` "is claude running" probe cached once per `List` call (#54).
- **internal/source/cursor/** — Reads `ai-tracking.db` for metadata, `agent-transcripts/*.txt` for content.
- **internal/source/codex/** — Parses `~/.codex/history.jsonl` + session JSONL files.
- **internal/source/copilot/** — Parses `~/.copilot/session-state/<uuid>/{events.jsonl,vscode.metadata.json}` for GitHub Copilot CLI sessions.
- **internal/index/** — SQLite transcript cache. `Index` interface (`EnsureSession`, `QuerySession`, `QueryWindow`), OTel GenAI-aligned schema, `(mtime, size, has_full_payloads)` invalidation key, one transaction per session. Source-agnostic — converts via `SessionFromModel(*model.Session, providerName)`.
- **internal/detect/process.go** — `IsProcessRunning(name)` and `IsFileRecentlyModified(path, threshold)`.
- **internal/procsnap/** — Live-process correlation for Claude and Codex sessions (macOS only). `Enumerate()` scans `~/.claude/sessions/<PID>.json` for Claude (filters alive PIDs) and, for Codex, maps `codex` processes to the rollout JSONLs they hold open via one `lsof` call, parsing each rollout's `session_meta` first line. Walks ancestors via `ps`. Returns `ErrUnsupported` off darwin.
- **internal/output/render.go** — `RenderTable()` and `RenderJSON()` dispatched by format flag.
- **~~internal/search/search.go~~** — Planned, not yet implemented. Search currently lives in `cmd/search.go`.

## Invariants

- Source packages never import each other.
- `cmd/` never accesses filesystem directly — always through source interface.
- `internal/model/` has zero imports from other internal packages.
- All timestamps are `time.Time` in UTC internally, formatted to local time only in output.
