package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/psacc/omnisess/internal/model"
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
