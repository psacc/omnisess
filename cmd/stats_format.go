package cmd

import (
	"fmt"
	"io"

	"github.com/psacc/omnisess/internal/index"
)

// writeSessionStatsText renders a per-session stats block to w.
func writeSessionStatsText(w io.Writer, s *index.SessionStats) {
	fmt.Fprintf(w, "Session: %s\n", s.ConversationID)
	fmt.Fprintf(w, "Provider: %s   Model: %s\n", emptyDash(s.ProviderName), emptyDash(s.RequestModel))
	if !s.StartedAt.IsZero() {
		fmt.Fprintf(w, "Started: %s\n", s.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	if !s.UpdatedAt.IsZero() {
		fmt.Fprintf(w, "Updated: %s\n", s.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(w, "Tokens:  input=%d output=%d cache-create=%d cache-read=%d\n",
		s.TotalInputTokens, s.TotalOutputTokens,
		s.TotalCacheCreateTokens, s.TotalCacheReadTokens)
	fmt.Fprintf(w, "Tool calls: %d (errors: %d)\n\n", s.ToolCallCount, s.ErrorCount)

	if len(s.Tools) > 0 {
		fmt.Fprintln(w, "Tools")
		fmt.Fprintln(w, "-----")
		fmt.Fprintf(w, "%-40s %8s %8s\n", "name", "count", "errors")
		for _, t := range s.Tools {
			fmt.Fprintf(w, "%-40s %8d %8d\n", t.Name, t.Count, t.ErrorCount)
		}
		fmt.Fprintln(w)
	}

	if len(s.Files) > 0 {
		fmt.Fprintln(w, "Files")
		fmt.Fprintln(w, "-----")
		fmt.Fprintf(w, "%-60s %6s %6s %6s %6s %6s\n", "path", "reads", "writes", "edits", "+", "-")
		for _, f := range s.Files {
			fmt.Fprintf(w, "%-60s %6d %6d %6d %6d %6d\n",
				f.Path, f.Reads, f.Writes, f.Edits, f.LinesAdded, f.LinesRemoved)
		}
		fmt.Fprintln(w)
	}
}

// writeAggregateStatsText renders a window-aggregate block to w.
func writeAggregateStatsText(w io.Writer, a *index.AggregateStats, windowStr string) {
	fmt.Fprintf(w, "Window: %s (%s → %s)\n",
		windowStr,
		a.WindowStart.Local().Format("2006-01-02 15:04"),
		a.WindowEnd.Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(w, "Sessions: %d   Tool calls: %d (errors: %d)\n",
		a.SessionCount, a.ToolCallCount, a.ErrorCount)
	fmt.Fprintf(w, "Tokens:   input=%d output=%d cache-create=%d cache-read=%d\n\n",
		a.TotalInputTokens, a.TotalOutputTokens,
		a.TotalCacheCreateTokens, a.TotalCacheReadTokens)

	if len(a.Tools) > 0 {
		fmt.Fprintln(w, "Tools")
		fmt.Fprintln(w, "-----")
		fmt.Fprintf(w, "%-40s %8s %8s\n", "name", "count", "errors")
		for _, t := range a.Tools {
			fmt.Fprintf(w, "%-40s %8d %8d\n", t.Name, t.Count, t.ErrorCount)
		}
		fmt.Fprintln(w)
	}

	if len(a.Files) > 0 {
		fmt.Fprintln(w, "Files")
		fmt.Fprintln(w, "-----")
		fmt.Fprintf(w, "%-60s %6s %6s %6s %6s %6s\n", "path", "reads", "writes", "edits", "+", "-")
		for _, f := range a.Files {
			fmt.Fprintf(w, "%-60s %6d %6d %6d %6d %6d\n",
				f.Path, f.Reads, f.Writes, f.Edits, f.LinesAdded, f.LinesRemoved)
		}
		fmt.Fprintln(w)
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
