package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/procsnap"
)

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

// treeNode is a transient representation used only during rendering.
type treeNode struct {
	label    string      // what to print on this line
	children []*treeNode // ancestors-as-children going downward
	byPID    map[int]*treeNode
}

func newNode(label string) *treeNode {
	return &treeNode{label: label, byPID: map[int]*treeNode{}}
}

// renderTree prints the merged process tree to out.
func renderTree(out io.Writer, snap procsnap.Snapshot) {
	if len(snap.Sessions) == 0 {
		return
	}

	root := newNode("") // virtual super-root; its children are the true roots
	for _, s := range snap.Sessions {
		// Ancestors are leaf-to-root; flip so root is first.
		chain := make([]procsnap.Ancestor, len(s.Ancestors))
		for i, a := range s.Ancestors {
			chain[len(chain)-1-i] = a
		}

		parent := root
		for _, a := range chain {
			child, ok := parent.byPID[a.PID]
			if !ok {
				label := fmt.Sprintf("%s (%d)", a.Command, a.PID)
				child = newNode(label)
				parent.byPID[a.PID] = child
				parent.children = append(parent.children, child)
			}
			parent = child
		}
		leaf := newNode(leafLabel(s, snap.Built))
		parent.children = append(parent.children, leaf)
	}

	for _, top := range root.children {
		printNode(out, top, "", true)
	}
}

func leafLabel(s procsnap.Session, now time.Time) string {
	name := s.Name
	if name == "" {
		name = shortID(s.SessionID)
	}
	project := projectBase(s.CWD)
	age := formatAge(now.Sub(s.StartedAt))
	entry := s.Entrypoint
	if entry == "claude-desktop" {
		entry = "desktop"
	}
	return fmt.Sprintf("claude  %s  %s (%s)  %s  %s",
		name, project, shortID(s.SessionID), entry, age)
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func projectBase(cwd string) string {
	if cwd == "" {
		return "-"
	}
	idx := strings.LastIndex(cwd, "/")
	if idx < 0 || idx == len(cwd)-1 {
		return cwd
	}
	return cwd[idx+1:]
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func printNode(out io.Writer, n *treeNode, prefix string, isLast bool) {
	var connector string
	var nextPrefix string
	if prefix == "" && isLast {
		// Top-level node: no connector.
		fmt.Fprintln(out, n.label)
		nextPrefix = ""
	} else {
		if isLast {
			connector = "└─ "
			nextPrefix = prefix + "   "
		} else {
			connector = "├─ "
			nextPrefix = prefix + "│  "
		}
		fmt.Fprintf(out, "%s%s%s\n", prefix, connector, n.label)
	}

	// Stable ordering: named claude leaves first, then by label.
	sort.SliceStable(n.children, func(i, j int) bool {
		return n.children[i].label < n.children[j].label
	})
	for i, c := range n.children {
		last := i == len(n.children)-1
		var childPrefix string
		if prefix == "" && connector == "" {
			childPrefix = ""
		} else {
			childPrefix = nextPrefix
		}
		printNode(out, c, childPrefix, last)
	}
}
