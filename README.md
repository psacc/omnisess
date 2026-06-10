# omnisess

Aggregate AI coding sessions across Claude Code, Cursor, Codex, and GitHub Copilot CLI — search, list, and detect active sessions from one place.

[![CI](https://github.com/psacc/omnisess/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/psacc/omnisess/actions/workflows/ci.yml)

---

## Install

```bash
go install github.com/psacc/omnisess@latest
```

The binary is named `omnisess`. Or build from source:

```bash
git clone https://github.com/psacc/omnisess.git
cd omnisess
go build -o omnisess .
make install  # installs binary to ~/go/bin/omnisess
make smoke    # verifies the installed binary is reachable and functional
```

---

## Install as Claude Code Plugin

Use omnisess directly from Claude Code via slash commands — no context switching needed.

### Prerequisites

The `omnisess` binary must be installed and in your PATH before installing the plugin:

```bash
go install github.com/psacc/omnisess@latest
```

### Install the plugin

In Claude Code, run:

```
/plugin marketplace add psacc/omnisess
/plugin install omnisess@psacc
```

> The marketplace identifier is `omnisess@psacc` — `omnisess` is the plugin name, `psacc` is the marketplace source added in the first command.

### Usage

Once installed, ten slash commands are available:

| Command | Description | Example |
|---|---|---|
| `/omnisess:list` | List all sessions across all sources | `/omnisess:list --tool claude --limit 10` |
| `/omnisess:search` | Full-text search across sessions | `/omnisess:search "database migration"` |
| `/omnisess:active` | Show currently running sessions | `/omnisess:active` |
| `/omnisess:show` | Show full detail for a session | `/omnisess:show claude:5c3f2742` |
| `/omnisess:recap` | Structured markdown briefing of sessions for a time window | `/omnisess:recap today` |
| `/omnisess:digest` | Print a calendar day's sessions as Obsidian-compatible markdown with full Q&A | `/omnisess:digest --date 2026-05-09` |
| `/omnisess:stats` | Tool-call counts and file-I/O activity from the transcript index | `/omnisess:stats --window 7d` |
| `/omnisess:index` | Bulk-populate the transcript SQLite cache used by `stats` | `/omnisess:index --all` |
| `/omnisess:ps` | Live Claude + Codex process tree with ancestor lineage (macOS) | `/omnisess:ps` |
| `/omnisess:skills-audit` | Classify Claude Code skills by usage (Keep / Borderline / Archive) | `/omnisess:skills-audit --window 90d` |

Each command checks for the `omnisess` binary at invocation time and prints clear install instructions if it is not found.

---

## Quick start

```
# List all sessions, most recent first
$ omnisess list
TOOL       ID              PROJECT                      MESSAGES  LAST ACTIVE
claude     5c3f2742        ~/prj/myapp                  42        2h ago
cursor     a1b2c3d4        ~/prj/api                    18        5h ago

# Sessions from a specific calendar day (local time)
$ omnisess list --date 2026-04-22

# Search across all sources
$ omnisess search "database migration"
claude:5c3f2742  ~/prj/myapp  "...ran the database migration script..."

# Show currently active sessions
$ omnisess active
claude:5c3f2742  ~/prj/myapp  (process alive, modified 47s ago)
```

---

## Supported sources

| Source              | Status |
|---------------------|--------|
| Claude Code         | Full   |
| Cursor              | Full   |
| Codex               | Full   |
| GitHub Copilot CLI  | Full   |

---

## Commands

| Command                       | Description                                       |
|-------------------------------|---------------------------------------------------|
| `omnisess list`               | List all sessions across all sources              |
| `omnisess search <query>`     | Full-text search across sessions                  |
| `omnisess active`             | Show sessions detected as currently running       |
| `omnisess show <tool:id>`     | Show full detail for a single session             |
| `omnisess digest`             | Print a calendar day's sessions as Obsidian-compatible markdown |
| `omnisess index --all`        | Bulk-populate the transcript SQLite index (Claude only in v1) |
| `omnisess stats`              | Tool counts + file activity stats from the transcript index |
| `omnisess ps`                 | Live Claude + Codex process tree with ancestor lineage (macOS) |
| `omnisess tui`                | Interactive terminal UI for browsing sessions     |
| `omnisess version`            | Print the installed omnisess version              |
| `omnisess skills audit`       | Classify skills by usage (Keep/Borderline/Archive); see [docs/skills-audit.md](docs/skills-audit.md) |

### Global flags

These flags are accepted by every subcommand:

| Flag | Description |
|---|---|
| `--json` | Output as JSON instead of a table |
| `--tool <name>` | Filter by source (`claude`, `cursor`, `codex`, `copilot`) |
| `--since <duration>` | Only sessions updated within duration (Go duration, plus `Nd` for days, `Nw` for weeks) |
| `--date YYYY-MM-DD` | Only sessions updated on this calendar day (local time). Combines with `--since` by intersection |
| `--limit N` | Max results (`0` = unlimited) |
| `--project <substring>` | Filter by project path substring |
| `--exclude-project <substring>` | Exclude project path substring (repeatable; also via `OMNISESS_EXCLUDE_PROJECTS` env var, comma-separated) |

---

## Stats (transcript index)

`omnisess` maintains a derived SQLite cache of per-tool-call metadata
(`Read`/`Write`/`Edit` file paths, line counts, token usage, error flags).
The cache is OpenTelemetry GenAI-aligned: column names map 1:1 to OTel
attributes (`tool_call_id`, `provider_name`, `request_model`, …).

```bash
# Bulk-populate the cache once (also runs lazily on first stats call):
omnisess index --all

# Tool counts + file activity for the last 7 days (default window):
omnisess stats

# Aggregate over a different window:
omnisess stats --window 30d

# Per-session detail:
omnisess stats --session claude:5c3f2742

# Machine-readable:
omnisess stats --window 7d --json | jq .

# Capture full arguments + result payloads (privacy-sensitive — Bash commands
# and Write/Edit content may contain secrets):
omnisess index --all --full
omnisess stats --session claude:5c3f2742 --full
```

Even without `--full`, the index records file paths touched by Read/Write/Edit
tool calls (and your home directory often appears in them) — treat the cache
file as user-private data.

The index file lives at `os.UserCacheDir() + /omnisess/index.sqlite` by
default; override via `OMNISESS_INDEX_PATH`.

**Caveat for v0.7+:** the index is currently used only by `omnisess stats`.
`list`/`active`/`digest`/`search` still re-parse JSONL on every invocation;
PR2 will route them through the index (issue #54).

---

## Releases

Versioned releases are published to the [GitHub releases page](https://github.com/psacc/omnisess/releases).

Install a specific version:

```bash
go install github.com/psacc/omnisess@v0.4.1
```

Install the latest tagged release:

```bash
go install github.com/psacc/omnisess@latest
```

Verify the installed version:

```bash
omnisess --version
```

Binaries installed via `go install github.com/psacc/omnisess@vX.Y.Z` embed the
module version automatically. Binaries built locally with `go build` report
`(devel)`.

See [`docs/process/release.md`](docs/process/release.md) for the release process.

---

## Contributing

1. Fork the repository and create a feature branch (`git checkout -b feat/my-change`).
2. Run `make setup` (one-time) — installs git hooks and golangci-lint at the pinned version. Or run `make install-lint` to install/upgrade the linter alone.
3. Make your changes and run `make check` (fmt + vet + lint + test) — must pass clean.
4. Open a pull request against `main`.

All source packages under `internal/source/<name>/` must remain isolated (no cross-imports).
Pure Go only — no CGO.

---

## License

MIT — see [LICENSE](LICENSE).
