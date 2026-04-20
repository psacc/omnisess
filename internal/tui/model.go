package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/psacc/omnisess/internal/procsnap"
)

// Column widths (fixed layout).
const (
	colTool    = 8
	colProject = 26
	colPreview = 0 // dynamic: fills remaining space
	colTime    = 6
	colStatus  = 6

	// Lines reserved for header + column headers + footer.
	chromeLines = 4
)

// Styles.
var (
	styleSelected = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleActive   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleHeader   = lipgloss.NewStyle().Bold(true)
	styleFooter   = lipgloss.NewStyle().Faint(true)
	styleMessage  = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
)

// Model is the Bubble Tea model for the session picker TUI.
type Model struct {
	sessions       []model.Session
	cursor         int
	offset         int // scroll offset for viewport
	width          int
	height         int
	selected       *model.Session
	selectedMode   string // resume mode chosen by user (e.g. "resume", "tmux", "aoe", "fork")
	quitting       bool
	message        string // inline error/info message
	toolModes      map[model.Tool][]string
	snapshot       procsnap.Snapshot
	showingLineage bool
	enumerate      func() (procsnap.Snapshot, error)
}

// New creates a Model pre-loaded with sessions.
// toolModes maps each tool to its available mode strings (e.g. "resume", "fork", "tmux", "aoe").
// Passing nil means only "aoe" is universally available.
func New(sessions []model.Session, toolModes map[model.Tool][]string) Model {
	if toolModes == nil {
		toolModes = map[model.Tool][]string{}
	}
	return Model{
		sessions:  sessions,
		width:     80,
		height:    24,
		toolModes: toolModes,
	}
}

// SetSnapshot attaches a snapshot used by the lineage overlay and active flag.
func (m *Model) SetSnapshot(snap procsnap.Snapshot) {
	m.snapshot = snap
}

// snapshotTickMsg fires every 5 seconds to refresh the process snapshot.
type snapshotTickMsg struct{}

const snapshotTickInterval = 5 * time.Second

// SetEnumerator injects a procsnap enumerator. Callers provide this
// before launching the program so the TUI can refresh. When nil, tick
// messages are no-ops.
func (m *Model) SetEnumerator(e func() (procsnap.Snapshot, error)) {
	m.enumerate = e
}

// Selected returns the session the user picked, or nil if they quit.
func (m Model) Selected() *model.Session {
	return m.selected
}

// SelectedMode returns the resume mode string chosen by the user.
// Empty string if no selection was made. Values: "resume", "tmux", "aoe", "fork".
func (m Model) SelectedMode() string {
	return m.selectedMode
}

// Quitting returns true if the user chose to exit.
func (m Model) Quitting() bool {
	return m.quitting
}

// ApplySnapshot overrides the Active flag for every claude session based on
// the snapshot, and cascades the live /rename Name into Title when non-empty.
// Non-claude sessions are untouched. Caller must only invoke this when
// Enumerate returned nil error (never on ErrUnsupported).
func ApplySnapshot(sessions []model.Session, snap procsnap.Snapshot) []model.Session {
	bySessionID := make(map[string]procsnap.Session, len(snap.Sessions))
	for _, s := range snap.Sessions {
		bySessionID[s.SessionID] = s
	}
	for i := range sessions {
		if sessions[i].Tool != model.ToolClaude {
			continue
		}
		live, ok := bySessionID[sessions[i].ID]
		sessions[i].Active = ok
		if ok && live.Name != "" {
			sessions[i].Title = live.Name
		}
	}
	return sessions
}

// snapshotTickCallback is the tea.Tick callback for the snapshot refresh.
// Extracted from the inline closure so tests can cover it without waiting
// the full tick interval for tea.Tick to fire.
func snapshotTickCallback(time.Time) tea.Msg {
	return snapshotTickMsg{}
}

// Init implements tea.Model. Schedules the first snapshot refresh tick.
func (m Model) Init() tea.Cmd {
	return tea.Tick(snapshotTickInterval, snapshotTickCallback)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampViewport()
		return m, nil

	case snapshotTickMsg:
		if m.enumerate == nil {
			return m, nil
		}
		if snap, err := m.enumerate(); err == nil {
			m.snapshot = snap
			m.sessions = ApplySnapshot(m.sessions, snap)
		}
		return m, tea.Tick(snapshotTickInterval, snapshotTickCallback)

	case tea.KeyMsg:
		// Clear any inline message on next keypress.
		m.message = ""

		switch msg.String() {
		case "l":
			m.showingLineage = true
			m.message = ""
			return m, nil
		case "esc":
			if m.showingLineage {
				m.showingLineage = false
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.clampViewport()
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
				m.clampViewport()
			}
			return m, nil

		case "enter":
			return m.selectWithMode("resume")

		case "t":
			return m.selectWithMode("tmux")

		case "a":
			return m.selectWithMode("aoe")

		case "f":
			return m.selectWithMode("fork")

		case "o":
			return m.selectWithMode("open")
		}
	}

	return m, nil
}

// selectWithMode attempts to select the current session with the given mode.
// If no sessions exist or the mode is not available for the tool, it sets an
// inline message instead.
func (m Model) selectWithMode(mode string) (tea.Model, tea.Cmd) {
	if len(m.sessions) == 0 {
		return m, nil
	}

	sess := m.sessions[m.cursor]

	if !m.hasModeForTool(sess.Tool, mode) {
		m.message = fmt.Sprintf("%s not supported for %s", mode, sess.Tool)
		return m, nil
	}

	m.selected = &sess
	m.selectedMode = mode
	m.quitting = true
	return m, tea.Quit
}

// hasModeForTool checks whether the given mode is available for the tool.
// "aoe" and "open" are always available (they do not depend on the resumer).
func (m Model) hasModeForTool(tool model.Tool, mode string) bool {
	if mode == "aoe" || mode == "open" {
		return true
	}
	modes := m.toolModes[tool]
	for _, available := range modes {
		if available == mode {
			return true
		}
	}
	return false
}

// View implements tea.Model.
func (m Model) View() string {
	if len(m.sessions) == 0 {
		return "No sessions found.\n"
	}

	var b strings.Builder

	// Header: "Sessions (X active)"
	activeCount := 0
	for _, s := range m.sessions {
		if s.Active {
			activeCount++
		}
	}
	header := fmt.Sprintf("Sessions (%d active)", activeCount)
	b.WriteString(styleHeader.Render(header))
	b.WriteByte('\n')

	// Column headers
	previewWidth := m.previewWidth()
	colHeader := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
		colTool, "TOOL",
		colProject, "PROJECT",
		previewWidth, "PREVIEW",
		colTime, "AGO",
		"STATUS")
	b.WriteString(styleFooter.Render(colHeader))
	b.WriteByte('\n')

	// Session rows
	visibleRows := m.visibleRows()
	end := m.offset + visibleRows
	if end > len(m.sessions) {
		end = len(m.sessions)
	}

	for i := m.offset; i < end; i++ {
		row := m.renderRow(i, previewWidth)
		if i == m.cursor {
			row = styleSelected.Render(row)
		}
		b.WriteString(row)
		b.WriteByte('\n')
	}

	// Inline message (if any)
	if m.message != "" {
		b.WriteString(styleMessage.Render(m.message))
		b.WriteByte('\n')
	}

	// Footer — dynamic based on selected session's tool
	footer := m.footerHelp()
	b.WriteString(styleFooter.Render(footer))
	b.WriteByte('\n')

	if m.showingLineage && len(m.sessions) > 0 {
		b.WriteString(m.renderLineage())
	}

	return b.String()
}

// footerHelp returns the keybinding help line for the currently selected session's tool.
func (m Model) footerHelp() string {
	var parts []string
	parts = append(parts, "j/k: navigate")

	if len(m.sessions) > 0 {
		tool := m.sessions[m.cursor].Tool
		modes := m.toolModes[tool]

		// Build mode keybindings in a fixed order.
		modeSet := make(map[string]bool, len(modes))
		for _, mode := range modes {
			modeSet[mode] = true
		}

		if modeSet["resume"] {
			parts = append(parts, "enter: resume")
		}
		if modeSet["tmux"] {
			parts = append(parts, "t: tmux")
		}
		// AoE and open are always available, no need to check modeSet.
		parts = append(parts, "a: aoe")
		parts = append(parts, "o: open")
		if modeSet["fork"] {
			parts = append(parts, "f: fork")
		}
	}

	parts = append(parts, "q: quit")
	return strings.Join(parts, "  ")
}

// renderLineage renders the lineage overlay for the currently selected session.
func (m Model) renderLineage() string {
	sess := m.sessions[m.cursor]
	if sess.Tool != model.ToolClaude {
		return styleFooter.Render("Lineage unavailable: not a Claude session.\n")
	}
	var live *procsnap.Session
	for i := range m.snapshot.Sessions {
		if m.snapshot.Sessions[i].SessionID == sess.ID {
			live = &m.snapshot.Sessions[i]
			break
		}
	}
	if live == nil {
		return styleFooter.Render("Lineage: session has no live process.\n")
	}
	var sb strings.Builder
	sb.WriteString(styleHeader.Render("Lineage"))
	sb.WriteByte('\n')
	fmt.Fprintf(&sb, "  claude (%d)\n", live.PID)
	for _, a := range live.Ancestors {
		fmt.Fprintf(&sb, "  └─ %s (%d)\n", a.Command, a.PID)
	}
	sb.WriteString(styleFooter.Render("(esc to dismiss)\n"))
	return sb.String()
}

// renderRow formats a single session row.
func (m Model) renderRow(idx, previewWidth int) string {
	s := m.sessions[idx]

	tool := truncatePad(string(s.Tool), colTool)
	project := truncatePad(s.ShortProject(), colProject)
	previewText := s.Preview
	if previewText == "" {
		previewText = s.QualifiedID()
	}
	preview := truncatePad(previewText, previewWidth)
	ago := truncatePad(output.FormatDuration(time.Since(s.UpdatedAt)), colTime)

	// Status indicator: pad to colStatus visible width for alignment with header.
	var status string
	if s.Active {
		status = styleActive.Render("*") + strings.Repeat(" ", colStatus-1)
	} else {
		status = strings.Repeat(" ", colStatus)
	}

	return fmt.Sprintf("  %s %s %s %s %s", tool, project, preview, ago, status)
}

// previewWidth computes the dynamic preview column width.
func (m Model) previewWidth() int {
	// Layout: indent(2) TOOL(8) sp PROJECT(26) sp PREVIEW(pw) sp AGO(6) sp STATUS(*)
	// STATUS is unpadded (last column, variable width: "  " or "* "), so not counted.
	fixed := 2 + colTool + 1 + colProject + 1 + 1 + colTime + 1 + colStatus
	pw := m.width - fixed
	if pw < 10 {
		pw = 10
	}
	return pw
}

// visibleRows returns how many session rows fit in the viewport.
func (m Model) visibleRows() int {
	extra := chromeLines
	if m.message != "" {
		extra++
	}
	rows := m.height - extra
	if rows < 1 {
		rows = 1
	}
	return rows
}

// clampViewport ensures the cursor is visible within the scrolled viewport.
func (m *Model) clampViewport() {
	visible := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	// offset is always ≥ 0 after clamp operations above.
}

// truncatePad truncates s to maxLen (with "..." suffix) and pads with spaces.
// Uses byte length, not rune count — safe for ASCII session data from AI tools.
func truncatePad(s string, maxLen int) string {
	if len(s) > maxLen {
		if maxLen > 3 {
			s = s[:maxLen-3] + "..."
		} else {
			s = s[:maxLen]
		}
	}
	return fmt.Sprintf("%-*s", maxLen, s)
}
