package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/skills/allowlist"
	"github.com/psacc/omnisess/internal/skills/audit"
	"github.com/psacc/omnisess/internal/skills/discovery"
	"github.com/psacc/omnisess/internal/skills/report"
	"github.com/psacc/omnisess/internal/skills/usage"
)

// auditDeps holds injectable dependencies for runSkillsAudit.
// Tests override these to avoid real filesystem access.
type auditDeps struct {
	homeDir          func() (string, error)
	findSessionFiles func(root string) ([]string, error)
}

// defaultAuditDeps returns production dependencies.
func defaultAuditDeps() auditDeps {
	return auditDeps{
		homeDir:          os.UserHomeDir,
		findSessionFiles: usage.FindSessionFiles,
	}
}

// auditDepsVar is the active dependency set; tests can replace it.
var auditDepsVar = defaultAuditDeps()

var (
	auditRoots     []string
	auditNoGlobals bool
	auditAllowlist string
	auditWindow    string
	auditFormat    string
	auditOutput    string
)

var skillsAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Classify skills by usage (Keep/Borderline/Archive/Unknown)",
	Long: `Audit agent skills by joining the discovered inventory with invocation
counts from Claude Code session JSONL transcripts.

Read-only. Never modifies skills, sessions, or the allowlist.`,
	RunE: runSkillsAudit,
}

func init() {
	skillsCmd.AddCommand(skillsAuditCmd)
	skillsAuditCmd.Flags().StringSliceVar(&auditRoots, "root", nil, "Project root to walk (repeatable)")
	skillsAuditCmd.Flags().BoolVar(&auditNoGlobals, "no-globals", false, "Skip global skill locations (~/.claude/skills, ~/.agents/skills, ~/.claude/plugins)")
	skillsAuditCmd.Flags().StringVar(&auditAllowlist, "allowlist", "", "Path to plain-text allowlist file (one skill name per line)")
	skillsAuditCmd.Flags().StringVar(&auditWindow, "window", "90d", "Telemetry window: 30d, 90d, 180d")
	skillsAuditCmd.Flags().StringVar(&auditFormat, "format", "markdown", "Output format: markdown or json")
	skillsAuditCmd.Flags().StringVarP(&auditOutput, "output", "o", "", "Output file (default stdout)")
}

func runSkillsAudit(cmd *cobra.Command, args []string) error {
	deps := auditDepsVar
	return runSkillsAuditWith(cmd, deps)
}

func runSkillsAuditWith(cmd *cobra.Command, deps auditDeps) error {
	if len(auditRoots) == 0 && auditNoGlobals {
		return fmt.Errorf("at least one --root is required (or remove --no-globals)")
	}

	window, err := parseWindow(auditWindow)
	if err != nil {
		return err
	}

	// 1. Discover
	discOpts := discovery.Options{
		Roots:          auditRoots,
		IncludeGlobals: !auditNoGlobals,
	}
	skillsList, err := discovery.Walk(discOpts)
	if err != nil {
		return fmt.Errorf("discovery: %w", err)
	}

	// 2. Scan usage
	home, err := deps.homeDir()
	if err != nil {
		home = ""
	}
	projectsRoot := filepath.Join(home, ".claude", "projects")
	files, err := deps.findSessionFiles(projectsRoot)
	if err != nil {
		return fmt.Errorf("find sessions: %w", err)
	}
	invocations, err := usage.Scan(usage.ScanOptions{
		Files: files,
		Since: time.Now().Add(-window),
	})
	if err != nil {
		return fmt.Errorf("scan sessions: %w", err)
	}

	// 3. Allowlist
	allow, err := allowlist.Load(auditAllowlist)
	if err != nil {
		return fmt.Errorf("allowlist: %w", err)
	}

	// 4. Tier
	res := audit.Tier(audit.TierInput{
		Skills:      skillsList,
		Invocations: invocations,
		Allowlist:   allow,
		Window:      window,
		Now:         time.Now(),
	})
	res.Roots = auditRoots
	res.GlobalsIncluded = !auditNoGlobals
	res.SessionsParsed = len(files)
	res.OmnisessVersion = resolveVersion()

	// 5. Render
	w := io.Writer(cmd.OutOrStdout())
	if auditOutput != "" {
		f, err := os.Create(auditOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	switch strings.ToLower(auditFormat) {
	case "markdown", "md":
		return report.Markdown(w, res)
	case "json":
		return report.JSON(w, res)
	default:
		return fmt.Errorf("unknown format %q (want markdown or json)", auditFormat)
	}
}

func parseWindow(s string) (time.Duration, error) {
	switch s {
	case "30d":
		return 30 * 24 * time.Hour, nil
	case "90d":
		return 90 * 24 * time.Hour, nil
	case "180d":
		return 180 * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid --window %q (allowed: 30d, 90d, 180d)", s)
}
