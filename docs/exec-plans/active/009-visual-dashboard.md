# 009 — Visual Sessions Dashboard (Arc Lifecycle + TUI Enhancement)

**Status**: Not started
**Priority**: High
**Estimated effort**: 5-6 person-weeks (MVP Core ~4.75 + Polish ~1.0)
**PRD**: `docs/design-docs/visual-dashboard-prd.md`

## Summary

Replace the current single-list TUI stub with a three-panel visual dashboard (sidebar / center / detail) that organizes sessions by lifecycle state (PINNED / ACTIVE / ARCHIVED) backed by a SQLite metadata store at `~/.config/sessions/sessions.db`. The dashboard adds tool-based color coding, an 800ms pulsing active-session indicator, age/staleness indicators, a persistent status bar, and pin/unpin keyboard actions — all fully functional without an API key, with no new `go.mod` dependencies beyond what is already present (bubbletea, lipgloss, modernc.org/sqlite).

## Problem

Engineers running multiple concurrent AI coding sessions across Claude Code, Cursor, and Codex have no visual command center: `sessions list` returns a flat time-sorted table with no lifecycle context, no tool differentiation, and no way to separate genuinely active sessions from stale ones accumulated over days. The cognitive overhead of mentally tracking which sessions are live, which matter, and which are noise compounds with concurrency — this dashboard offloads that state management to the tool.

## Design Decisions

1. **TUI-only (no Web UI)**: The existing `sessions tui` stub already uses bubbletea + lipgloss. Extending it avoids new dependencies and keeps the tool self-contained. Web UI is backlog (Wave 3 D3).

2. **SQLite at `~/.config/sessions/sessions.db`**: Lifecycle state (pinned / active / archived), TTL, and future LLM summary cache are stored in a metadata overlay. Source files remain READ-ONLY at all times. `modernc.org/sqlite` is already in `go.mod` — no new dependency.

3. **Haiku as LLM default when key is present (Wave 2 C1, out of scope for MVP)**: MVP does not make LLM calls. The center panel uses a stateless `resolvePreview()` cascade: cached summary → `session.Summary` → `session.Preview` → `session.QualifiedID()`. When `ANTHROPIC_API_KEY` is unset, a `[LLM: no key]` indicator is shown; when set but no cached summary exists, `[preview]` is shown.

## Dependencies

- **002-codex-source**: Not required for MVP. Codex sessions will appear with stub data if 002 is not yet merged; dashboard layout and lifecycle mechanics do not depend on Codex being real.
- **004-improvements**: Not required. `FormatDuration` in `internal/output/render.go` (used by B6) is already present. The `session.Summary` and `session.Preview` fields used by C4 are already in the model.
- **008-aoe-resume**: Required before Phase 6 smoke testing. The `resume` package and `tui.Model.SelectedMode()` contract must be stable, as Phase 2 builds on the existing TUI model that references them. All resume keybindings (`r`, `t`, `a`, `f`, `o`) are already handled in the TUI; this plan does not change them.

## Implementation Phases

---

### Phase 1 — Lifecycle Metadata Storage (`internal/lifecycle/`)

**Goal**: Introduce the SQLite-backed lifecycle store as a standalone, testable package. No TUI changes yet. At the end of this phase, `lifecycle.Open()` creates the DB, applies the schema, runs the TTL sweep, and exposes the `Store` API.

**Files created/modified**:
- `internal/lifecycle/store.go` — new file: `State` type, `Store` struct, `Open()`, `Close()`, `RunTTLSweep()`, `GetState()`, `SetState()`, `TouchViewed()`, `TogglePin()`, `UpsertIfMissing()`
- `internal/lifecycle/store_test.go` — new file: table-driven tests covering Open (first run creates schema + version row), TTL sweep (session older than TTL → archived), UpsertIfMissing (idempotent), TogglePin (active→pinned→active), TouchViewed (last_viewed_at updated), corrupted DB (Open returns error, caller degrades gracefully)

**DDL** (executed inside `Open()` via `db.Exec`):
```sql
CREATE TABLE IF NOT EXISTS schema_version (
    version_id INTEGER PRIMARY KEY,
    version    TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_version (version_id, version) VALUES (1, '1.0.0');

CREATE TABLE IF NOT EXISTS sessions_meta (
    session_qualified_id TEXT PRIMARY KEY,
    state                TEXT NOT NULL DEFAULT 'active'
                         CHECK (state IN ('active', 'pinned', 'archived')),
    pinned_at            DATETIME,
    last_viewed_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ttl_hours            INTEGER NOT NULL DEFAULT 12
);

CREATE TABLE IF NOT EXISTS summaries (
    session_qualified_id TEXT NOT NULL,
    model_version        TEXT NOT NULL,
    summary_text         TEXT NOT NULL,
    cached_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (session_qualified_id, model_version)
);
```

**Constraints**:
- Import only `database/sql`, `modernc.org/sqlite`, `internal/model`. Never import `internal/source/*`.
- `//go:build` tags: none needed — `modernc.org/sqlite` is pure Go.

**Verification**:
```bash
go test ./internal/lifecycle/... -v -run .
# Expected: all table-driven cases pass; no CGO errors.
sqlite3 /tmp/test-sessions.db ".tables"
# Expected: schema_version  sessions_meta  summaries
```

---

### Phase 2 — Three-Panel TUI Layout Skeleton

**Goal**: Replace the current single-list `internal/tui/model.go` with the three-panel composite model (sidebar / center / detail). The layout renders correctly with real sessions, sections show PINNED / ACTIVE / ARCHIVED headers, and `WindowSizeMsg` recomputes panel widths. No lifecycle store integration yet — all sessions placed in ACTIVE section. Existing resume keybindings remain functional.

**Files created/modified**:
- `internal/tui/model.go` — replace single-list `Model` with new composite `Model`:
  - Fields: `sections []lifecycleSection`, `sidebarWidth int`, `centerWidth int`, `detailWidth int`, `detailOpen bool`, `tickPhase bool`, `apiKeyAvailable bool`, `width int`, `height int`
  - New types: `lifecycleSection{title string, sessions []EnrichedSession, cursor int, offset int}`, `EnrichedSession{Session model.Session, LifecycleState string}`
  - `New(sessions []model.Session, toolModes map[model.Tool][]string) Model` — signature unchanged for now; all sessions bucketed as ACTIVE
  - `Init()` returns `tea.Tick(800*time.Millisecond, ...)` and `type tickMsg time.Time`
  - `Update()` handles `tea.WindowSizeMsg` (recompute widths), `tickMsg` (toggle `tickPhase`), existing key handlers (`j/k`, `Enter`, `r/t/a/f/o`, `q`)
  - `View()` assembles: `lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(...panels...), statusBar)`
  - Stub renderers: `renderSidebar()`, `renderCenter()`, `renderDetail()`, `renderStatusBar()`
  - Width rule: sidebar=30 fixed, detail=40 when `detailOpen`, center=remaining (min 10); if `width < 80` → print "Terminal too narrow (min 80 cols)" and return `tea.Quit`
- `internal/tui/theme.go` — new file:
  - `var ToolColors = map[model.Tool]lipgloss.Color{ToolClaude: "#4A9EFF", ToolCursor: "#9B59B6", ToolCodex: "#2ECC71", ToolGemini: "#F1C40F"}`
  - `func ToolStyle(t model.Tool) lipgloss.Style` — returns style with left border in tool color; falls back to `#FFFFFF` for unknown tools
- `internal/tui/model_test.go` — new file: tests for `resolvePreview()` (all cascade branches), `renderStatusBar()` width clamping, section assignment logic

**Verification**:
```bash
go build -o sessions . && ./sessions tui
# Expected: three-panel layout renders without panic; PINNED / ACTIVE / ARCHIVED headers visible in sidebar; j/k navigation works; q exits cleanly.
```

---

### Phase 3 — Tool Color Coding + Active Pulse Indicator

**Goal**: Each sidebar row renders with a tool-color left border (B2) and an 800ms pulsing `●` for sessions where `session.Active == true` (B3). Age strings appear right-aligned in each sidebar row (B6 partial — staleness dimming in Phase 4).

**Files created/modified**:
- `internal/tui/model.go` — update `renderSidebar()`:
  - Sidebar row format: `<lipgloss-left-border><pulse-1char><space><tool-6chars><space><project-14chars><space><age-4chars>`
  - Tool border: `lipgloss.NewStyle().Border(lipgloss.InnerHalfBlockBorder(), false, false, false, true).BorderForeground(ToolColors[sess.Tool])`
  - Pulse: if `sess.Active && tickPhase` → `lipgloss.NewStyle().Foreground("#00CC00").Render("●")`; if `sess.Active && !tickPhase` → `lipgloss.NewStyle().Foreground("#336633").Render("●")`; if `!sess.Active` → `" "`
  - Age: `output.FormatDuration(time.Since(sess.UpdatedAt))` right-padded to 4 chars; zero time → `"?"`
  - Selected row: `lipgloss.NewStyle().Bold(true).Reverse(true)` applied to row content (border retains its color character)
- `internal/tui/theme.go` — section header style: `lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AAAAAA"))`

**Verification**:
```bash
go build -o sessions . && ./sessions tui
# Expected: Claude sessions show blue left border; Cursor sessions show purple left border; any active session (process running, file < 2 min) shows alternating green ●; age strings (e.g. "2h", "3d") visible right-aligned.
# Verify clean exit: ./sessions tui; press q; ps aux | grep sessions | grep -v grep → empty.
```

---

### Phase 4 — Arc Lifecycle Sections + Pin/Unpin + Timer Reset

**Goal**: Wire `lifecycle.Store` into `cmd/tui.go`. Sessions are now bucketed into PINNED / ACTIVE / ARCHIVED by real DB state. TTL sweep runs at startup. `p` key toggles pin. Cursor navigation fires `TouchViewed` asynchronously (A4). Archived rows render with `lipgloss.NewStyle().Faint(true)` and stale rows (> 12h) are dimmed (B6 complete).

**Files created/modified**:
- `cmd/tui.go` — after `Source.List()`, add:
  ```go
  store, storeErr := lifecycle.Open(lifecycleDBPath())
  if storeErr != nil {
      fmt.Fprintf(os.Stderr, "warning: lifecycle store unavailable: %v\n", storeErr)
  }
  if store != nil {
      defer store.Close()
      _ = store.RunTTLSweep(all)
      for _, s := range all { _ = store.UpsertIfMissing(s.QualifiedID()) }
  }
  enriched := enrichSessions(all, store)
  ```
  - `lifecycleDBPath()` helper: `filepath.Join(home, ".config", "sessions", "sessions.db")`
  - `enrichSessions()` maps `[]model.Session` + `*lifecycle.Store` → `[]tui.EnrichedSession`; if store is nil, all sessions get `LifecycleState = "active"`
  - Update `tui.New()` call: `tui.New(enriched, toolModes, store)` (new signature)
- `internal/tui/model.go` — update `New()` to accept `[]EnrichedSession` and `*lifecycle.Store`:
  - Bucket sessions: `pinned` → `sections[0]`, `active` or `sess.Active==true` override → `sections[1]`, `archived` → `sections[2]`
  - Sort: PINNED by pinned_at desc, ACTIVE by UpdatedAt desc, ARCHIVED by last_viewed_at desc (use session UpdatedAt as proxy when last_viewed_at not available in memory)
  - Handle `"p"` key in `Update()`: call `m.store.TogglePin(qualifiedID)`, re-bucket sessions in-memory immediately
  - Handle `j/k` in `Update()`: after cursor update, `go m.store.TouchViewed(qualifiedID)` (fire-and-forget, only for active sessions)
  - Archived rows rendered with `lipgloss.NewStyle().Faint(true)` wrapper
  - Staleness dimming: `time.Since(sess.UpdatedAt) > 12*time.Hour` → `Faint(true)` on non-pinned, non-archived rows

**Verification**:
```bash
go build -o sessions . && ./sessions tui
# Expected: sessions.db created at ~/.config/sessions/sessions.db on first run.
sqlite3 ~/.config/sessions/sessions.db ".schema"
# Expected: tables schema_version, sessions_meta, summaries present.
sqlite3 ~/.config/sessions/sessions.db "SELECT version FROM schema_version;"
# Expected: 1.0.0
# Press p on an ACTIVE session → it moves to PINNED section immediately.
sqlite3 ~/.config/sessions/sessions.db "SELECT state FROM sessions_meta LIMIT 3;"
# Expected: at least one row shows 'pinned' after the action.
```

---

### Phase 5 — Status Bar + Graceful Degradation Preview

**Goal**: Implement the full-width bottom status bar (E5) and the `resolvePreview()` cascade for the center panel (C4). Status bar shows section name, session counts, and LLM enrichment mode. Center panel always shows non-empty content.

**Files created/modified**:
- `internal/tui/model.go` — implement `renderStatusBar()`:
  - Left: current section name (`PINNED` / `ACTIVE (Today)` / `ARCHIVED`) derived from which section the cursor is in
  - Center: `"N sessions (P pinned, A active, R archived)"` — truncate center if total overflows terminal width
  - Right: `"[LLM: no key]"` if `!m.apiKeyAvailable`, `"[LLM: cached]"` (green) if cached summary found (never in MVP), `"[LLM: preview]"` otherwise
  - Container: `lipgloss.NewStyle().Width(m.width).Background("#1A1A2E").Foreground("#CCCCCC")`
  - `View()` final assembly: `lipgloss.JoinVertical(lipgloss.Left, mainContent, m.renderStatusBar())`
- `internal/tui/model.go` — implement `resolvePreview()`:
  - Signature: `func resolvePreview(s EnrichedSession, apiKeySet bool) previewResult`
  - `previewResult` struct: `{ text string; source previewSource }` where `previewSource` is `sourceLLM | sourceSummary | sourcePreview | sourceID`
  - Cascade: cached summary (stub: always empty in MVP) → `s.Session.Summary` → `s.Session.Preview` truncated to 200 chars with "..." → `s.Session.QualifiedID()`
  - Center panel header indicator: `sourceLLM` → `[LLM: cached]` (dim green), `sourceSummary/sourcePreview` with key set → `[preview]` (dim gray), without key → `[LLM: no key]` (dim)
  - `m.apiKeyAvailable` set in `New()` via `os.Getenv("ANTHROPIC_API_KEY") != ""`
- `internal/tui/model_test.go` — add/expand tests: `resolvePreview` all branches (empty summary + empty preview → qualifiedID, preview > 200 chars truncated, summary non-empty used first), `renderStatusBar` count correctness, no overflow at 80 cols

**Verification**:
```bash
go build -o sessions . && ./sessions tui
# Expected: status bar visible on last line; does not wrap at 80-col terminal.
# Unset key: ANTHROPIC_API_KEY="" ./sessions tui → status bar right shows [LLM: no key].
# Center panel always shows non-empty text for every session navigated to.
```

---

### Phase 6 — Integration Smoke Test + `make check`

**Goal**: Full end-to-end verification against real local data. All 10 PRD success criteria observed. `make check` passes with zero new failures. Any linter or vet issue found is fixed in this phase.

**Files created/modified**: Only fixes — no new features.
- Fix any `golangci-lint` complaints (unused imports, missing error checks on goroutine DB writes, etc.)
- If `internal/enrichment/` stub is required by any import path introduced during implementation, create `internal/enrichment/enrichment.go` with `// Wave 2: LLM summary enrichment. See feature C1.` comment only.

**Verification** (all must pass):
```bash
make check
# Expected: fmt + vet + lint + test all green; no new failures.

go build -o sessions . && ./sessions tui
# Observe against real local data:
# 1. Three-panel layout opens without panic on >= 80 col terminal.
# 2. Claude sessions: blue left border. Cursor sessions: purple left border.
# 3. At least one active session shows pulsing ● alternating at ~800ms.
# 4. Status bar renders section name, counts, [LLM: no key] without wrapping at 80 and 120 cols.
# 5. Center panel non-empty for every session.
# 6. ~/.config/sessions/sessions.db exists with correct schema.
# 7. Session not viewed for > 12h is archived on next startup.
# 8. make check passes.
# 9. q exits cleanly: ps aux | grep sessions | grep -v grep → empty.
# 10. p pins; p again unpins.

sqlite3 ~/.config/sessions/sessions.db "SELECT version FROM schema_version WHERE version_id=1;"
# Expected: 1.0.0
```

---

## Testing Strategy

- **`internal/lifecycle/`**: Table-driven unit tests in `store_test.go` using an in-memory `file::memory:?cache=shared` SQLite path. Cover: first-run schema creation, TTL sweep boundary (exactly 12h, 11h59m, 12h01m), UpsertIfMissing idempotency, TogglePin state machine, TouchViewed timestamp update, corrupted DB open failure.
- **`internal/tui/`**: Unit tests in `model_test.go` for `resolvePreview()` (all four cascade branches), `renderStatusBar()` (count arithmetic, width clamping at 80 cols), section bucketing (active override for `session.Active=true` even when DB says archived).
- **Smoke test**: `./sessions tui` against real local data (Claude + Cursor sessions on this machine) as the final gate in Phase 6.
- **No parser regression**: `go test ./internal/source/...` must remain green throughout — lifecycle package is isolated from source packages by invariant.

## Key Invariants

1. **No CGO** — `modernc.org/sqlite` only. Never `mattn/go-sqlite3`.
2. **Source files are READ-ONLY** — `internal/lifecycle/` stores metadata separately; source files are never opened for writing.
3. **Source packages never import each other** — `internal/lifecycle/` is not imported by any `internal/source/*` package.
4. **No new `go.mod` dependencies** — bubbletea, lipgloss, and modernc.org/sqlite are already present.
5. **Home directory at runtime** — `os.UserHomeDir()` for all path resolution; no hardcoded `/Users/...` paths.
6. **Session IDs as `<tool>:<id>`** — `session.QualifiedID()` used throughout; never bare IDs.
7. **Graceful degradation** — if `lifecycle.Open()` fails, TUI continues with all sessions in ACTIVE, no panic.

## Out of Scope (Wave 2+)

| ID | Feature | Reason Deferred |
|----|---------|----------------|
| C1 | LLM-Generated Summaries (on-demand via claude-sdk-go / Haiku) | Requires new SDK dep + API key. Validate baseline dashboard value first. |
| E1 | Watch Mode (auto-refresh every N seconds) | Goroutine lifecycle risk; validate static snapshot value first. |
| D1 | Compact Single-Column Mode (< 80 cols) | Most users have >= 80 col terminals; defer until usage data available. |
| E4 | Resume Mode Quick-Select Tray | Wave 2 once B1 layout is stable. |
| A2 | Configurable TTL per Session | Validate 12h default before adding config complexity. |
| A5 | Archive Restore Action | Rare action; ship after measuring archive adoption. |
| E3 | Project Filter Sticky State | Valuable at 5+ projects; validate usage patterns first. |
| B5 | Keyboard Help Overlay | Add once all keybindings are stable. |
| B4 | Fuzzy Search Overlay | Wave 2; j/k scroll sufficient for MVP. |
| D3 | Web UI | Backlog. |
| C2 | Batch Summary Pre-Generation | Backlog. |
