package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/psacc/omnisess/internal/source"
	"github.com/spf13/cobra"

	// Register all sources via init()
	_ "github.com/psacc/omnisess/internal/source/claude"
	_ "github.com/psacc/omnisess/internal/source/codex"
	_ "github.com/psacc/omnisess/internal/source/cursor"
	_ "github.com/psacc/omnisess/internal/source/gemini"
)

var (
	flagJSON            bool
	flagTool            string
	flagSince           string
	flagDate            string
	flagLimit           int
	flagProject         string
	flagExcludeProjects []string
)

var rootCmd = &cobra.Command{
	Use:   "omnisess",
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
	rootCmd.PersistentFlags().StringVar(&flagDate, "date", "", "Only sessions updated on this calendar day (YYYY-MM-DD, local time). Combines with --since by intersection.")
	rootCmd.PersistentFlags().IntVar(&flagLimit, "limit", 0, "Max results (0 = unlimited)")
	rootCmd.PersistentFlags().StringVar(&flagProject, "project", "", "Filter by project path substring")
	rootCmd.PersistentFlags().StringSliceVar(&flagExcludeProjects, "exclude-project", nil,
		"Exclude sessions matching project path substring (repeatable, also OMNISESS_EXCLUDE_PROJECTS env var)")
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
	if flagDate != "" {
		d, err := time.ParseInLocation("2006-01-02", flagDate, time.Local)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --date value: %s (expected YYYY-MM-DD)\n", err)
			os.Exit(1)
		}
		opts.OnDate = d
	}

	// Merge --exclude-project flag with OMNISESS_EXCLUDE_PROJECTS env var.
	excludes := append([]string(nil), flagExcludeProjects...)
	if env := os.Getenv("OMNISESS_EXCLUDE_PROJECTS"); env != "" {
		excludes = append(excludes, strings.Split(env, ",")...)
	}
	opts.ExcludeProjects = excludes

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
