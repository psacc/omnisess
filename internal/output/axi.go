package output

import (
	"encoding/json"
	"io"
	"os"

	"github.com/psacc/omnisess/internal/model"
)

// axiSession is the minimal per-session projection emitted under FormatAxi.
// Five fields — the essentials an agent needs to identify, filter, and drill
// into a session — instead of the ~13-field model.Session the --json path
// emits. id is the FULL session ID (not the 8-char short form) so that the
// next-step `omnisess show <tool>:<id>` hint resolves: showSession calls
// src.Get(id) with the exact ID.
type axiSession struct {
	Tool    string `json:"tool"`
	ID      string `json:"id"`
	Project string `json:"project"`
	Status  string `json:"status"`
	Updated string `json:"updated"`
}

// axiSummary is the pre-computed aggregate (axi principle #4): counts that let
// an agent answer "how many, of what, doing what" without a follow-up round
// trip or re-counting the array itself. Total is also the definitive empty
// state signal (#5): total=0 is unambiguous where a bare [] is not. Status and
// Tools are always non-nil so they serialize as {} rather than null.
type axiSummary struct {
	Total  int            `json:"total"`
	Active int            `json:"active"`
	Status map[string]int `json:"status"`
	Tools  map[string]int `json:"tools"`
}

// axiEnvelope wraps the projection with the aggregate and the next-step hints
// (#9). It is emitted as compact (unindented) JSON — the token-efficient form.
type axiEnvelope struct {
	Sessions []axiSession `json:"sessions"`
	Summary  axiSummary   `json:"summary"`
	Next     []string     `json:"next"`
}

// axiStatus maps a session to a machine-friendly status token: the live
// registry status when known (busy/idle/waiting/...), else "active" for a
// live session with no finer signal, else "inactive".
func axiStatus(s model.Session) string {
	switch {
	case s.Active && s.Status != "":
		return s.Status
	case s.Active:
		return "active"
	default:
		return "inactive"
	}
}

// buildAxiEnvelope projects sessions into the minimal schema, computes the
// aggregate, and derives command-agnostic next-step hints. Shared by `list`
// and `active` (both call RenderSessions).
func buildAxiEnvelope(sessions []model.Session) axiEnvelope {
	items := make([]axiSession, 0, len(sessions))
	summary := axiSummary{Status: map[string]int{}, Tools: map[string]int{}}

	for _, s := range sessions {
		st := axiStatus(s)
		items = append(items, axiSession{
			Tool:    string(s.Tool),
			ID:      s.ID,
			Project: sanitizeString(s.ShortProject()),
			Status:  st,
			Updated: s.UpdatedAt.Local().Format("2006-01-02T15:04"),
		})
		summary.Total++
		if s.Active {
			summary.Active++
		}
		summary.Status[st]++
		summary.Tools[string(s.Tool)]++
	}

	next := []string{"omnisess list --since 24h"}
	if len(sessions) > 0 {
		next = []string{
			"omnisess show " + sessions[0].QualifiedID(),
			`omnisess search "<term>"`,
		}
	}

	return axiEnvelope{Sessions: items, Summary: summary, Next: next}
}

// renderAxiSessions writes the axi envelope for a session list as compact JSON.
func renderAxiSessions(w io.Writer, sessions []model.Session) {
	enc := json.NewEncoder(w) // no SetIndent → compact, one line
	enc.SetEscapeHTML(false)  // keep "<term>" readable, don't emit <
	_ = enc.Encode(buildAxiEnvelope(sessions))
}

// renderAxiSessionsStdout is the os.Stdout entry point used by RenderSessions.
func renderAxiSessionsStdout(sessions []model.Session) {
	renderAxiSessions(os.Stdout, sessions)
}
