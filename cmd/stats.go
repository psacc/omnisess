package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/index"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
	"github.com/psacc/omnisess/internal/source/claude"
)

var (
	flagStatsSession string
	flagStatsWindow  string
	flagStatsTool    string
	flagStatsFull    bool
	flagStatsJSON    bool
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show tool and file activity stats from the transcript index",
	Long: `omnisess stats reads the transcript SQLite index (populated by
omnisess index --all, or lazily on first use) and aggregates tool counts
and file-I/O activity.

  omnisess stats --session <id>     single-session detail
  omnisess stats --window 7d         aggregate over the last 7 days (default)
  omnisess stats --json              machine-readable output
  omnisess stats --full              include arguments/result payloads (requires --full also on index)
`,
	RunE: runStats,
}

func init() {
	statsCmd.Flags().StringVar(&flagStatsSession, "session", "", "Single session ID (with or without tool prefix)")
	statsCmd.Flags().StringVar(&flagStatsWindow, "window", "", "Time window (default 7d if no --session). Examples: 1d, 7d, 24h")
	statsCmd.Flags().StringVar(&flagStatsTool, "tool", "", "Filter to one source ('claude' is the only meaningful value in PR1)")
	statsCmd.Flags().BoolVar(&flagStatsFull, "full", false, "Include arguments and result payloads (requires --full also on prior index runs)")
	statsCmd.Flags().BoolVar(&flagStatsJSON, "json", false, "Machine-readable JSON output")
	rootCmd.AddCommand(statsCmd)
}

func runStats(_ *cobra.Command, _ []string) error {
	return runStatsTo(os.Stdout, os.Stderr)
}

// runStatsTo writes the rendered output to stdout (w) and warnings to wErr.
func runStatsTo(w, wErr io.Writer) error {
	if flagStatsSession != "" && flagStatsWindow != "" {
		return fmt.Errorf("--session and --window are mutually exclusive")
	}

	idx, err := openDefaultIndex()
	if err != nil {
		return err
	}
	defer idx.Close()

	if flagStatsSession != "" {
		return runStatsSession(w, wErr, idx)
	}
	return runStatsWindow(w, wErr, idx)
}

// runStatsSession handles --session <id> lookup.
func runStatsSession(w, wErr io.Writer, idx index.Index) error {
	origArg := flagStatsSession
	_, sessID := splitQualifiedID(origArg)
	// claude source is always registered (blank-imported in cmd/root.go);
	// no defensive empty-list check.
	src := source.ByName(model.ToolClaude)[0]
	full, err := src.Get(sessID)
	if err == nil && full != nil {
		// Lazy populate.
		cf, ferr := claude.SessionFilePath(full.ID)
		if ferr == nil && cf != "" {
			_ = idx.EnsureSession(cf, full.ID, flagStatsFull, false, index.SessionFromModel(full, "anthropic"))
		}
	}

	stats, err := idx.QuerySession(sessID)
	if err != nil {
		return fmt.Errorf("query session: %w", err)
	}
	if stats == nil {
		// Preserve the original tool-qualified arg in the error so the user
		// sees the same string they typed (e.g. "unknown:abc" not just "abc").
		return fmt.Errorf("session not found: %s", origArg)
	}
	if flagStatsJSON {
		return writeJSON(w, stats)
	}
	writeSessionStatsText(w, stats)
	_ = wErr
	return nil
}

// runStatsWindow handles --window <duration> aggregation.
func runStatsWindow(w, wErr io.Writer, idx index.Index) error {
	windowStr := flagStatsWindow
	if windowStr == "" {
		windowStr = "7d"
	}
	dur, err := parseDuration(windowStr)
	if err != nil {
		return fmt.Errorf("invalid --window: %w", err)
	}
	// Validate --tool BEFORE the lazy-index pass so an unknown value
	// fails fast without doing useless work. Only 'claude' is supported
	// in PR1; cursor/codex/copilot land in PR2.
	provider := ""
	switch flagStatsTool {
	case "":
		// no filter
	case "claude":
		provider = "anthropic"
	default:
		return fmt.Errorf("--tool %q not supported in PR1; only 'claude' is implemented", flagStatsTool)
	}
	end := time.Now()
	start := end.Add(-dur)

	// Lazy populate: enumerate Claude sessions in the window and ensure each
	// is indexed. Best-effort; warnings to stderr.
	for _, src := range source.ByName(model.ToolClaude) {
		lazyIndexWindow(wErr, src, idx, dur, flagStatsFull)
	}

	agg, err := idx.QueryWindow(start, end, provider)
	if err != nil {
		return fmt.Errorf("query window: %w", err)
	}
	if flagStatsJSON {
		return writeJSON(w, agg)
	}
	writeAggregateStatsText(w, agg, windowStr)
	return nil
}

// lazyIndexWindow enumerates a source's sessions in the window and lazy-
// indexes each. Warnings go to wErr; non-fatal so partial failures don't
// block the aggregate query.
func lazyIndexWindow(wErr io.Writer, src source.Source, idx index.Index, dur time.Duration, full bool) {
	sessions, lerr := src.List(source.ListOptions{Since: dur})
	if lerr != nil {
		fmt.Fprintf(wErr, "warning: list claude sessions: %v\n", lerr)
		return
	}
	for _, sess := range sessions {
		lazyIndexOne(wErr, src, idx, sess.ID, full)
	}
}

// lazyIndexOne is the stats variant of indexOneSession: errors are warnings,
// not fatal. Used to populate the index on first stats query.
func lazyIndexOne(wErr io.Writer, src source.Source, idx index.Index, id string, full bool) {
	sess, err := src.Get(id)
	if err != nil || sess == nil {
		return
	}
	cf, err := claude.SessionFilePath(id)
	if err != nil || cf == "" {
		return
	}
	if err := idx.EnsureSession(cf, id, full, false, index.SessionFromModel(sess, "anthropic")); err != nil {
		fmt.Fprintf(wErr, "warning: lazy-index %s: %v\n", id, err)
	}
}

// splitQualifiedID accepts either "<tool>:<id>" or just "<id>" and returns
// (tool, id). Tool defaults to "claude" when missing.
func splitQualifiedID(s string) (model.Tool, string) {
	if idx := strings.IndexByte(s, ':'); idx >= 0 {
		return model.Tool(s[:idx]), s[idx+1:]
	}
	return model.ToolClaude, s
}

// writeJSON marshals v as JSON to w, returning any encode error.
func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
