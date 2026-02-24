package resume

import (
	"fmt"

	"github.com/psacc/omnisess/internal/model"
)

// Mode describes how a session will be resumed.
type Mode string

const (
	ModeResume Mode = "resume" // Resume existing session (direct exec)
	ModeFork   Mode = "fork"   // Fork into new session
	ModeTmux   Mode = "tmux"   // Resume in background tmux session
	ModeAoE    Mode = "aoe"    // Open in AoE (new or attach existing)
	ModeOpen   Mode = "open"   // Degraded: open project dir only
)

// Resumer builds an exec plan for resuming a session.
type Resumer interface {
	// Tool returns which tool this resumer handles.
	Tool() model.Tool

	// Modes returns the resume modes this tool supports.
	Modes() []Mode

	// Exec replaces the current process to resume the session.
	// Returns error only if exec setup fails (on success, never returns).
	Exec(session *model.Session, mode Mode) error
}

var registry = map[model.Tool]Resumer{}

// Register adds a resumer to the global registry.
// Called from each resumer's init() function.
func Register(r Resumer) {
	registry[r.Tool()] = r
}

// Get returns the resumer for the given tool, or nil and false if none is registered.
func Get(tool model.Tool) (Resumer, bool) {
	r, ok := registry[tool]
	return r, ok
}

// Modes returns the available resume modes for a tool.
// Returns nil if no resumer is registered for the tool.
func Modes(tool model.Tool) []Mode {
	r, ok := registry[tool]
	if !ok {
		return nil
	}
	return r.Modes()
}

// ErrUnsupportedMode is returned when a resumer does not support the requested mode.
type ErrUnsupportedMode struct {
	Tool model.Tool
	Mode Mode
}

func (e *ErrUnsupportedMode) Error() string {
	return fmt.Sprintf("%s does not support resume mode %q", e.Tool, e.Mode)
}
