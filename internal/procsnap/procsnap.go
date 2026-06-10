// Package procsnap enumerates live AI coding sessions by correlating
// on-disk session state with the running process table: Claude Code via
// ~/.claude/sessions/<PID>.json registry files, Codex via rollout JSONL
// files held open under ~/.codex/sessions. macOS only; other platforms
// return ErrUnsupported.
package procsnap

import (
	"errors"
	"time"
)

// ErrUnsupported is returned by Enumerate on platforms that are not macOS.
var ErrUnsupported = errors.New("procsnap: unsupported platform")

// Tool values carried by Session.Tool. They match the model.Tool string
// values so downstream consumers can compare across packages.
const (
	ToolClaude = "claude"
	ToolCodex  = "codex"
)

// Session is one live AI coding session process with its ancestor chain.
type Session struct {
	Tool       string // ToolClaude | ToolCodex
	PID        int
	SessionID  string
	Name       string // from /rename, empty if unset (claude only)
	CWD        string
	StartedAt  time.Time
	Entrypoint string // claude: "cli" | "claude-desktop"; codex: originator, e.g. "codex-tui"
	Version    string
	Ancestors  []Ancestor // index 0 = immediate parent, last = root
}

// Ancestor is one step up the process tree above a claude session.
type Ancestor struct {
	PID     int
	Command string // comm (short executable name)
	Args    string // trimmed full args line
}

// Snapshot is a point-in-time view of all live Claude sessions.
type Snapshot struct {
	Sessions []Session
	Built    time.Time
}

// IsActive reports whether the given session ID is backed by a live
// process in this snapshot. Session IDs are UUIDs, so matches are
// unambiguous across tools.
func (s Snapshot) IsActive(sessionID string) bool {
	for i := range s.Sessions {
		if s.Sessions[i].SessionID == sessionID {
			return true
		}
	}
	return false
}
