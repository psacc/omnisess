package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/procsnap"
)

// testSessions returns a slice of sessions for testing.
func testSessions() []model.Session {
	now := time.Now()
	return []model.Session{
		{
			ID:        "aaa11111-1111-1111-1111-111111111111",
			Tool:      model.ToolClaude,
			Project:   "/home/user/projects/myapp",
			Preview:   "Implement TUI session picker",
			StartedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-5 * time.Minute),
			Active:    true,
		},
		{
			ID:        "bbb22222-2222-2222-2222-222222222222",
			Tool:      model.ToolCursor,
			Project:   "/home/user/projects/webapp",
			Preview:   "Fix authentication bug",
			StartedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-30 * time.Minute),
			Active:    false,
		},
		{
			ID:        "ccc33333-3333-3333-3333-333333333333",
			Tool:      model.ToolClaude,
			Project:   "/home/user/projects/api",
			Preview:   "Add search endpoint",
			StartedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
			Active:    true,
		},
	}
}

// testToolModes returns tool mode mappings matching Claude and Cursor resumers.
func testToolModes() map[model.Tool][]string {
	return map[model.Tool][]string{
		model.ToolClaude: {"resume", "fork", "tmux", "aoe"},
		model.ToolCursor: {"resume", "tmux", "aoe"},
	}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name        string
		sessions    []model.Session
		toolModes   map[model.Tool][]string
		keys        []tea.Msg // sequence of messages to send
		wantCursor  int
		wantSelect  bool       // expect selected != nil
		wantTool    model.Tool // expected selected session's tool (if wantSelect)
		wantMode    string     // expected selectedMode
		wantQuit    bool
		wantMessage string // expected inline message
	}{
		{
			name:       "down moves cursor",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("j")},
			wantCursor: 1,
		},
		{
			name:       "up moves cursor",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("j"), keyMsg("j"), keyMsg("k")},
			wantCursor: 1,
		},
		{
			name:       "down arrow moves cursor",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyDown)},
			wantCursor: 1,
		},
		{
			name:       "up arrow moves cursor",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyUp)},
			wantCursor: 0,
		},
		{
			name:       "cursor clamps at top",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("k"), keyMsg("k"), keyMsg("k")},
			wantCursor: 0,
		},
		{
			name:       "cursor clamps at bottom",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("j"), keyMsg("j"), keyMsg("j"), keyMsg("j"), keyMsg("j")},
			wantCursor: 2, // 3 sessions, max index = 2
		},
		{
			name:       "enter on claude session selects with resume mode",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEnter)},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "resume",
			wantQuit:   true,
		},
		{
			name:       "enter on cursor session selects with resume mode",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("j"), specialKeyMsg(tea.KeyEnter)},
			wantCursor: 1,
			wantSelect: true,
			wantTool:   model.ToolCursor,
			wantMode:   "resume",
			wantQuit:   true,
		},
		{
			name:        "enter on tool without resume shows message",
			sessions:    testSessions(),
			toolModes:   map[model.Tool][]string{}, // no modes registered
			keys:        []tea.Msg{specialKeyMsg(tea.KeyEnter)},
			wantCursor:  0,
			wantSelect:  false,
			wantQuit:    false,
			wantMessage: "resume not supported for claude",
		},
		{
			name:       "t key selects tmux mode on claude",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("t")},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "tmux",
			wantQuit:   true,
		},
		{
			name:       "t key selects tmux mode on cursor",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("j"), keyMsg("t")},
			wantCursor: 1,
			wantSelect: true,
			wantTool:   model.ToolCursor,
			wantMode:   "tmux",
			wantQuit:   true,
		},
		{
			name:       "a key selects aoe mode (always available)",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("a")},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "aoe",
			wantQuit:   true,
		},
		{
			name:       "a key selects aoe even with no tool modes",
			sessions:   testSessions(),
			toolModes:  map[model.Tool][]string{},
			keys:       []tea.Msg{keyMsg("a")},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "aoe",
			wantQuit:   true,
		},
		{
			name:       "f key selects fork mode on claude",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("f")},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "fork",
			wantQuit:   true,
		},
		{
			name:        "f key on cursor shows unsupported message",
			sessions:    testSessions(),
			toolModes:   testToolModes(),
			keys:        []tea.Msg{keyMsg("j"), keyMsg("f")},
			wantCursor:  1,
			wantSelect:  false,
			wantQuit:    false,
			wantMessage: "fork not supported for cursor",
		},
		{
			name:       "o key selects open mode on claude",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("o")},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "open",
			wantQuit:   true,
		},
		{
			name:       "o key selects open even with no tool modes",
			sessions:   testSessions(),
			toolModes:  map[model.Tool][]string{},
			keys:       []tea.Msg{keyMsg("o")},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantMode:   "open",
			wantQuit:   true,
		},
		{
			name:       "q quits without selection",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("q")},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   true,
		},
		{
			name:       "esc quits without selection",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEsc)},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   true,
		},
		{
			name:       "ctrl+c quits without selection",
			sessions:   testSessions(),
			toolModes:  testToolModes(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyCtrlC)},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   true,
		},
		{
			name:       "enter on empty sessions does nothing",
			sessions:   nil,
			toolModes:  testToolModes(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEnter)},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   false,
		},
		{
			name:       "down on empty sessions does nothing",
			sessions:   nil,
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("j")},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   false,
		},
		{
			name:       "a on empty sessions does nothing",
			sessions:   nil,
			toolModes:  testToolModes(),
			keys:       []tea.Msg{keyMsg("a")},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   false,
		},
		{
			name:       "message clears on next keypress",
			sessions:   testSessions(),
			toolModes:  map[model.Tool][]string{}, // no modes — enter will fail
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEnter), keyMsg("j")},
			wantCursor: 1,
			wantSelect: false,
			wantQuit:   false,
			// The message from enter should be cleared by the subsequent "j"
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.sessions, tt.toolModes)

			var mdl tea.Model = m
			for _, msg := range tt.keys {
				mdl, _ = mdl.Update(msg)
			}

			got := mdl.(Model)

			if got.cursor != tt.wantCursor {
				t.Errorf("cursor = %d, want %d", got.cursor, tt.wantCursor)
			}

			if tt.wantSelect && got.Selected() == nil {
				t.Error("expected Selected() != nil, got nil")
			}
			if !tt.wantSelect && got.Selected() != nil {
				t.Errorf("expected Selected() == nil, got %v", got.Selected())
			}
			if tt.wantSelect && got.Selected() != nil && got.Selected().Tool != tt.wantTool {
				t.Errorf("Selected().Tool = %s, want %s", got.Selected().Tool, tt.wantTool)
			}

			if tt.wantMode != "" && got.SelectedMode() != tt.wantMode {
				t.Errorf("SelectedMode() = %q, want %q", got.SelectedMode(), tt.wantMode)
			}

			if got.Quitting() != tt.wantQuit {
				t.Errorf("Quitting() = %v, want %v", got.Quitting(), tt.wantQuit)
			}

			if got.message != tt.wantMessage {
				t.Errorf("message = %q, want %q", got.message, tt.wantMessage)
			}
		})
	}
}

func TestWindowResize(t *testing.T) {
	m := New(testSessions(), testToolModes())

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	mdl, _ := m.Update(msg)
	got := mdl.(Model)

	if got.width != 120 {
		t.Errorf("width = %d, want 120", got.width)
	}
	if got.height != 40 {
		t.Errorf("height = %d, want 40", got.height)
	}
}

func TestViewEmpty(t *testing.T) {
	m := New(nil, nil)
	view := m.View()

	if !strings.Contains(view, "No sessions") {
		t.Errorf("empty view should contain 'No sessions', got: %q", view)
	}
}

func TestViewContainsSessionInfo(t *testing.T) {
	m := New(testSessions(), testToolModes())
	m.width = 120
	m.height = 30
	view := m.View()

	// Header should show active count
	if !strings.Contains(view, "2 active") {
		t.Errorf("view should show active count, got:\n%s", view)
	}

	// Should contain tool names
	if !strings.Contains(view, "claude") {
		t.Errorf("view should contain 'claude', got:\n%s", view)
	}
	if !strings.Contains(view, "cursor") {
		t.Errorf("view should contain 'cursor', got:\n%s", view)
	}

	// Should contain column headers
	if !strings.Contains(view, "TOOL") {
		t.Errorf("view should contain column header 'TOOL', got:\n%s", view)
	}

	// Should contain footer help
	if !strings.Contains(view, "enter: resume") {
		t.Errorf("view should contain footer help, got:\n%s", view)
	}
}

func TestViewScrolling(t *testing.T) {
	// Create many sessions to force scrolling.
	sessions := make([]model.Session, 20)
	now := time.Now()
	for i := range sessions {
		sessions[i] = model.Session{
			ID:        strings.Repeat("a", 36),
			Tool:      model.ToolClaude,
			Project:   "/home/user/project",
			Preview:   "session preview",
			StartedAt: now,
			UpdatedAt: now,
		}
	}

	m := New(sessions, testToolModes())
	m.width = 80
	m.height = 10 // Only ~6 visible rows (10 - 4 chrome lines)

	// Navigate down past visible area.
	var mdl tea.Model = m
	for i := 0; i < 8; i++ {
		mdl, _ = mdl.Update(keyMsg("j"))
	}

	got := mdl.(Model)
	if got.cursor != 8 {
		t.Errorf("cursor = %d, want 8", got.cursor)
	}
	// offset should have scrolled to keep cursor visible
	if got.offset == 0 {
		t.Error("expected offset > 0 after scrolling down")
	}
}

func TestRenderRowWidthBudget(t *testing.T) {
	sessions := testSessions()
	for _, width := range []int{80, 120, 200} {
		m := New(sessions, testToolModes())
		m.width = width
		pw := m.previewWidth()
		for i := range sessions {
			row := m.renderRow(i, pw)
			// Rendered row visible width must not exceed terminal width.
			// Note: lipgloss ANSI codes add non-visible bytes for the active indicator,
			// so we measure by stripping ANSI.
			visible := stripAnsi(row)
			if len(visible) > width {
				t.Errorf("width=%d, row %d: visible len=%d exceeds terminal width\nrow: %q", width, i, len(visible), visible)
			}
		}
	}
}

func TestPreviewWidthNarrowTerminal(t *testing.T) {
	m := New(testSessions(), testToolModes())
	m.width = 30 // Very narrow
	pw := m.previewWidth()
	if pw < 10 {
		t.Errorf("previewWidth() = %d at width=30, want >= 10 (clamped minimum)", pw)
	}
}

// stripAnsi removes ANSI escape sequences for width measurement.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestTruncatePad(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello     "},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"hello", 5, "hello"},
		{"ab", 1, "a"},
	}

	for _, tt := range tests {
		got := truncatePad(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncatePad(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestFooterHelp_ClaudeSession(t *testing.T) {
	m := New(testSessions(), testToolModes())
	// Cursor is on index 0 (claude session).
	footer := m.footerHelp()

	for _, want := range []string{"enter: resume", "t: tmux", "a: aoe", "o: open", "f: fork", "q: quit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("Claude footer should contain %q, got: %q", want, footer)
		}
	}
}

func TestFooterHelp_CursorSession(t *testing.T) {
	m := New(testSessions(), testToolModes())
	// Move cursor to index 1 (cursor session).
	var mdl tea.Model = m
	mdl, _ = mdl.Update(keyMsg("j"))
	got := mdl.(Model)
	footer := got.footerHelp()

	for _, want := range []string{"enter: resume", "t: tmux", "a: aoe", "o: open", "q: quit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("Cursor footer should contain %q, got: %q", want, footer)
		}
	}

	// Cursor does not support fork.
	if strings.Contains(footer, "f: fork") {
		t.Errorf("Cursor footer should NOT contain 'f: fork', got: %q", footer)
	}
}

func TestFooterHelp_UnknownTool(t *testing.T) {
	sessions := []model.Session{
		{
			ID:        "ddd44444-4444-4444-4444-444444444444",
			Tool:      model.ToolGemini,
			Project:   "/home/user/projects/gem",
			Preview:   "Test session",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	// No modes registered for gemini.
	m := New(sessions, map[model.Tool][]string{})
	footer := m.footerHelp()

	// AoE and open are always available.
	for _, want := range []string{"a: aoe", "o: open"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer should always contain %q, got: %q", want, footer)
		}
	}

	// No resume/tmux/fork for unknown tool.
	for _, notWant := range []string{"enter: resume", "t: tmux", "f: fork"} {
		if strings.Contains(footer, notWant) {
			t.Errorf("unknown tool footer should NOT contain %q, got: %q", notWant, footer)
		}
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// TestInit verifies that Init schedules the first snapshot tick.
func TestInit(t *testing.T) {
	m := New(testSessions(), testToolModes())
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() must return a tick command")
	}
}

// ---------------------------------------------------------------------------
// Update — unhandled key
// ---------------------------------------------------------------------------

// TestUpdate_UnhandledKey verifies that an unhandled key (e.g. "x") returns
// the model unchanged and a nil command.
func TestUpdate_UnhandledKey(t *testing.T) {
	m := New(testSessions(), testToolModes())
	updated, cmd := m.Update(keyMsg("x"))
	if cmd != nil {
		t.Errorf("Update(unhandled key): cmd = %v, want nil", cmd)
	}
	// Cursor should be unchanged.
	got := updated.(Model).cursor
	if got != 0 {
		t.Errorf("Update(unhandled key): cursor = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// View — with inline message
// ---------------------------------------------------------------------------

// TestView_WithMessage verifies that an inline message is rendered in View
// and also exercises the extra++ path in visibleRows.
func TestView_WithMessage(t *testing.T) {
	m := New(testSessions(), testToolModes())
	m.message = "something went wrong"
	view := m.View()
	if !strings.Contains(view, "something went wrong") {
		t.Errorf("View() with message: expected message in output, got:\n%s", view)
	}
}

// ---------------------------------------------------------------------------
// renderRow — empty preview
// ---------------------------------------------------------------------------

// TestRenderRow_EmptyPreview verifies that a session with empty Preview
// falls back to QualifiedID() in the rendered row.
func TestRenderRow_EmptyPreview(t *testing.T) {
	sess := model.Session{
		ID:        "short01",
		Tool:      model.ToolClaude,
		Project:   "/tmp/testproject",
		Preview:   "", // empty: should fall back to QualifiedID
		UpdatedAt: time.Now(),
	}
	m := New([]model.Session{sess}, testToolModes())
	row := m.renderRow(0, 30)
	if !strings.Contains(row, sess.QualifiedID()) {
		t.Errorf("renderRow empty preview: expected QualifiedID %q in row %q", sess.QualifiedID(), row)
	}
}

// ---------------------------------------------------------------------------
// visibleRows — tiny height clamping
// ---------------------------------------------------------------------------

// TestVisibleRows_TinyHeight verifies that visibleRows returns 1 when the
// terminal height is smaller than chromeLines.
func TestVisibleRows_TinyHeight(t *testing.T) {
	m := New(testSessions(), testToolModes())
	m.height = 3 // less than chromeLines (4)
	got := m.visibleRows()
	if got != 1 {
		t.Errorf("visibleRows() with height=3: got %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// clampViewport — scroll-up path
// ---------------------------------------------------------------------------

// TestClampViewport_ScrollUp verifies the scroll-up path in clampViewport:
// when cursor moves above the current offset, offset is adjusted downward.
func TestClampViewport_ScrollUp(t *testing.T) {
	sessions := testSessions()
	m := New(sessions, testToolModes())
	m.height = 5 // small viewport so offset can be > 0

	// Scroll down past visible rows to push offset > 0.
	for i := 0; i < len(sessions); i++ {
		m.cursor = i
		m.clampViewport()
	}
	if m.offset == 0 {
		t.Skip("all sessions fit in viewport; scroll-up path not reachable")
	}
	// Now move cursor back above offset.
	m.cursor = 0
	m.clampViewport()
	if m.offset != 0 {
		t.Errorf("clampViewport scroll-up: offset = %d, want 0", m.offset)
	}
}

// ---------------------------------------------------------------------------
// ApplySnapshot
// ---------------------------------------------------------------------------

func TestApplySnapshot_OverridesClaudeActive(t *testing.T) {
	sessions := []model.Session{
		{ID: "live-claude", Tool: model.ToolClaude, Active: false, UpdatedAt: time.Now()},
		{ID: "dead-claude", Tool: model.ToolClaude, Active: true, UpdatedAt: time.Now()},
		{ID: "any-cursor", Tool: model.ToolCursor, Active: true, UpdatedAt: time.Now()},
	}
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{{Tool: procsnap.ToolClaude, SessionID: "live-claude"}},
	}
	got := ApplySnapshot(sessions, snap)
	if !got[0].Active {
		t.Errorf("live claude must become Active=true")
	}
	if got[1].Active {
		t.Errorf("claude not in snapshot must become Active=false")
	}
	if !got[2].Active {
		t.Errorf("cursor Active must be untouched (was true)")
	}
}

func TestApplySnapshot_IgnoresCodexEntries(t *testing.T) {
	sessions := []model.Session{
		{ID: "shared-id", Tool: model.ToolClaude, Active: false, UpdatedAt: time.Now()},
	}
	// A codex entry in the snapshot must not mark a claude session active,
	// even on (theoretical) ID overlap.
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{{Tool: procsnap.ToolCodex, SessionID: "shared-id", Name: "x"}},
	}
	got := ApplySnapshot(sessions, snap)
	if got[0].Active {
		t.Error("codex snapshot entry must not activate a claude session")
	}
	if got[0].Title != "" {
		t.Errorf("codex entry Name must not cascade into claude Title, got %q", got[0].Title)
	}
}

func TestApplySnapshot_EmptySnapshotZeroesClaude(t *testing.T) {
	sessions := []model.Session{
		{ID: "x", Tool: model.ToolClaude, Active: true, UpdatedAt: time.Now()},
	}
	got := ApplySnapshot(sessions, procsnap.Snapshot{})
	// Empty snapshot overrides: claude session becomes inactive.
	// This matches "we believe the snapshot when we have it"; callers that
	// receive ErrUnsupported must not call ApplySnapshot.
	if got[0].Active {
		t.Error("empty snapshot must mark claude sessions inactive")
	}
}

func TestModel_LineageOverlay_ToggleAndDismiss(t *testing.T) {
	sessions := []model.Session{
		{ID: "aaa", Tool: model.ToolClaude, UpdatedAt: time.Now(), Active: true},
	}
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{{
			Tool:      procsnap.ToolClaude,
			SessionID: "aaa",
			PID:       1234,
			Ancestors: []procsnap.Ancestor{
				{PID: 100, Command: "zsh"},
				{PID: 1, Command: "launchd"},
			},
		}},
	}
	m := New(sessions, nil)
	m.SetSnapshot(snap)

	// Press 'l' — overlay becomes visible.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	view := m2.View()
	if !strings.Contains(view, "Lineage") || !strings.Contains(view, "zsh") || !strings.Contains(view, "launchd") {
		t.Errorf("expected lineage overlay with ancestors, got:\n%s", view)
	}

	// Press Esc — overlay dismissed.
	m3, _ := m2.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	view2 := m3.View()
	if strings.Contains(view2, "Lineage") {
		t.Errorf("overlay must dismiss on Esc, still shown:\n%s", view2)
	}
}

func TestRenderLineage_NonClaude(t *testing.T) {
	m := New([]model.Session{{ID: "x", Tool: model.ToolCursor, UpdatedAt: time.Now()}}, nil)
	m.showingLineage = true
	view := m.View()
	if !strings.Contains(view, "not a Claude session") {
		t.Errorf("expected non-claude notice, got:\n%s", view)
	}
}

func TestRenderLineage_NoLiveProcess(t *testing.T) {
	m := New([]model.Session{{ID: "x", Tool: model.ToolClaude, UpdatedAt: time.Now()}}, nil)
	m.showingLineage = true
	view := m.View()
	if !strings.Contains(view, "no live process") {
		t.Errorf("expected no-live-process notice, got:\n%s", view)
	}
}

func TestApplySnapshot_PopulatesRenameTitle(t *testing.T) {
	sessions := []model.Session{
		{ID: "aaa", Tool: model.ToolClaude, Title: "", UpdatedAt: time.Now()},
		{ID: "bbb", Tool: model.ToolClaude, Title: "existing preview", UpdatedAt: time.Now()},
	}
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{
			{Tool: procsnap.ToolClaude, SessionID: "aaa", Name: "pair with alice"},
			{Tool: procsnap.ToolClaude, SessionID: "bbb", Name: ""}, // no /rename
		},
	}
	got := ApplySnapshot(sessions, snap)
	if got[0].Title != "pair with alice" {
		t.Errorf("Title = %q, want %q", got[0].Title, "pair with alice")
	}
	if got[1].Title != "existing preview" {
		t.Errorf("empty rename must not overwrite existing Title, got %q", got[1].Title)
	}
}

// ---------------------------------------------------------------------------
// Snapshot tick
// ---------------------------------------------------------------------------

func TestModel_SnapshotTick_UpdatesSnapshot(t *testing.T) {
	sessions := []model.Session{
		{ID: "aaa", Tool: model.ToolClaude, UpdatedAt: time.Now()},
	}
	m := New(sessions, nil)

	// Inject a fake enumerator.
	calls := 0
	m.SetEnumerator(func() (procsnap.Snapshot, error) {
		calls++
		return procsnap.Snapshot{Sessions: []procsnap.Session{{Tool: procsnap.ToolClaude, SessionID: "aaa"}}}, nil
	})

	// Simulate the tick message delivery.
	m2, _ := m.Update(snapshotTickMsg{})
	mm := m2.(Model)
	if calls != 1 {
		t.Errorf("expected enumerator to be called once, got %d", calls)
	}
	if !mm.snapshot.IsActive("aaa") {
		t.Error("snapshot must have been stored")
	}
}

func TestModel_SnapshotTick_NoEnumerator(t *testing.T) {
	m := New([]model.Session{{ID: "x", Tool: model.ToolClaude, UpdatedAt: time.Now()}}, nil)
	m2, cmd := m.Update(snapshotTickMsg{})
	if cmd != nil {
		t.Error("nil enumerator must not schedule next tick")
	}
	_ = m2
}

func TestModel_SnapshotTick_EnumeratorError(t *testing.T) {
	m := New([]model.Session{{ID: "x", Tool: model.ToolClaude, UpdatedAt: time.Now()}}, nil)
	m.SetEnumerator(func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, errors.New("boom")
	})
	m2, cmd := m.Update(snapshotTickMsg{})
	if cmd == nil {
		t.Error("even on error, tick must reschedule")
	}
	_ = m2
}

func TestModel_Init_SchedulesFirstTick(t *testing.T) {
	m := New(nil, nil)
	if m.Init() == nil {
		t.Error("Init must return a tick command")
	}
}

func TestModel_SnapshotTick_RescheduleCmd(t *testing.T) {
	m := New([]model.Session{{ID: "x", Tool: model.ToolClaude, UpdatedAt: time.Now()}}, nil)
	m.SetEnumerator(func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, nil
	})
	_, cmd := m.Update(snapshotTickMsg{})
	if cmd == nil {
		t.Fatal("expected reschedule cmd")
	}
}

func TestSnapshotTickCallback(t *testing.T) {
	msg := snapshotTickCallback(time.Time{})
	if _, ok := msg.(snapshotTickMsg); !ok {
		t.Errorf("snapshotTickCallback must produce snapshotTickMsg, got %T", msg)
	}
}
