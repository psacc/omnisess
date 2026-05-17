package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
	"github.com/spf13/cobra"
)

// digestMaxTurnChars is the per-turn truncation limit for digest output.
const digestMaxTurnChars = 2000

var digestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Print sessions with full Q&A content as Obsidian-compatible markdown",
	RunE:  runDigest,
}

func init() {
	rootCmd.AddCommand(digestCmd)
}

func runDigest(_ *cobra.Command, _ []string) error {
	return runDigestTo(os.Stdout)
}

// runDigestTo is the testable inner of runDigest. It writes the rendered digest
// to w so tests can capture and assert on the output.
func runDigestTo(w io.Writer) error {
	srcs := getSources()
	opts := getListOptions()
	// Default to today when no time filter is given — digest is a daily-note helper.
	if opts.OnDate.IsZero() && opts.Since == 0 {
		d, _ := time.ParseInLocation("2006-01-02", time.Now().Format("2006-01-02"), time.Local)
		opts.OnDate = d
	}

	var all []model.Session
	for _, s := range srcs {
		sessions, err := s.List(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", s.Name(), err)
			continue
		}
		all = append(all, sessions...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].StartedAt.Before(all[j].StartedAt)
	})

	if opts.Limit > 0 && len(all) > opts.Limit {
		all = all[:opts.Limit]
	}

	srcByTool := make(map[model.Tool]source.Source, len(srcs))
	for _, s := range srcs {
		srcByTool[s.Name()] = s
	}

	dateLabel := flagDate
	if dateLabel == "" {
		dateLabel = time.Now().Format("2006-01-02")
	}

	writeDigest(w, all, srcByTool, dateLabel)
	return nil
}

func writeDigest(w io.Writer, sessions []model.Session, srcByTool map[model.Tool]source.Source, dateLabel string) {
	fmt.Fprintf(w, "## AI sessions — %s  (%d sessions)\n\n", dateLabel, len(sessions))
	if len(sessions) == 0 {
		return
	}

	rendered := 0
	for _, sess := range sessions {
		src, ok := srcByTool[sess.Tool]
		if !ok {
			continue
		}
		full, err := src.Get(sess.ID)
		if err != nil || full == nil {
			continue
		}
		if rendered > 0 {
			fmt.Fprintln(w, "---")
		}
		writeDigestSession(w, full)
		rendered++
	}
}

func writeDigestSession(w io.Writer, s *model.Session) {
	title := s.Preview
	if title == "" {
		title = s.ID
	}
	if len(title) > 80 {
		title = title[:80]
	}

	project := s.Project
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(project, home) {
		project = "~" + project[len(home):]
	}

	fmt.Fprintf(w, "### %s\n", title)
	fmt.Fprintf(w, "_project: `%s` · tool: %s_\n\n", project, s.Tool)

	for _, m := range s.Messages {
		content := strings.TrimSpace(m.Content)
		ts := ""
		if !m.Timestamp.IsZero() {
			ts = " [" + m.Timestamp.Local().Format("15:04") + "]"
		}
		switch m.Role {
		case model.RoleUser:
			if content == "" {
				continue
			}
			if len(content) > digestMaxTurnChars {
				content = content[:digestMaxTurnChars] + "\n\n_(truncated)_"
			}
			fmt.Fprintf(w, "**Q%s:** %s\n\n", ts, content)
		case model.RoleAssistant:
			if content == "" {
				if len(m.ToolCalls) > 0 {
					names := make([]string, len(m.ToolCalls))
					for j, tc := range m.ToolCalls {
						names[j] = tc.Name
					}
					fmt.Fprintf(w, "_[tools: %s]_\n\n", strings.Join(names, ", "))
				}
				continue
			}
			if len(content) > digestMaxTurnChars {
				content = content[:digestMaxTurnChars] + "\n\n_(truncated)_"
			}
			fmt.Fprintf(w, "**A%s:**\n%s\n\n", ts, content)
		}
	}
}
