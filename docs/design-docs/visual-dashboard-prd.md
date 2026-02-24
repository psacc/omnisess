# PRD: Visual Sessions Dashboard — Arc-Inspired Lifecycle + TUI

**Version**: 1.0
**Date**: 2026-02-20
**Status**: Draft — ready for implementation
**Owner**: Paolo Sacconier

---

## 1. Problem Statement

### The Pain

Staff and senior engineers running multiple concurrent AI coding sessions across Claude Code, Cursor, and Codex face a persistent orienta­tion problem: **there is no visual command center**.

Today, the workflow looks like this:

- Run `sessions list` to see a flat, time-sorted table of sessions — no lifecycle context, no visual differentiation by tool.
- Run `sessions active` separately to find what is live.
- Manually scan 20–50 session rows to find the one that matters.
- Sessions from days-old work pollute the list alongside genuinely active ones with no way to separate them.
- There is no way to "pin" a high-priority session so it stays visible.
- There is no way to know at a glance that a session was last touched 3 days ago without reading the timestamp.

The cognitive overhead compounds with concurrency: an engineer running a Claude Code refactor in one terminal, a Cursor agent in another, and occasionally checking a Codex session ends up managing mental state about what's live that should be handled by the tool.

### What Becomes Possible With This Feature

- Open `sessions tui` and immediately see: which sessions are pinned (always visible), which are active today, which are archived (older than 12h).
- Spot the currently-running Claude Code session via a pulsing green bullet — without running a separate command.
- Differentiate Claude (blue border), Cursor (purple), Codex (green), Gemini (yellow) by color without reading the tool column.
- See session age at a glance ("2h", "3d") with visual dimming for stale entries.
- Know the filter state, session count, and enrichment mode from the status bar at the bottom without any extra navigation.

---

## 2. Goal and Non-Goals

### Goal

Extend `sessions tui` into a three-panel visual dashboard that gives instant spatial overview of AI coding sessions across all tools, using lifecycle state (pinned/active/archived), tool-based color coding, active session pulse indicators, and a persistent status bar — fully functional without any API key.

### Non-Goals (MVP does NOT build)

- **LLM-generated summaries on-demand (C1)**: Wave 2. MVP shows `session.Summary` or `session.Preview` as the center-panel content.
- **Fuzzy search overlay (B4)**: Wave 2. Navigation is by j/k scroll in MVP.
- **Watch mode / auto-refresh (E1)**: Wave 2. Sessions are loaded at startup; user must reopen TUI to see new sessions.
- **Compact single-column mode for narrow terminals (D1)**: Wave 2.
- **Project filter sticky state (E3)**: Wave 2.
- **Resume mode quick-select tray (E4)**: Wave 2.
- **Configurable TTL per session (A2)**: Wave 2. Default 12h only in MVP.
- **Archive restore action (A5)**: Wave 2.
- **Keyboard navigation help overlay (B5)**: Wave 2.
- **Web UI (D3)**: Backlog.
- **Batch summary pre-generation (C2)**: Backlog.
- **Theme/topic LLM grouping (C3)**: Wave 3.
- **Shell alias integration (E2)**: Backlog.
- **JSON export of lifecycle state (D4)**: Backlog.
- **Any writes to source files**: never — all source data is read-only.
- **New go.mod dependencies**: MVP uses only existing deps (bubbletea, lipgloss, modernc.org/sqlite).

---

## 3. User Stories

### US-1: Spatial Overview at a Glance (maps to B1, B2, E5)
As a staff engineer juggling three concurrent AI coding sessions, I want to open `sessions tui` and see a sidebar with sessions organized by lifecycle state and colored by tool, so that I can orient myself in under 5 seconds without running separate commands.

### US-2: Active Session Detection Without a Separate Command (maps to B3)
As an engineer running a long Claude Code agent task, I want the dashboard to show a pulsing indicator on the session that is actively running, so that I can confirm the agent is still alive without switching terminal panes.

### US-3: Pinned Sessions That Never Auto-Archive (maps to A1, A3)
As an engineer with a critical multi-week refactor in progress, I want to pin that session so it stays permanently visible at the top of the dashboard, so that it never disappears due to inactivity even if I don't touch it for days.

### US-4: Stale Sessions Disappear Automatically (maps to A1, A4, B6)
As an engineer with 50+ sessions accumulated over two weeks, I want sessions I haven't viewed for 12 hours to be automatically moved to the archived section, so that the Active section contains only sessions I am genuinely working on right now.

### US-5: Functional Without an API Key (maps to C4)
As an engineer evaluating the tool or working in an air-gapped environment, I want the dashboard to show meaningful session content (first user message or session summary) even without `ANTHROPIC_API_KEY` set, so that the tool is immediately useful without any configuration.

---

## 4. Feature Specifications

### F-B1: Three-Panel TUI Layout (Sidebar + Main + Detail)

**ID**: B1
**Priority**: MVP Core
**Size**: L (~2 person-weeks for the core structural change)

**Description**: Replace the current single-list TUI (`internal/tui/model.go`) with a three-panel layout using `lipgloss.JoinHorizontal`. Left sidebar (session list, ~30 columns), center panel (session preview/summary, fluid width), detail panel (toggled by Enter, shows full conversation context).

**Exact Behavior**:
- On `sessions tui` startup, the TUI renders with sidebar visible and center panel showing the selected session's preview content.
- The detail panel is collapsed by default. Pressing `Enter` on a selected session toggles it open/closed.
- On `tea.WindowSizeMsg`, recompute panel widths: sidebar = 30 cols fixed, detail panel = 40 cols fixed when open, center panel = remaining width.
- When terminal width < 80 columns, sidebar only mode (no center or detail panel) — logged as a TODO stub for Wave 2 D1 compact mode. For MVP, display a message "Terminal too narrow (min 80 cols)" if width < 80 and quit cleanly.
- The sidebar shows three labeled sections: `PINNED`, `ACTIVE (Today)`, `ARCHIVED`. Each section header is rendered as a bold lipgloss label. Empty sections still render the header but show "(none)" placeholder.
- Session list in the sidebar scrolls independently via j/k. The cursor position in the sidebar drives what is shown in the center and detail panels.

**Edge Cases**:
- No sessions at all: display "No sessions found." and exit cleanly (existing behavior preserved).
- All sessions archived: PINNED and ACTIVE sections show "(none)", ARCHIVED section shows all sessions.
- Terminal resize during use: `WindowSizeMsg` recomputes layout without losing cursor position.
- 100+ sessions: sidebar scrolls with viewport clamping. Only visible rows are rendered.

**Acceptance Criteria**:
1. `sessions tui` opens without panicking or printing error output on terminals >= 80 columns wide.
2. The sidebar contains exactly three section headers: `PINNED`, `ACTIVE (Today)`, `ARCHIVED`.
3. Pressing `Enter` on a session toggles the detail panel; pressing `Enter` again collapses it.
4. Resizing the terminal window recomputes panel widths without crashing.
5. With 0 sessions, the TUI prints "No sessions found." to stderr and returns exit code 0.

**Implementation Notes**:
- File to modify: `internal/tui/model.go` — replace existing single-list `Model` struct with a new composite `Model` that holds `sidebarWidth`, `centerWidth`, `detailWidth`, `detailOpen bool`, `sections []lifecycleSection`.
- New type `lifecycleSection` with fields `title string`, `sessions []enrichedSession`, `cursor int`, `offset int`.
- Use `lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, centerView, detailView)` for layout assembly.
- `tea.WindowSizeMsg` handler: `m.sidebarWidth = 30; m.detailWidth = 40 if m.detailOpen else 0; m.centerWidth = msg.Width - m.sidebarWidth - m.detailWidth`.
- File to call from: `cmd/tui.go` — `tui.New()` signature changes to accept lifecycle-enriched sessions once A1 is implemented.

---

### F-A1: Session Lifecycle State Machine

**ID**: A1
**Priority**: MVP Core
**Size**: M (~1 person-week)

**Description**: Introduce a three-state lifecycle — `pinned`, `active`, `archived` — persisted in a SQLite metadata overlay at `~/.config/sessions/sessions.db`. State is layered on top of the read-only source files; source files are never modified.

**Exact Behavior**:
- On `sessions tui` startup, open or create `~/.config/sessions/sessions.db`.
- On first run, create three tables and insert a schema version row (see Section 6 for exact DDL).
- Run a TTL sweep at startup: for every session in `sessions_meta` with `state = 'active'`, compute `now() - last_viewed_at`. If `> ttl_hours * 3600 seconds` (default `ttl_hours = 12`), update `state = 'archived'`.
- For sessions in source data with no row in `sessions_meta`, insert `state = 'active', last_viewed_at = now(), ttl_hours = 12`.
- Sessions with `session.Active = true` (process alive AND file modified < 2 min) are always displayed as `active` in the TUI, even if the metadata row says `archived` — active process detection overrides lifecycle state for display only.
- The `state` column has three valid values: `pinned`, `active`, `archived`. No other values are valid.

**Edge Cases**:
- `~/.config/sessions/` directory does not exist: create it with `os.MkdirAll`.
- `sessions.db` is corrupt (cannot open): log warning to stderr, continue with no lifecycle metadata (all sessions display as `active`).
- Session exists in `sessions_meta` but no longer in any source: ignore orphaned rows (no cleanup in MVP).
- TTL sweep with 1000+ sessions: run synchronously at startup. Acceptable — SQLite UPDATE is fast at this scale.

**Acceptance Criteria**:
1. `~/.config/sessions/sessions.db` is created on first `sessions tui` invocation with tables `sessions_meta`, `summaries`, `schema_version` and a row `(1, '1.0.0')` in `schema_version`.
2. After 12 hours of inactivity (last_viewed_at older than 12h), a session's state is `archived` in the DB as verified by `sqlite3 ~/.config/sessions/sessions.db "SELECT state FROM sessions_meta WHERE session_qualified_id='claude:abc123'"`.
3. A session with `session.Active = true` is always shown in the ACTIVE section of the sidebar, regardless of its `sessions_meta.state` value.
4. Corrupted DB (zero-byte file) causes a stderr warning but `sessions tui` continues to open showing sessions without lifecycle state.
5. `make check` passes with no new test failures.

**Implementation Notes**:
- New package: `internal/lifecycle/store.go` — implements `Store` struct with `Open(dbPath string) (*Store, error)`, `RunTTLSweep(sessions []model.Session)`, `GetState(qualifiedID string) (State, error)`, `SetState(qualifiedID string, state State) error`, `TouchViewed(qualifiedID string) error`.
- DB path resolved via `os.UserHomeDir()` + `/.config/sessions/sessions.db`.
- Use `modernc.org/sqlite` (already in `go.mod`) — never `mattn/go-sqlite3`.
- Import in `cmd/tui.go`: open the lifecycle store before calling sources, pass enriched sessions to `tui.New()`.
- New type `enrichedSession` in `internal/tui/` with `Session model.Session` and `LifecycleState lifecycle.State`.

---

### F-C4: Graceful Degradation to Heuristic Preview

**ID**: C4
**Priority**: MVP Core
**Size**: XS (~2 hours)

**Description**: The center panel always shows meaningful session content using a priority cascade: (1) cached LLM summary from `summaries` table, (2) `session.Summary` field, (3) `session.Preview` field truncated to 200 chars, (4) `session.QualifiedID()` as last resort.

**Exact Behavior**:
- Center panel renders the result of `resolvePreview(enrichedSession)`.
- `resolvePreview` checks in order: cached summary in SQLite (stub for MVP — always empty), then `session.Summary`, then `session.Preview`, then `session.QualifiedID()`.
- When showing a heuristic preview (not a cached LLM summary), render a subtle indicator: a dim gray `[preview]` tag in the top-right corner of the center panel.
- When no API key is configured (`os.Getenv("ANTHROPIC_API_KEY") == ""`), the indicator changes to `[no key]` to communicate that LLM enrichment is unavailable.
- The cascade is stateless and pure — no API calls in MVP.

**Edge Cases**:
- `session.Summary` is a non-empty but short string (e.g., "Refactor"): display it as-is without truncation.
- `session.Preview` is empty and `session.Summary` is empty: fall through to `session.QualifiedID()`.
- Very long `session.Preview` (>500 chars): truncate to 200 chars with "..." suffix.

**Acceptance Criteria**:
1. With `ANTHROPIC_API_KEY` unset, every session in the center panel shows either `session.Summary`, `session.Preview`, or `session.QualifiedID()` — never an empty string.
2. The `[no key]` or `[preview]` indicator is visible in the center panel top-right area when no cached summary is used.
3. A session where `session.Summary = ""` and `session.Preview = ""` shows the qualified ID (e.g., `claude:5c3f2742`) in the center panel.
4. Preview text longer than 200 chars is truncated with "..." at the 200-char boundary.

**Implementation Notes**:
- Implement as function `resolvePreview(s enrichedSession, apiKeySet bool) previewResult` in `internal/tui/model.go`.
- `previewResult` struct: `{ text string; source previewSource }` where `previewSource` is an enum (`sourceLLM`, `sourceSummary`, `sourcePreview`, `sourceID`).
- The center panel render function uses `previewResult.source` to decide the indicator string.
- No new packages required.

---

### F-B2: Tool-Based Color Coding

**ID**: B2
**Priority**: MVP Core
**Size**: S (~half day)

**Description**: Each AI tool is assigned a distinct color applied as a colored left border on sidebar session entries. Color palette defined in a central theme struct.

**Exact Behavior**:
- Sidebar session entry row has a 1-char wide colored left border using `lipgloss.NewStyle().Border(lipgloss.InnerHalfBlockBorder(), false, false, false, true).BorderForeground(toolColor)`.
- Tool color map:
  - `claude` → `#4A9EFF` (blue)
  - `cursor` → `#9B59B6` (purple)
  - `codex` → `#2ECC71` (green)
  - `gemini` → `#F1C40F` (yellow)
- When a session is selected (cursor on it), the entire row uses `styleSelected` (Bold + Reverse) — the border color is preserved in the border character but the row background reverses.
- Colors are defined as `lipgloss.Color` hex values (truecolor). No terminal capability downgrade in MVP; lipgloss handles degradation automatically.

**Edge Cases**:
- New tool not in the color map: fall back to white (`#FFFFFF`).
- Terminal that does not support truecolor: lipgloss auto-degrades to ANSI256 or ANSI16 — no special handling needed.
- Stub tools (Codex, Gemini) with no sessions: color map entries defined but no rows rendered.

**Acceptance Criteria**:
1. A Claude Code session entry in the sidebar has a blue left border visible in a truecolor terminal.
2. A Cursor session entry has a purple left border.
3. The selected session row shows reverse video styling without losing the tool-color border character.
4. Adding a new tool string not in the color map does not panic — it renders with a white border.

**Implementation Notes**:
- New file: `internal/tui/theme.go`.
- Define `var ToolColors = map[model.Tool]lipgloss.Color{ ... }` and `func ToolStyle(t model.Tool) lipgloss.Style`.
- Import `model` package in `theme.go` (already done in `model.go`).
- Used in sidebar row renderer in `model.go`.

---

### F-B3: Active Session Pulse Indicator

**ID**: B3
**Priority**: MVP Core
**Size**: S (~half day)

**Description**: Sessions detected as currently active (process alive + file modified < 2 min) show an animated pulsing bullet in the sidebar. The bullet alternates between bright green `●` and dim `●` on each tick (every 800ms).

**Exact Behavior**:
- `Model.Init()` returns `tickCmd()` — a `tea.Cmd` that fires a `tickMsg` after 800ms.
- Each `tickMsg` toggles a `bool tickPhase` field in the `Model` and schedules the next `tickCmd`.
- Session rows with `session.Active = true` render: `tickPhase == true` → `lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC00")).Render("●")`, `tickPhase == false` → `lipgloss.NewStyle().Faint(true).Render("●")`.
- Session rows with `session.Active = false` render a non-animated space character (same width as the bullet).
- Active detection logic remains in `internal/detect/process.go` — the TUI does not re-run detection on each tick; it reads the `session.Active` field loaded at startup.

**Edge Cases**:
- No active sessions: tick still fires (it is always running) but no visible pulse occurs. This is acceptable — tick is cheap (no I/O).
- TUI exit: bubbletea teardown cancels all running commands; `tickCmd` goroutine does not leak because it uses `tea.Tick` which returns a `tea.Cmd` (fire-and-forget, not a goroutine the TUI owns).
- Session becomes active after TUI is opened: not reflected in MVP (no watch mode). The `Active` field is set at load time.

**Acceptance Criteria**:
1. A session known to be active (process running) shows a green `●` character in the sidebar.
2. The `●` character visibly alternates bright/dim at approximately 800ms intervals when watching the TUI.
3. Pressing `q` exits cleanly with no goroutine leaks (verified by no hung process after exit).
4. A session with `session.Active = false` shows a space character aligned with the bullet position of active sessions.

**Implementation Notes**:
- Modify `Model.Init()` in `internal/tui/model.go` to return `tea.Tick(800*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })`.
- Add `type tickMsg time.Time` to `model.go`.
- Add `tickPhase bool` field to `Model` struct.
- Handle `tickMsg` in `Update()`: toggle `m.tickPhase`, return next `tea.Tick` command.
- Active indicator rendering moved from current `renderRow()` to new sidebar row renderer.

---

### F-E5: Status Bar with Context and Mode Indicators

**ID**: E5
**Priority**: MVP Core
**Size**: S (~half day)

**Description**: A persistent full-width bottom status bar showing: current lifecycle section in focus, total session count (visible / total), and LLM enrichment mode (cached / preview / no-key).

**Exact Behavior**:
- Status bar is the last line rendered by `View()`, always visible, using `lipgloss.NewStyle().Width(m.width).Background(lipgloss.Color("#1A1A2E")).Foreground(lipgloss.Color("#CCCCCC"))`.
- Left segment: current section name — `PINNED`, `ACTIVE`, or `ARCHIVED` — matching the sidebar section where the cursor sits.
- Center segment: session counts — e.g., `12 sessions (3 pinned, 7 active, 2 archived)`.
- Right segment: enrichment mode — `[LLM: cached]`, `[LLM: preview]`, or `[LLM: no key]`.
- All three segments separated by padding to fill the full width.
- Status bar content is derived purely from the current model state — no additional data fetches.

**Edge Cases**:
- Terminal width exactly 80 cols: status bar must not wrap. Truncate center segment if total width exceeds terminal width.
- All sessions in ARCHIVED section: left segment shows `ARCHIVED`.
- 0 sessions: status bar shows `0 sessions (0 pinned, 0 active, 0 archived)`.

**Acceptance Criteria**:
1. Status bar renders on the last line of the TUI in all tested terminal widths (80, 120, 200 cols).
2. Status bar does not wrap to a second line at 80 columns.
3. The lifecycle section name in the status bar updates as the cursor moves between PINNED, ACTIVE, and ARCHIVED sections.
4. The enrichment mode shows `[LLM: no key]` when `ANTHROPIC_API_KEY` is not set.
5. Session count totals are arithmetically correct (pinned + active + archived = total).

**Implementation Notes**:
- Implement `func (m Model) renderStatusBar() string` in `internal/tui/model.go`.
- Use `lipgloss.NewStyle().Width(m.width)` for the bar container to ensure full-width rendering.
- Enrichment mode derived from model field `m.apiKeyAvailable bool` set at `New()` time via `os.Getenv("ANTHROPIC_API_KEY") != ""`.
- `View()` assembles: `lipgloss.JoinVertical(lipgloss.Left, mainContent, m.renderStatusBar())`.

---

### F-A3: Pin / Unpin Session Action (MVP Polish)

**ID**: A3
**Priority**: MVP Polish
**Size**: S (~half day)

**Description**: Pressing `p` on any session in the sidebar toggles its pinned state. Pinned sessions move to the PINNED section immediately.

**Exact Behavior**:
- In `Update()`, `KeyMsg "p"` calls `m.lifecycleStore.TogglePin(qualifiedID)`.
- If current state is `pinned` → set to `active` with `last_viewed_at = now()`.
- If current state is `active` or `archived` → set to `pinned`.
- After toggling, re-sort the in-memory session list: pinned first (by `pinned_at` desc), then active (by `UpdatedAt` desc), then archived (by `last_viewed_at` desc).
- Pinned sessions render with a `[P]` or pin character prefix in the sidebar (before the colored border).
- Pinned sessions are never included in the TTL sweep.

**Edge Cases**:
- DB write fails (disk full): log to stderr, revert the in-memory state, show inline message "pin failed: disk full".
- Pinning a session that is already `pinned`: unpins it (toggle behavior).
- Pinning when the lifecycle store is not available (degraded mode): show inline message "pin not available (no metadata store)".

**Acceptance Criteria**:
1. Pressing `p` on an active session moves it to the PINNED section immediately (same TUI frame after the key press).
2. Pressing `p` on a pinned session returns it to the ACTIVE section.
3. After pin, running `sqlite3 ~/.config/sessions/sessions.db "SELECT state FROM sessions_meta WHERE ..."` shows `pinned`.
4. Pinned sessions are not affected by the TTL sweep (re-verified by waiting for next startup sweep).

**Implementation Notes**:
- Add `TogglePin(qualifiedID string) error` to `lifecycle.Store` in `internal/lifecycle/store.go`.
- Handle `"p"` key in `Update()` in `internal/tui/model.go`.
- Add `pinned_at DATETIME` column to `sessions_meta` DDL (set when state transitions to `pinned`, cleared on unpin).

---

### F-A4: Archive Sweep on View (Reset Inactivity Timer) (MVP Polish)

**ID**: A4
**Priority**: MVP Polish
**Size**: XS (~1 hour)

**Description**: Each time a session is selected (cursor moves to it), reset `last_viewed_at` in `sessions_meta` to prevent it from being auto-archived while in active use.

**Exact Behavior**:
- On `KeyMsg "up"` or `"down"`, after updating cursor position, call `m.lifecycleStore.TouchViewed(qualifiedID)` asynchronously (fire-and-forget via goroutine — do not block the render loop).
- `TouchViewed` is a simple `UPDATE sessions_meta SET last_viewed_at = now() WHERE session_qualified_id = ?`.
- No user-visible feedback for this action — it is silent.
- Only `active` sessions get their timer reset. `pinned` and `archived` sessions are untouched by `TouchViewed`.

**Edge Cases**:
- DB write in goroutine fails: log to stderr. TUI continues normally.
- Rapid cursor movement (j/j/j/j): each move triggers a goroutine. At typical speeds this is 1–5 DB writes/second — acceptable.

**Acceptance Criteria**:
1. After selecting a session and waiting 11 hours, a second TUI startup does NOT archive that session (TTL starts from last view, not from original `UpdatedAt`).
2. `last_viewed_at` in `sessions_meta` is updated within 1 second of cursor moving to a session.
3. Fire-and-forget goroutine does not cause a visible delay or stutter in navigation.

**Implementation Notes**:
- `TouchViewed` already specified in A1 `lifecycle.Store` interface.
- Modify cursor movement handlers in `internal/tui/model.go` to call `go m.lifecycleStore.TouchViewed(...)` after cursor update.

---

### F-B6: Session Age and Staleness Indicators (MVP Polish)

**ID**: B6
**Priority**: MVP Polish
**Size**: XS (~1 hour)

**Description**: Each session entry in the sidebar shows a human-readable age string (e.g., "2h", "3d") and is visually dimmed when its age exceeds the TTL threshold (12h default).

**Exact Behavior**:
- Age string is computed as `output.FormatDuration(time.Since(session.UpdatedAt))` — uses existing function in `internal/output/render.go`.
- Age string is right-aligned in the sidebar entry, occupying at most 4 characters (e.g., "2h", "3d", "1w").
- Sessions with `time.Since(session.UpdatedAt) > 12*time.Hour` are rendered with `lipgloss.NewStyle().Faint(true)` applied to the entire row (except the tool-color border which retains its color).
- Staleness threshold is hardcoded to 12h in MVP (configurable TTL is Wave 2 A2).

**Edge Cases**:
- Session `UpdatedAt` is zero time: show "?" as the age string.
- Session `UpdatedAt` is in the future (clock skew): show "0s".
- `output.FormatDuration` already handles sub-minute, minute, hour, day, week cases — no additional formatting needed.

**Acceptance Criteria**:
1. A session last updated 3 days ago shows "3d" in the sidebar (or the equivalent `FormatDuration` output).
2. A session last updated 13 hours ago is rendered with dimmed/faint styling.
3. A session last updated 1 hour ago is rendered with normal styling.
4. Age strings do not overflow the allocated 4-character width in the sidebar.

**Implementation Notes**:
- `output.FormatDuration` is defined in `internal/output/render.go`. Import `output` package in `tui/model.go` (already imported in current `model.go` at line 12).
- Add age rendering in the sidebar row renderer function.
- Staleness check: `time.Since(session.UpdatedAt) > 12*time.Hour`.

---

## 5. UI/UX Specification

### Primary Layout

```
┌─────────────────────────────┬─────────────────────────────────────┬────────────────────────┐
│  SIDEBAR (30 cols fixed)    │  CENTER PANEL (fluid)               │  DETAIL PANEL (40 cols)│
│                             │                                     │  (only when Enter)     │
│ ─ PINNED ──────────────── │ [preview]                           │                        │
│ │[P] claude │my-project  2d │ claude:5c3f2742                    │ Full conversation      │
│ ─ ACTIVE (Today) ──────── │                                     │ messages rendered      │
│ │●  claude │sessions   12m │ Refactor the source interface to... │ here with role labels. │
│ │   cursor │b2b-orders   1h │                                     │                        │
│ ─ ARCHIVED ────────────── │                                     │                        │
│ │   codex  │some-proj   3d │                                     │                        │
└─────────────────────────────┴─────────────────────────────────────┴────────────────────────┘
│ ACTIVE  │  12 sessions (2 pinned, 8 active, 2 archived)  │  [LLM: no key]              │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

**Panel Widths**:
- Sidebar: 30 columns, fixed.
- Detail panel: 40 columns, fixed, only rendered when `m.detailOpen == true`.
- Center panel: `terminalWidth - 30 - (40 if detail open else 0)` — minimum 10 cols.
- Status bar: full terminal width, 1 line, always rendered.

**Sidebar Layout (per row)**:
```
│<border-1char><pulse-1char><tool-badge-7chars><project-17chars><age-4chars>│
```
Total row: 1 (border) + 1 (pulse or space) + 7 (tool) + 1 (space) + 17 (project) + 1 (space) + 4 (age) = 32 chars → fit within 30 cols by sharing: border is the lipgloss left-border, pulse+tool+project+age fit in 29 chars of content.

Practical sidebar row content (29 chars of content after the border):
```
● claude │my-project     2h
```
- Pulse indicator: 1 char + 1 space = 2 chars
- Tool name truncated to 6 chars + 1 space = 7 chars
- Project (last component) truncated to 14 chars + 1 space = 15 chars
- Age: 4 chars
- Total: 2 + 7 + 15 + 4 = 28 chars (fits in 29)

**Three Sections in List Panel**:
1. `PINNED` — sessions with `state = pinned` in `sessions_meta`. Sorted by `pinned_at` desc.
2. `ACTIVE (Today)` — sessions with `state = active` (or `session.Active = true` override). Sorted by `UpdatedAt` desc.
3. `ARCHIVED` — sessions with `state = archived`. Sorted by `last_viewed_at` desc. Rendered with `lipgloss.NewStyle().Faint(true)`.

Section headers use: `lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AAAAAA")).Render("─ PINNED ─────────────────")`.

### Color Palette (lipgloss)

| Element                  | Hex Color   | lipgloss Usage                                          |
|--------------------------|-------------|--------------------------------------------------------|
| Active sessions (pulse)  | `#00CC00`   | `lipgloss.Color("#00CC00")` for bright bullet         |
| Active sessions (dim)    | `#336633`   | `lipgloss.Color("#336633")` for dim bullet phase      |
| Pinned indicator `[P]`   | `#00CCCC`   | `lipgloss.Color("#00CCCC")` for cyan pin marker       |
| Archived / stale rows    | dim         | `lipgloss.NewStyle().Faint(true)`                     |
| Tool: Claude             | `#4A9EFF`   | Left border of sidebar entry                          |
| Tool: Cursor             | `#9B59B6`   | Left border of sidebar entry                          |
| Tool: Codex              | `#2ECC71`   | Left border of sidebar entry                          |
| Tool: Gemini             | `#F1C40F`   | Left border of sidebar entry                          |
| Status bar background    | `#1A1A2E`   | Bar container background                              |
| Status bar foreground    | `#CCCCCC`   | Bar text color                                        |
| Section headers          | `#AAAAAA`   | Section title text                                    |

**Pulsing Active Indicator**:
- Tick fires every 800ms via `tea.Tick`.
- `tickPhase == true`: `lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC00")).Render("●")`
- `tickPhase == false`: `lipgloss.NewStyle().Foreground(lipgloss.Color("#336633")).Render("●")`
- Inactive sessions (and archived): `" "` (space, same width as `●`).

### Keyboard Shortcuts (Complete List)

| Key          | Action                                            |
|--------------|---------------------------------------------------|
| `j` / `Down` | Move cursor down in sidebar                       |
| `k` / `Up`   | Move cursor up in sidebar                         |
| `Enter`      | Toggle detail panel open/closed for selection     |
| `r`          | Resume session in-place (existing resume behavior)|
| `t`          | Open session in tmux window                       |
| `a`          | Open session in AoE                               |
| `o`          | Open project directory                            |
| `f`          | Fork session (if supported by tool)               |
| `p`          | Toggle pin / unpin on selected session            |
| `q` / `Esc`  | Quit                                              |
| `Ctrl+C`     | Force quit                                        |

### LLM States (Center Panel Indicator)

| State              | Condition                                      | Indicator shown         |
|--------------------|------------------------------------------------|-------------------------|
| `cached`           | Summary found in `summaries` SQLite table      | `[LLM: cached]` (green) |
| `preview`          | No cache, using `session.Summary`/`Preview`    | `[preview]` (dim gray)  |
| `no-key`           | `ANTHROPIC_API_KEY` not set                    | `[LLM: no key]` (dim)   |
| `loading` (stub)   | Wave 2 — LLM call in progress                  | spinner (not MVP)        |

In MVP, `cached` state is never reached (no LLM calls). The status bar right segment shows the active indicator. The center panel top-right shows the per-session indicator.

---

## 6. Technical Architecture

### New Packages

**`internal/lifecycle/`**:
- `store.go` — SQLite-backed lifecycle state store.
  ```go
  type State string
  const (
      StateActive   State = "active"
      StatePinned   State = "pinned"
      StateArchived State = "archived"
  )
  type Store struct { db *sql.DB }
  func Open(dbPath string) (*Store, error)
  func (s *Store) Close() error
  func (s *Store) RunTTLSweep(sessions []model.Session) error
  func (s *Store) GetState(qualifiedID string) (State, error)
  func (s *Store) SetState(qualifiedID string, state State) error
  func (s *Store) TouchViewed(qualifiedID string) error
  func (s *Store) TogglePin(qualifiedID string) error
  func (s *Store) UpsertIfMissing(qualifiedID string) error
  ```
- Does NOT import any `internal/source/*` package (invariant preserved).
- Imports: `database/sql`, `modernc.org/sqlite`, `github.com/psacc/omnisess/internal/model`.

**`internal/enrichment/`** (Wave 2 stub only — do NOT implement in MVP):
- Create a stub `enrichment.go` with a comment: `// Wave 2: LLM summary enrichment. See feature C1.`
- No actual code — just the package directory and a stub file so the package exists for future import.
- **Do not add this package to go.mod; it will be a local package with no external deps.**

### New Metadata Storage

**File**: `~/.config/sessions/sessions.db` (resolved at runtime via `os.UserHomeDir()`)

**DDL** (exact schema, executed at first open):
```sql
CREATE TABLE IF NOT EXISTS schema_version (
    version_id  INTEGER PRIMARY KEY,
    version     TEXT NOT NULL,
    applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_version (version_id, version) VALUES (1, '1.0.0');

CREATE TABLE IF NOT EXISTS sessions_meta (
    session_qualified_id TEXT PRIMARY KEY,  -- e.g. "claude:5c3f2742"
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

**TTL Sweep Query** (runs at startup):
```sql
UPDATE sessions_meta
SET    state = 'archived'
WHERE  state = 'active'
AND    (julianday('now') - julianday(last_viewed_at)) * 24 > ttl_hours;
```

### Changes to Existing Files

**`cmd/tui.go`** (`cmd/tui.go`):
- After `getSources()` / `Source.List()`, open `lifecycle.Store`.
- Run `store.UpsertIfMissing()` for each session.
- Run `store.RunTTLSweep()`.
- Wrap each `model.Session` into `tui.EnrichedSession{Session: s, LifecycleState: store.GetState(s.QualifiedID())}`.
- Pass `enrichedSessions` and `lifecycleStore` to updated `tui.New()`.
- Defer `store.Close()`.

**`internal/tui/model.go`** (`internal/tui/model.go`):
- Replace current single-list `Model` with three-panel composite model.
- Add `tickPhase bool`, `detailOpen bool`, `sections []LifecycleSection`, `apiKeyAvailable bool`, `lifecycleStore *lifecycle.Store` fields.
- Add `Init()` returning `tea.Tick(800ms)`.
- Add `renderStatusBar()`, `renderSidebar()`, `renderCenter()`, `renderDetail()` methods.

**`internal/tui/theme.go`** (new file):
- Tool color map and `ToolStyle()` function.

**`go.mod`**: No changes. All required packages (bubbletea, lipgloss, modernc.org/sqlite) are already present.

### Data Flow

```
sessions tui (cmd/tui.go)
    │
    ├── Source.List() × N sources → []model.Session
    │
    ├── lifecycle.Open(~/.config/sessions/sessions.db)
    │       └── RunTTLSweep()
    │       └── UpsertIfMissing() × N sessions
    │
    ├── Enrich: []EnrichedSession{Session, LifecycleState}
    │
    └── tui.New(enrichedSessions, toolModes, lifecycleStore)
            └── tea.NewProgram(m, tea.WithAltScreen()).Run()
                    ├── Init() → tickCmd (800ms)
                    ├── Update(WindowSizeMsg) → recompute widths
                    ├── Update(tickMsg) → toggle tickPhase
                    ├── Update(KeyMsg "p") → TogglePin
                    ├── Update(KeyMsg "j/k") → TouchViewed (goroutine)
                    └── View() → JoinVertical(mainPanels, statusBar)
```

### Dependency Constraints

- No new entries in `go.mod` for MVP.
- `internal/lifecycle/` imports: `database/sql`, `modernc.org/sqlite` (already in go.sum).
- `internal/tui/` imports: `internal/lifecycle/`, `internal/model/`, `internal/output/` (already imported).
- Source packages remain isolated — `internal/lifecycle/` is NOT imported by any `internal/source/*` package.

---

## 7. Out of Scope (Wave 2+)

**Wave 2** features deferred from MVP:

| ID | Feature | Reason Deferred |
|----|---------|----------------|
| C1 | LLM-Generated Summaries (on-demand) | Requires new SDK dep + API key. Validate baseline first. |
| E1 | Watch Mode (auto-refresh) | Goroutine lifecycle management risk. Validate static dashboard value first. |
| D1 | Compact Single-Column Mode | Effort trade-off. Most users have >= 80 col terminals. |
| E4 | Resume Mode Quick-Select Tray | Good UX. Wave 2 once B1 is stable. |
| A2 | Configurable TTL per session | Validate that 12h default is right before adding config complexity. |
| A5 | Archive Restore Action | Rare action. Ship after measuring archive adoption. |
| E3 | Project Filter Sticky State | Valuable at 5+ projects. Validate usage patterns first. |
| B5 | Keyboard Help Overlay | Nice-to-have. Add once all keys are stable. |

**Backlog**:
B4 (fuzzy search), E2 (shell aliases), D4 (JSON export), C5 (configurable model), D2 (panel ratios), D5 (tmux popup), C2 (batch summaries), D3 (web UI), C3 (LLM topic grouping).

---

## 8. Success Criteria

The MVP is working when ALL of the following are observable without any manual inspection of source code:

1. `sessions tui` opens a three-panel layout (sidebar / center / detail toggle) without crashing on a terminal >= 80 columns wide.

2. Sessions in the sidebar are visually differentiated by tool: Claude entries have a blue left border, Cursor entries have a purple left border, when viewed in a truecolor terminal (iTerm2, Ghostty, Warp).

3. At least one session known to be actively running (Claude Code or Cursor process alive, file modified < 2 min) shows a green `●` that visibly alternates to dim at approximately 800ms intervals.

4. The status bar at the bottom renders the current lifecycle section name, session count, and LLM enrichment status (`[LLM: no key]` when `ANTHROPIC_API_KEY` is unset) without layout overflow at 80 and 120 column terminal widths.

5. Session preview text is non-empty for every session displayed in the center panel, regardless of whether `ANTHROPIC_API_KEY` is set.

6. `~/.config/sessions/sessions.db` is created on first run with the correct schema: three tables (`sessions_meta`, `summaries`, `schema_version`) and a row `(1, '1.0.0')` in `schema_version`.

7. A session not viewed for > 12 hours transitions to `state = 'archived'` in `sessions_meta` on the next `sessions tui` startup.

8. `make check` passes (fmt + vet + lint + test) with no new failures introduced by the dashboard implementation.

9. The TUI exits cleanly on `q` or `Ctrl+C` with no hung process observable via `ps aux | grep sessions` after exit.

10. Pressing `p` on a session in the ACTIVE section moves it to the PINNED section in the same TUI frame, and pressing `p` again returns it to ACTIVE.
