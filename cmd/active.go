package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/spf13/cobra"
)

var activeCmd = &cobra.Command{
	Use:   "active",
	Short: "Show only active (running) sessions",
	RunE:  runActive,
}

func init() {
	rootCmd.AddCommand(activeCmd)
}

func runActive(cmd *cobra.Command, args []string) error {
	sources := getSources()
	opts := getListOptions()
	opts.Active = true

	var all []model.Session
	for _, s := range sources {
		sessions, err := s.List(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", s.Name(), err)
			continue
		}
		all = append(all, sessions...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(all) > opts.Limit {
		all = all[:opts.Limit]
	}

	output.RenderSessions(all, getFormat())
	return nil
}
