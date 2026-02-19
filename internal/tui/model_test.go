package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/psacconier/sessions/internal/model"
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
		keys        []tea.Msg // sequence of messages to send
		wantCursor  int
		wantSelect  bool       // expect selected != nil
		wantTool    model.Tool // expected selected session's tool (if wantSelect)
		wantQuit    bool
		wantMessage string // expected inline message
	}{
		{
			name:       "down moves cursor",
			sessions:   testSessions(),
			keys:       []tea.Msg{keyMsg("j")},
			wantCursor: 1,
		},
		{
			name:       "up moves cursor",
			sessions:   testSessions(),
			keys:       []tea.Msg{keyMsg("j"), keyMsg("j"), keyMsg("k")},
			wantCursor: 1,
		},
		{
			name:       "down arrow moves cursor",
			sessions:   testSessions(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyDown)},
			wantCursor: 1,
		},
		{
			name:       "up arrow moves cursor",
			sessions:   testSessions(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyDown), specialKeyMsg(tea.KeyUp)},
			wantCursor: 0,
		},
		{
			name:       "cursor clamps at top",
			sessions:   testSessions(),
			keys:       []tea.Msg{keyMsg("k"), keyMsg("k"), keyMsg("k")},
			wantCursor: 0,
		},
		{
			name:       "cursor clamps at bottom",
			sessions:   testSessions(),
			keys:       []tea.Msg{keyMsg("j"), keyMsg("j"), keyMsg("j"), keyMsg("j"), keyMsg("j")},
			wantCursor: 2, // 3 sessions, max index = 2
		},
		{
			name:       "enter on claude session selects",
			sessions:   testSessions(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEnter)},
			wantCursor: 0,
			wantSelect: true,
			wantTool:   model.ToolClaude,
			wantQuit:   true,
		},
		{
			name:        "enter on non-claude session shows message",
			sessions:    testSessions(),
			keys:        []tea.Msg{keyMsg("j"), specialKeyMsg(tea.KeyEnter)},
			wantCursor:  1,
			wantSelect:  false,
			wantQuit:    false,
			wantMessage: "resume not supported for cursor",
		},
		{
			name:       "q quits without selection",
			sessions:   testSessions(),
			keys:       []tea.Msg{keyMsg("q")},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   true,
		},
		{
			name:       "esc quits without selection",
			sessions:   testSessions(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEsc)},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   true,
		},
		{
			name:       "ctrl+c quits without selection",
			sessions:   testSessions(),
			keys:       []tea.Msg{specialKeyMsg(tea.KeyCtrlC)},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   true,
		},
		{
			name:       "enter on empty sessions does nothing",
			sessions:   nil,
			keys:       []tea.Msg{specialKeyMsg(tea.KeyEnter)},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   false,
		},
		{
			name:       "down on empty sessions does nothing",
			sessions:   nil,
			keys:       []tea.Msg{keyMsg("j")},
			wantCursor: 0,
			wantSelect: false,
			wantQuit:   false,
		},
		{
			name:       "message clears on next keypress",
			sessions:   testSessions(),
			keys:       []tea.Msg{keyMsg("j"), specialKeyMsg(tea.KeyEnter), keyMsg("j")},
			wantCursor: 2,
			wantSelect: false,
			wantQuit:   false,
			// The message from enter on cursor session should be cleared by the subsequent "j"
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.sessions)

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
	m := New(testSessions())

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
	m := New(nil)
	view := m.View()

	if !strings.Contains(view, "No sessions") {
		t.Errorf("empty view should contain 'No sessions', got: %q", view)
	}
}

func TestViewContainsSessionInfo(t *testing.T) {
	m := New(testSessions())
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

	m := New(sessions)
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
		m := New(sessions)
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
	m := New(testSessions())
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
