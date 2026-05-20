package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/spf13/cobra"
)

// errEmptySearchQuery is returned by searchArgs when the query is empty or
// whitespace-only. Surfaced at the cobra Args layer so the command fails fast
// without scanning the corpus (an empty pattern matches every message).
var errEmptySearchQuery = errors.New("search query cannot be empty")

// searchArgs validates that exactly one non-whitespace query argument is
// present. It combines cobra.MinimumNArgs(1) (cobra treats "" as a present
// arg) with an explicit empty-string check so that `omnisess search ""` is
// rejected with a clear message instead of dumping the full corpus.
func searchArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		return err
	}
	if strings.TrimSpace(args[0]) == "" {
		return errEmptySearchQuery
	}
	return nil
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across session content",
	Args:  searchArgs,
	RunE:  runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	sources := getSources()
	opts := getListOptions()

	var all []model.SearchResult
	for _, s := range sources {
		results, err := s.Search(query, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", s.Name(), err)
			continue
		}
		all = append(all, results...)
	}

	// Sort by recency
	sort.Slice(all, func(i, j int) bool {
		return all[i].Session.UpdatedAt.After(all[j].Session.UpdatedAt)
	})

	if opts.Limit > 0 && len(all) > opts.Limit {
		all = all[:opts.Limit]
	}

	output.RenderSearchResults(all, getFormat())
	return nil
}
