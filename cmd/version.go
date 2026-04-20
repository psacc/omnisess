package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// resolveVersionFromInfo extracts a user-facing version string from a build
// info triple. A binary installed via `go install ...@vX.Y.Z` has the module
// version embedded; `go build` (devel) or unknown falls back to "(devel)".
func resolveVersionFromInfo(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "(devel)"
	}
	return info.Main.Version
}

func resolveVersion() string {
	return resolveVersionFromInfo(debug.ReadBuildInfo())
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print omnisess version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), resolveVersion())
	},
}

func init() {
	rootCmd.Version = resolveVersion()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.AddCommand(versionCmd)
}
