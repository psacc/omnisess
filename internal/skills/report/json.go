package report

import (
	"encoding/json"
	"io"

	"github.com/psacc/omnisess/internal/skills"
)

type jsonOutput struct {
	Metadata  jsonMetadata        `json:"metadata"`
	Skills    []skills.SkillAudit `json:"skills"`
	Ghosts    []string            `json:"ghost_usage,omitempty"`
	Unmatched []string            `json:"unmatched_allowlist,omitempty"`
}

type jsonMetadata struct {
	WindowDays      int      `json:"window_days"`
	GeneratedAt     string   `json:"generated_at"`
	Roots           []string `json:"roots"`
	GlobalsIncluded bool     `json:"globals_included"`
	SessionsParsed  int      `json:"sessions_parsed"`
	OmnisessVersion string   `json:"omnisess_version"`
}

// JSON writes the audit result as indented JSON.
func JSON(w io.Writer, r skills.AuditResult) error {
	out := jsonOutput{
		Metadata: jsonMetadata{
			WindowDays:      int(r.Window.Hours() / 24),
			GeneratedAt:     r.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Roots:           r.Roots,
			GlobalsIncluded: r.GlobalsIncluded,
			SessionsParsed:  r.SessionsParsed,
			OmnisessVersion: r.OmnisessVersion,
		},
		Skills:    r.Skills,
		Ghosts:    r.GhostUsage,
		Unmatched: r.UnmatchedAllow,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
