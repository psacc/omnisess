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
```

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
