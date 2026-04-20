package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/procsnap"
)

// TODO(B2): flagPSTree is declared now to reserve the name; it becomes
// meaningful when the tree renderer lands in Task B2. Unused until then.
//
//nolint:unused // reserved flag, see TODO.
var flagPSTree bool

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show active Claude sessions as a process tree",
	Long:  "List live Claude Code sessions (CLI + Claude Desktop agent mode), grouped by shared ancestor chain up to launchd.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPSWith(os.Stdout, procsnap.Enumerate, flagJSON)
	},
}

func init() {
	rootCmd.AddCommand(psCmd)
}

// enumerator matches procsnap.Enumerate's signature for dependency injection.
type enumerator func() (procsnap.Snapshot, error)

func runPSWith(out io.Writer, enum enumerator, asJSON bool) error {
	snap, err := enum()
	if err != nil {
		if errors.Is(err, procsnap.ErrUnsupported) {
			fmt.Fprintln(out, "omnisess ps: unsupported on this platform (macOS only)")
			return nil
		}
		return err
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	}

	if len(snap.Sessions) == 0 {
		fmt.Fprintln(out, "No live Claude sessions.")
		return nil
	}
	renderTree(out, snap)
	return nil
}

// renderTree is defined in Task B2.
func renderTree(out io.Writer, snap procsnap.Snapshot) { /* filled in Task B2 */ }
