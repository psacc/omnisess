# 011 — TUI Enhancements (3-Panel Layout)

**Status**: Not started
**Priority**: Medium
**Estimated effort**: 1–2 person-days
**Depends on**: 010-session-names (Name() must be available before sidebar renders)
**Blocks**: nothing

## Problem

The current TUI is a single-column list. It shows qualified IDs and timestamps — enough to pick a session, but not enough to understand sessions at a glance. As session counts grow (especially with multiple tools active), the flat list requires more mental work to navigate.

A three-panel layout — session list, message preview, metadata — gives the information hierarchy needed without requiring any new infrastructure.

## Proposed Layout

```
┌──────────────────┬───────────────────────────┬─────────────────┐
│  Sessions (30)   │  Preview                  │  Metadata       │
│                  │                           │                 │
│ > claude  foo    │  "refactor the auth       │  Tool:  Claude  │
│   cursor  bar    │   middleware to use JWT   │  ID:    5c3f27  │
│   claude  baz    │   and drop the session    │  Proj:  omnisess│
│                  │   table entirely..."      │  Age:   2h      │
│                  │                           │  Msgs:  42      │
└──────────────────┴───────────────────────────┴─────────────────┘
│ ACTIVE  12 sessions  [LLM: no key]                             │
└────────────────────────────────────────────────────────────────┘
```

Width rules (same as 009's Phase 2 spec):
- Sidebar: 30 cols fixed
- Detail panel: 40 cols, shown/hidden with `Tab` or `d`
- Center: remaining width (minimum 10)
- Terminal < 80 cols: print "Terminal too narrow (min 80 cols)" and exit

## Proposed Approach

This plan intentionally inherits the Phase 2–5 implementation design from the now-superseded `009-visual-dashboard.md`, stripped of the lifecycle-store integration (which belongs to 012) and with `Name()` from 010 used in sidebar rows.

### Track 1 — Layout skeleton (no lifecycle store)

- Replace `internal/tui/model.go` single-list model with three-panel composite model.
- All sessions placed in a single ACTIVE section (no state bucketing until 012 is wired in).
- `WindowSizeMsg` recomputes panel widths.
- Stub renderers: `renderSidebar()`, `renderCenter()`, `renderDetail()`, `renderStatusBar()`.
- Existing resume keybindings (`Enter`, `r`, `t`, `a`, `f`, `o`, `q`) remain functional.

### Track 2 — Tool color coding + active pulse

- `internal/tui/theme.go`: `ToolColors` map and `ToolStyle()` (left border in tool color).
- Sidebar row: `<pulse><space><Name()><right-align age>`.
- 800ms tick: alternating `●` (bright/dim green) for `session.Active == true`.

### Track 3 — Center panel (preview) + metadata panel

- Center: `resolvePreview()` cascade — `session.Summary` → `session.Preview` → `session.Name()` → `session.QualifiedID()`.
- Detail: tool, qualified ID, project path, age, message count.
- Status bar: session count, LLM availability indicator.

### Lifecycle store integration point

When 012-lifecycle-store lands, 011 will accept an optional `*lifecycle.Store` in `tui.New()`. If nil, all sessions stay in ACTIVE. If non-nil, sessions are bucketed into PINNED / ACTIVE / ARCHIVED and `p` key becomes active. This keeps 011 shippable before 012.

## Open Questions

1. Should the detail panel be open by default, or hidden until toggled? Leaning hidden — saves width on narrow terminals and 80-col is the common case.
2. Does merging 011 before 012 mean we ship a TUI with no persistence? Yes, intentionally. The TUI is still useful without lifecycle state.
3. `model_test.go` vs `model_extra_test.go` — the repo already has `internal/tui/model_extra_test.go` (untracked). Coordinate to avoid conflicts.

## Scope Estimate

- `internal/tui/model.go` — rewrite (~300 lines; existing file is a stub)
- `internal/tui/theme.go` — new file (~40 lines)
- `internal/tui/model_test.go` or `model_extra_test.go` — `resolvePreview()` cascade, sidebar row formatting, width clamping
- `cmd/tui.go` — minor: pass enriched sessions to updated `tui.New()` signature
- No new `go.mod` dependencies (bubbletea, lipgloss already present)

## Dependency Graph

```
010-session-names  ──►  011-tui-enhancements
                              │
                              └── optional wire-in ──►  012-lifecycle-store
```

011 can ship without 012. 012 adds lifecycle bucketing as an optional upgrade.

## Out of Scope

- LLM-generated summaries (Wave 2)
- Watch mode / auto-refresh
- Fuzzy search overlay
- Keyboard help overlay
- Compact single-column mode (< 80 cols)
