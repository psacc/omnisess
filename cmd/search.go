package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across session content",
	Args:  cobra.MinimumNArgs(1),
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
