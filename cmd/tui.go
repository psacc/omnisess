//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/resume"
	"github.com/psacc/omnisess/internal/tui"

	// Register resumers via init() (behind !windows, same as this file).
	_ "github.com/psacc/omnisess/internal/resume/claude"
	_ "github.com/psacc/omnisess/internal/resume/cursor"
)

const defaultTUILimit = 50

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive session picker",
	Long:  "Browse sessions interactively. Select a session to resume, fork, or open in tmux/AoE.",
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

// buildToolModes queries the resume registry and returns a map of tool -> mode
// strings suitable for passing to the TUI (keeping the TUI decoupled from
// the resume package types).
func buildToolModes() map[model.Tool][]string {
	tools := []model.Tool{model.ToolClaude, model.ToolCursor, model.ToolCodex, model.ToolGemini}
	tm := make(map[model.Tool][]string)
	for _, tool := range tools {
		modes := resume.Modes(tool)
		if len(modes) == 0 {
			continue
		}
		strs := make([]string, len(modes))
		for i, m := range modes {
			strs[i] = string(m)
		}
		tm[tool] = strs
	}
	return tm
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
	toolModes := buildToolModes()
	m := tui.New(all, toolModes)
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

	mode := resume.Mode(result.SelectedMode())

	// AoE mode is handled directly (no resumer needed).
	if mode == resume.ModeAoE {
		title := sess.ShortProject() + " (" + string(sess.Tool) + ")"
		return resume.ExecInAoE(string(sess.Tool), sess.Project, title)
	}

	// Open mode: open project directory (no resumer needed).
	if mode == resume.ModeOpen {
		return openProjectDir(sess.Project)
	}

	// Look up the resumer for this tool.
	resumer, ok := resume.Get(sess.Tool)
	if !ok {
		return fmt.Errorf("resume not supported for %s (try 'o' to open project dir)", sess.Tool)
	}

	return resumer.Exec(sess, mode)
}

// openProjectDir opens a project directory using $EDITOR, falling back to
// "open" on macOS, and finally just printing the path.
func openProjectDir(dir string) error {
	// Prefer $EDITOR: exec into it with the directory as argument.
	if editor := os.Getenv("EDITOR"); editor != "" {
		editorPath, err := exec.LookPath(editor)
		if err != nil {
			return fmt.Errorf("$EDITOR=%q not found: %w", editor, err)
		}
		return syscall.Exec(editorPath, []string{editor, dir}, os.Environ())
	}

	// macOS: use "open" to reveal in Finder / default handler.
	if runtime.GOOS == "darwin" {
		openPath, err := exec.LookPath("open")
		if err == nil {
			return syscall.Exec(openPath, []string{"open", dir}, os.Environ())
		}
	}

	// Last resort: print path so the user can cd into it.
	fmt.Println(dir)
	return nil
}
