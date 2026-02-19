package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/output"
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
	sessions []model.Session
	cursor   int
	offset   int // scroll offset for viewport
	width    int
	height   int
	selected *model.Session
	quitting bool
	message  string // inline error/info message
}

// New creates a Model pre-loaded with sessions.
func New(sessions []model.Session) Model {
	return Model{
		sessions: sessions,
		width:    80,
		height:   24,
	}
}

// Selected returns the session the user picked, or nil if they quit.
func (m Model) Selected() *model.Session {
	return m.selected
}

// Quitting returns true if the user chose to exit.
func (m Model) Quitting() bool {
	return m.quitting
}

// Init implements tea.Model. Data is pre-loaded, so no initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampViewport()
		return m, nil

	case tea.KeyMsg:
		// Clear any inline message on next keypress.
		m.message = ""

		switch msg.String() {
		case "q", "esc", "ctrl+c":
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
			if len(m.sessions) == 0 {
				return m, nil
			}
			sess := m.sessions[m.cursor]
			if sess.Tool != model.ToolClaude {
				m.message = fmt.Sprintf("resume not supported for %s", sess.Tool)
				return m, nil
			}
			m.selected = &sess
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
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

	// Footer
	footer := "j/k: navigate  enter: resume  q: quit"
	b.WriteString(styleFooter.Render(footer))
	b.WriteByte('\n')

	return b.String()
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
	if m.offset < 0 {
		m.offset = 0
	}
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
