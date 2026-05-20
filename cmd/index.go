package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/index"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
	"github.com/psacc/omnisess/internal/source/claude"
)

var (
	flagIndexAll     bool
	flagIndexFull    bool
	flagIndexRebuild bool
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Walk every session and ensure it is in the transcript index (SQLite cache)",
	Long: `omnisess index --all bulk-walks every registered (non-stub) source's
sessions and ensures each is present in the transcript SQLite cache. The
default invocation captures metadata only; pass --full to also persist
tool_use arguments and toolUseResult payloads (privacy-sensitive — see docs).
`,
	RunE: runIndex,
}

func init() {
	indexCmd.Flags().BoolVar(&flagIndexAll, "all", false, "Index every session of every registered (non-stub) source")
	indexCmd.Flags().BoolVar(&flagIndexFull, "full", false, "Capture arguments and result payloads (privacy-sensitive)")
	indexCmd.Flags().BoolVar(&flagIndexRebuild, "rebuild", false, "Drop existing rows for each session before re-indexing")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(_ *cobra.Command, _ []string) error {
	return runIndexTo(os.Stderr)
}

// runIndexTo is the testable inner of runIndex. It writes progress to w.
func runIndexTo(w io.Writer) error {
	if !flagIndexAll {
		return fmt.Errorf("--all is required (other modes not implemented in PR1)")
	}

	idx, err := openDefaultIndex()
	if err != nil {
		return err
	}
	defer idx.Close()

	srcs := getSources()
	indexedCount := 0
	for _, s := range srcs {
		// Only Claude is implemented in this PR. Skip others silently so
		// future sources can be added without flag-list drift.
		if s.Name() != model.ToolClaude {
			continue
		}
		indexedCount += indexSourceTo(w, s, idx, flagIndexFull, flagIndexRebuild)
	}
	fmt.Fprintf(w, "Done. Indexed %d session(s).\n", indexedCount)
	return nil
}

// indexSourceTo lists sessions from src and indexes each one. Returns the
// number of sessions successfully indexed. Warnings (List failures,
// per-session failures) go to w; the function does not return an error so
// progress continues across source boundaries.
func indexSourceTo(w io.Writer, src source.Source, idx index.Index, full, rebuild bool) int {
	sessions, err := src.List(source.ListOptions{})
	if err != nil {
		fmt.Fprintf(w, "warning: list %s: %v\n", src.Name(), err)
		return 0
	}
	count := 0
	for _, sess := range sessions {
		ok := indexOneSession(w, src, idx, sess.ID, full, rebuild)
		if ok {
			count++
			if count%50 == 0 {
				fmt.Fprintf(w, "indexed %d sessions...\n", count)
			}
		}
	}
	return count
}

// indexOneSession runs the per-session pipeline: Get → resolve file path →
// SessionFromModel → EnsureSession. Returns true if the row was written.
func indexOneSession(w io.Writer, src source.Source, idx index.Index, id string, full, rebuild bool) bool {
	sess, err := src.Get(id)
	if err != nil || sess == nil {
		return false
	}
	cf, err := claude.SessionFilePath(id)
	if err != nil || cf == "" {
		return false
	}
	if err := idx.EnsureSession(cf, id, full, rebuild, index.SessionFromModel(sess, "anthropic")); err != nil {
		fmt.Fprintf(w, "warning: index %s: %v\n", id, err)
		return false
	}
	return true
}

// openDefaultIndex resolves the index path (env + default) and Opens.
// Wrapped to a single helper so cmd/stats.go can share it without
// duplicating the env-resolution logic.
func openDefaultIndex() (index.Index, error) {
	path, err := index.DefaultPath()
	if err != nil {
		return nil, fmt.Errorf("resolve index path: %w", err)
	}
	idx, err := index.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open index at %s: %w", path, err)
	}
	return idx, nil
}
