package source

import (
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// ListOptions controls filtering for List and Search operations.
type ListOptions struct {
	Since           time.Duration // only sessions updated within this duration
	OnDate          time.Time     // only sessions updated on this calendar day (in OnDate's location); zero = no filter
	Limit           int           // max results (0 = unlimited)
	Project         string        // filter by project path substring (include)
	ExcludeProjects []string      // exclude sessions matching any of these project substrings
	Active          bool          // only active sessions
}

// MatchesExclude returns true if project matches any of the exclusion substrings.
func MatchesExclude(project string, excludes []string) bool {
	for _, ex := range excludes {
		if ex != "" && strings.Contains(project, ex) {
			return true
		}
	}
	return false
}

// MatchesDate returns true if onDate is unset (zero) OR ts falls on the same
// calendar day as onDate, evaluated in onDate's location.
//
// This is the predicate used by the --date flag. It's a calendar-day match,
// not a duration-from-now match — useful for "show me sessions from
// 2026-04-22" without dealing with rolling windows.
func MatchesDate(ts, onDate time.Time) bool {
	if onDate.IsZero() {
		return true
	}
	loc := onDate.Location()
	y1, m1, d1 := ts.In(loc).Date()
	y2, m2, d2 := onDate.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// Source is the interface that each tool's session parser implements.
// See AGENTS.md for the full contract.
type Source interface {
	// Name returns the tool identifier ("claude", "cursor", "codex", "copilot").
	Name() model.Tool

	// List returns sessions ordered by most recent first.
	// Messages are NOT populated — use Get() for full content.
	List(opts ListOptions) ([]model.Session, error)

	// Get returns a single session with full message history.
	Get(sessionID string) (*model.Session, error)

	// Search returns sessions containing the query string.
	Search(query string, opts ListOptions) ([]model.SearchResult, error)
}
