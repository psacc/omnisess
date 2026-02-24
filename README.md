# omnisess

Aggregate AI coding sessions across Claude Code, Cursor, Codex, and Gemini — search, list, and detect active sessions from one place.

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

Once installed, four slash commands are available:

| Command | Description | Example |
|---|---|---|
| `/omnisess:list` | List all sessions across all sources | `/omnisess:list --tool claude --limit 10` |
| `/omnisess:search` | Full-text search across sessions | `/omnisess:search "database migration"` |
| `/omnisess:active` | Show currently running sessions | `/omnisess:active` |
| `/omnisess:show` | Show full detail for a session | `/omnisess:show claude:5c3f2742` |

Each command checks for the `omnisess` binary at invocation time and prints clear install instructions if it is not found.

---

## Quick start

```
# List all sessions, most recent first
$ omnisess list
TOOL       ID              PROJECT                      MESSAGES  LAST ACTIVE
claude     5c3f2742        ~/prj/myapp                  42        2h ago
cursor     a1b2c3d4        ~/prj/api                    18        5h ago

# Search across all sources
$ omnisess search "database migration"
claude:5c3f2742  ~/prj/myapp  "...ran the database migration script..."

# Show currently active sessions
$ omnisess active
claude:5c3f2742  ~/prj/myapp  (process alive, modified 47s ago)
```

---

## Supported sources

| Source      | Status |
|-------------|--------|
| Claude Code | Full   |
| Cursor      | Full   |
| Codex       | Stub   |
| Gemini      | Stub   |

---

## Commands

| Command                       | Description                                       |
|-------------------------------|---------------------------------------------------|
| `omnisess list`               | List all sessions across all sources              |
| `omnisess search <query>`     | Full-text search across sessions                  |
| `omnisess active`             | Show sessions detected as currently running       |
| `omnisess show <tool:id>`     | Show full detail for a single session             |
| `omnisess tui`                | Interactive terminal UI for browsing sessions     |

---

## Contributing

1. Fork the repository and create a feature branch (`git checkout -b feat/my-change`).
2. Make your changes and run `make check` (fmt + vet + lint + test) — must pass clean.
3. Open a pull request against `main`.

All source packages under `internal/source/<name>/` must remain isolated (no cross-imports).
Pure Go only — no CGO.

---

## License

MIT — see [LICENSE](LICENSE).
