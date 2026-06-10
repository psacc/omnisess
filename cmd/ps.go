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
	Short: "Show active Claude and Codex sessions as a process tree",
	Long:  "List live Claude Code sessions (CLI + Claude Desktop agent mode) and Codex sessions (CLI + Codex.app), grouped by shared ancestor chain up to launchd.",
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
		fmt.Fprintln(out, "No live sessions.")
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

	sort.SliceStable(root.children, func(i, j int) bool {
		return root.children[i].label < root.children[j].label
	})
	for _, top := range root.children {
		fmt.Fprintln(out, top.label)
		printChildren(out, top.children, "")
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
	return fmt.Sprintf("%s  %s  %s (%s)  %s  %s",
		s.Tool, name, project, shortID(s.SessionID), entry, age)
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

// printChildren renders the given list of children under a parent at `prefix`
// (empty prefix means "direct children of a top-level root"). Every child
// receives a connector — `├─` for middle children, `└─` for the last — and a
// correspondingly-indented prefix is passed down. This separation from the
// top-level print avoids the previous cascade where a last-child at prefix=""
// was treated as if it were another top-level root.
func printChildren(out io.Writer, children []*treeNode, prefix string) {
	sort.SliceStable(children, func(i, j int) bool {
		return children[i].label < children[j].label
	})
	for i, c := range children {
		last := i == len(children)-1
		connector := "├─ "
		nextPrefix := prefix + "│  "
		if last {
			connector = "└─ "
			nextPrefix = prefix + "   "
		}
		fmt.Fprintf(out, "%s%s%s\n", prefix, connector, c.label)
		printChildren(out, c.children, nextPrefix)
	}
}
