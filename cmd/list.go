package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/output"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions across all tools",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	sources := getSources()
	opts := getListOptions()

	var all []model.Session
	for _, s := range sources {
		sessions, err := s.List(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", s.Name(), err)
			continue
		}
		all = append(all, sessions...)
	}

	// Sort by UpdatedAt descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(all) > opts.Limit {
		all = all[:opts.Limit]
	}

	output.RenderSessions(all, getFormat())
	return nil
}
