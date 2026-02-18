package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/psacconier/sessions/internal/model"
)

// sanitizeString strips C0 control characters (U+0000-U+001F) that are not
// tab, newline, or carriage return. These characters have no semantic value
// in session content (ANSI escapes, null bytes, bell, etc.) and can cause
// issues with strict JSON parsers in downstream consumers like Python.
//
// NOTE: DEL (U+007F) and C1 controls (U+0080-U+009F) are left as-is;
// extend here if downstream parsers choke on those too.
func sanitizeString(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return -1 // drop the rune
		}
		return r
	}, s)
}

// sanitizeSession returns a deep copy of the session with all string
// fields sanitized for safe JSON output. The original session is not modified.
func sanitizeSession(s *model.Session) model.Session {
	out := *s
	out.Title = sanitizeString(out.Title)
	out.Summary = sanitizeString(out.Summary)
	out.Preview = sanitizeString(out.Preview)
	out.Project = sanitizeString(out.Project)
	out.Branch = sanitizeString(out.Branch)
	out.Model = sanitizeString(out.Model)

	if len(s.Messages) > 0 {
		out.Messages = make([]model.Message, len(s.Messages))
		for i, m := range s.Messages {
			out.Messages[i] = model.Message{
				Role:      m.Role,
				Content:   sanitizeString(m.Content),
				Timestamp: m.Timestamp,
			}
			if len(m.ToolCalls) > 0 {
				out.Messages[i].ToolCalls = make([]model.ToolCall, len(m.ToolCalls))
				for j, tc := range m.ToolCalls {
					out.Messages[i].ToolCalls[j] = model.ToolCall{
						Name:   sanitizeString(tc.Name),
						Input:  sanitizeString(tc.Input),
						Output: sanitizeString(tc.Output),
					}
				}
			}
		}
	}

	return out
}

// sanitizeSearchResults returns a sanitized copy of search results for
// safe JSON output.
func sanitizeSearchResults(results []model.SearchResult) []model.SearchResult {
	out := make([]model.SearchResult, len(results))
	for i, r := range results {
		out[i] = model.SearchResult{
			Session: sanitizeSession(&r.Session),
			Matches: make([]model.SearchMatch, len(r.Matches)),
		}
		for j, m := range r.Matches {
			out[i].Matches[j] = model.SearchMatch{
				MessageIndex: m.MessageIndex,
				Snippet:      sanitizeString(m.Snippet),
				Role:         m.Role,
			}
		}
	}
	return out
}

// sanitizeSessions returns a sanitized copy of sessions for safe JSON output.
func sanitizeSessions(sessions []model.Session) []model.Session {
	out := make([]model.Session, len(sessions))
	for i := range sessions {
		out[i] = sanitizeSession(&sessions[i])
	}
	return out
}

// Format represents the output format.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

// RenderSessions outputs a list of sessions in the given format.
func RenderSessions(sessions []model.Session, format Format) {
	switch format {
	case FormatJSON:
		renderJSON(os.Stdout, sanitizeSessions(sessions))
	default:
		renderTable(os.Stdout, sessions)
	}
}

// RenderSession outputs a single session with full message history.
func RenderSession(session *model.Session, format Format) {
	switch format {
	case FormatJSON:
		sanitized := sanitizeSession(session)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(sanitized)
	default:
		renderSessionDetail(os.Stdout, session)
	}
}

// RenderSearchResults outputs search results in the given format.
func RenderSearchResults(results []model.SearchResult, format Format) {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(sanitizeSearchResults(results))
	default:
		renderSearchTable(os.Stdout, results)
	}
}

func renderTable(w io.Writer, sessions []model.Session) {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No sessions found.")
		return
	}

	// Header
	fmt.Fprintf(w, "%-8s %-28s %-18s %-50s %-18s %s\n",
		"TOOL", "PROJECT", "BRANCH", "PREVIEW", "STARTED", "STATUS")
	fmt.Fprintln(w, strings.Repeat("-", 140))

	for _, s := range sessions {
		status := "-"
		if s.Active {
			status = "ACTIVE"
		}
		branch := truncate(s.Branch, 16)
		project := truncate(s.ShortProject(), 26)
		preview := truncate(s.Preview, 48)
		started := s.StartedAt.Local().Format("2006-01-02 15:04")

		fmt.Fprintf(w, "%-8s %-28s %-18s %-50s %-18s %s\n",
			s.Tool, project, branch, preview, started, status)
	}
}

func renderSessionDetail(w io.Writer, s *model.Session) {
	fmt.Fprintf(w, "Session: %s (%s)\n", s.ShortID(), s.Tool)
	fmt.Fprintf(w, "Project: %s\n", s.Project)
	if s.Branch != "" {
		fmt.Fprintf(w, "Branch:  %s\n", s.Branch)
	}
	if s.Model != "" {
		fmt.Fprintf(w, "Model:   %s\n", s.Model)
	}
	fmt.Fprintf(w, "Started: %s\n", s.StartedAt.Local().Format("2006-01-02 15:04:05"))
	if s.Active {
		fmt.Fprintf(w, "Status:  ACTIVE\n")
	}
	fmt.Fprintln(w)

	for _, m := range s.Messages {
		ts := m.Timestamp.Local().Format("15:04:05")
		fmt.Fprintf(w, "--- [%s] %s ---\n", m.Role, ts)
		fmt.Fprintln(w, m.Content)
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(w, "  [tool: %s]\n", tc.Name)
		}
		fmt.Fprintln(w)
	}
}

func renderSearchTable(w io.Writer, results []model.SearchResult) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No matches found.")
		return
	}

	for _, r := range results {
		fmt.Fprintf(w, "%s  %-28s  %s\n",
			r.Session.QualifiedID(),
			r.Session.ShortProject(),
			r.Session.StartedAt.Local().Format("2006-01-02"))

		for _, m := range r.Matches {
			fmt.Fprintf(w, "  [%s] %s\n", m.Role, m.Snippet)
		}
		fmt.Fprintln(w)
	}
}

func renderJSON(w io.Writer, v interface{}) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// FormatDuration returns a human-readable duration like "2h", "3d", "1w".
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours < 1 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	weeks := days / 7
	return fmt.Sprintf("%dw", weeks)
}
