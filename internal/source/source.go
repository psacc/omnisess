package source

import (
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// ListOptions controls filtering for List and Search operations.
type ListOptions struct {
	Since   time.Duration // only sessions updated within this duration
	Limit   int           // max results (0 = unlimited)
	Project string        // filter by project path substring
	Active  bool          // only active sessions
}

// Source is the interface that each tool's session parser implements.
// See AGENTS.md for the full contract.
type Source interface {
	// Name returns the tool identifier ("claude", "cursor", "codex", "gemini").
	Name() model.Tool

	// List returns sessions ordered by most recent first.
	// Messages are NOT populated — use Get() for full content.
	List(opts ListOptions) ([]model.Session, error)

	// Get returns a single session with full message history.
	Get(sessionID string) (*model.Session, error)

	// Search returns sessions containing the query string.
	Search(query string, opts ListOptions) ([]model.SearchResult, error)
}
