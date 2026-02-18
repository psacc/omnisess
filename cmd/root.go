package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/output"
	"github.com/psacconier/sessions/internal/source"
	"github.com/spf13/cobra"

	// Register all sources via init()
	_ "github.com/psacconier/sessions/internal/source/claude"
	_ "github.com/psacconier/sessions/internal/source/codex"
	_ "github.com/psacconier/sessions/internal/source/cursor"
	_ "github.com/psacconier/sessions/internal/source/gemini"
)

var (
	flagJSON    bool
	flagTool    string
	flagSince   string
	flagLimit   int
	flagProject string
)

var rootCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Aggregate AI coding sessions across tools",
	Long:  "Search, list, and monitor AI coding sessions from Claude Code, Cursor, Codex, and Gemini.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().StringVar(&flagTool, "tool", "", "Filter by tool (claude, cursor, codex, gemini)")
	rootCmd.PersistentFlags().StringVar(&flagSince, "since", "", "Only sessions updated within duration (e.g., 24h, 7d, 2w)")
	rootCmd.PersistentFlags().IntVar(&flagLimit, "limit", 0, "Max results (0 = unlimited)")
	rootCmd.PersistentFlags().StringVar(&flagProject, "project", "", "Filter by project path substring")
}

func getFormat() output.Format {
	if flagJSON {
		return output.FormatJSON
	}
	return output.FormatTable
}

func getSources() []source.Source {
	if flagTool != "" {
		return source.ByName(model.Tool(flagTool))
	}
	return source.All()
}

func getListOptions() source.ListOptions {
	opts := source.ListOptions{
		Limit:   flagLimit,
		Project: flagProject,
	}
	if flagSince != "" {
		d, err := parseDuration(flagSince)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --since value: %s\n", err)
			os.Exit(1)
		}
		opts.Since = d
	}
	return opts
}

// parseDuration handles Go durations plus "d" (days) and "w" (weeks).
func parseDuration(s string) (time.Duration, error) {
	// Try standard Go duration first
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}

	// Handle "Nd" and "Nw"
	var n int
	if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if _, err := fmt.Sscanf(s, "%dw", &n); err == nil {
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}

	return 0, fmt.Errorf("cannot parse %q (use Go durations, Nd for days, or Nw for weeks)", s)
}
