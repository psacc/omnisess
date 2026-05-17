package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/psacc/omnisess/internal/source"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <tool:session-id>",
	Short: "Show full session details",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	toolName, sessionID, err := parseQualifiedID(args[0])
	if err != nil {
		return err
	}
	// parseQualifiedID validates the tool name, so source.ByName always returns ≥ 1 element.
	return showSession(source.ByName(toolName)[0], args[0], sessionID, getFormat())
}

func showSession(src source.Source, qualifiedID, sessionID string, format output.Format) error {
	session, err := src.Get(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", qualifiedID)
	}
	output.RenderSession(session, format)
	return nil
}

func parseQualifiedID(s string) (model.Tool, string, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected format tool:session-id (e.g., claude:5c3f2742), got %q", s)
	}
	tool := model.Tool(parts[0])
	// Validate against the live source registry rather than a hardcoded
	// switch. PR #44 had to patch this allow-list when Copilot was missed;
	// driving it from source.All() removes the drift class entirely.
	if tool == "" || len(source.ByName(tool)) == 0 {
		return "", "", fmt.Errorf("unknown tool %q, expected one of: %s",
			parts[0], strings.Join(registeredToolNames(), ", "))
	}
	return tool, parts[1], nil
}

// registeredToolNames returns the names of every registered source, sorted
// alphabetically for stable error messages.
func registeredToolNames() []string {
	srcs := source.All()
	names := make([]string, len(srcs))
	for i, s := range srcs {
		names[i] = string(s.Name())
	}
	sort.Strings(names)
	return names
}
