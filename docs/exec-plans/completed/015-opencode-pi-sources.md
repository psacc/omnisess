# 015 — OpenCode and pi sources

## Goal

Add first-class `opencode` and `pi` sources to `omnisess list`, `show`,
`search`, `active`, and `tui`, following the existing per-source contract
(`docs/design-docs/source-interface.md`).

## Findings

- **OpenCode** (v2.x) persists every session in a single SQLite database at
  `$XDG_DATA_HOME/opencode/opencode.db` (default `~/.local/share/opencode`).
  Current releases write `session_v2` + `session_message` with message parts
  inlined in the row `data` JSON. Upgraded databases keep the pre-v2
  `session`/`message`/`part` tables; `session_v2` is a superset, so legacy
  tables are only read when `session_v2` is absent.
- **pi** (v1.x) writes one append-only JSONL per session under
  `~/.pi/agent/sessions/<encoded-cwd>/`, with a `{"type":"session"}` header.
  Tool results are separate `toolResult` messages keyed by `toolCallId` and are
  attached to the originating assistant tool call. Session names live in
  `session_info` entries.

## Implementation

- `internal/source/opencode/` — read-only SQLite access (`?mode=ro`), v2 and
  legacy parsers, `List`/`Get`/`Search`, unique-prefix ID resolution.
- `internal/source/pi/` — bounded `List` peek (200 lines/file), full
  `Get`/`Search` parser, unique-prefix ID resolution.
- `model.ToolOpenCode` / `model.ToolPi`; blank imports in `cmd/root.go`;
  `detect.IsToolRunning` cases (`opencode`, pi's node/bun command line).
- Format specs: `docs/references/opencode-format.md`,
  `docs/references/pi-format.md`.
- Tests build OpenCode fixtures from SQL scripts (`testdata/*.sql`) and copy
  the pi fixture tree into a temp HOME; both packages hold 100% statement
  coverage.

## Verification

- `make check`, `make cover-check`
- Smoke: `omnisess list --tool opencode`, `omnisess show opencode:<id>`,
  `omnisess search <term> --tool pi` against real local data.
