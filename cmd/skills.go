package cmd

import (
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Inspect and audit agent skills",
	Long:  "Subcommands for inspecting agent-skill inventories and usage. v0 ships only `audit`.",
}

func init() {
	rootCmd.AddCommand(skillsCmd)
}
