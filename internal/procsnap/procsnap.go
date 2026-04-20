// Package procsnap enumerates live Claude Code sessions by correlating
// ~/.claude/sessions/<PID>.json registry files with the running process
// table. macOS only; other platforms return ErrUnsupported.
package procsnap

import (
	"errors"
	"time"
)

// ErrUnsupported is returned by Enumerate on platforms that are not macOS.
var ErrUnsupported = errors.New("procsnap: unsupported platform")

// Session is one live Claude Code process with its ancestor chain.
type Session struct {
	PID        int
	SessionID  string
	Name       string // from /rename, empty if unset
	CWD        string
	StartedAt  time.Time
	Entrypoint string // "cli" | "claude-desktop" | other
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

// IsActive reports whether the given Claude session ID is backed by a
// live process in this snapshot.
func (s Snapshot) IsActive(sessionID string) bool {
	for i := range s.Sessions {
		if s.Sessions[i].SessionID == sessionID {
			return true
		}
	}
	return false
}
