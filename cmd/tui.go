//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/tui"
)

const defaultTUILimit = 50

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive session picker",
	Long:  "Browse sessions interactively. Select a Claude session to resume it.",
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	sources := getSources()
	opts := getListOptions()

	// Apply default limit if none specified.
	if opts.Limit == 0 {
		opts.Limit = defaultTUILimit
	}

	var all []model.Session
	for _, s := range sources {
		sessions, err := s.List(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", s.Name(), err)
			continue
		}
		all = append(all, sessions...)
	}

	// Sort by UpdatedAt descending.
	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	// Apply limit after merging all sources.
	if opts.Limit > 0 && len(all) > opts.Limit {
		all = all[:opts.Limit]
	}

	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "No sessions found.")
		return nil
	}

	// Run Bubble Tea program.
	m := tui.New(all)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	result := finalModel.(tui.Model)
	sess := result.Selected()
	if sess == nil {
		return nil // user quit without selecting
	}

	// Resume the selected Claude session from its project directory.
	return resumeClaude(sess.ID, sess.Project)
}

// resumeClaude replaces the current process with "claude --resume <id>".
// It changes to the session's project directory first so Claude Code can find it.
func resumeClaude(sessionID, projectDir string) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}

	if projectDir != "" {
		if err := os.Chdir(projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not chdir to %s: %v\n", projectDir, err)
		}
	}

	argv := []string{"claude", "--resume", sessionID}
	return syscall.Exec(claudePath, argv, os.Environ())
}
