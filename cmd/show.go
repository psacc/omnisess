package cmd

import (
	"fmt"
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

	sources := source.ByName(toolName)
	if len(sources) == 0 {
		return fmt.Errorf("unknown tool: %s", toolName)
	}

	session, err := sources[0].Get(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", args[0])
	}

	output.RenderSession(session, getFormat())
	return nil
}

func parseQualifiedID(s string) (model.Tool, string, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected format tool:session-id (e.g., claude:5c3f2742), got %q", s)
	}
	tool := model.Tool(parts[0])
	switch tool {
	case model.ToolClaude, model.ToolCursor, model.ToolCodex, model.ToolGemini:
		return tool, parts[1], nil
	default:
		return "", "", fmt.Errorf("unknown tool %q, expected one of: claude, cursor, codex, gemini", parts[0])
	}
}
