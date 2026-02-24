//go:build !windows

package cursor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/resume"
)

func init() {
	resume.Register(&cursorResumer{})
}

type cursorResumer struct{}

func (r *cursorResumer) Tool() model.Tool { return model.ToolCursor }

func (r *cursorResumer) Modes() []resume.Mode {
	return []resume.Mode{resume.ModeResume, resume.ModeTmux, resume.ModeAoE}
}

// buildArgv constructs the argv slice for resuming a Cursor session.
func buildArgv(sessionID string) []string {
	return []string{"cursor", "agent", "--resume", sessionID}
}

// Exec replaces the current process with "cursor agent --resume <id>".
// It changes to the session's project directory first so Cursor resolves
// the correct project context.
func (r *cursorResumer) Exec(session *model.Session, mode resume.Mode) error {
	switch mode {
	case resume.ModeResume:
		return r.execDirect(session)
	case resume.ModeTmux:
		return r.execTmux(session)
	case resume.ModeAoE:
		return r.execAoE(session)
	default:
		return &resume.ErrUnsupportedMode{Tool: model.ToolCursor, Mode: mode}
	}
}

func (r *cursorResumer) execDirect(session *model.Session) error {
	cursorPath, err := exec.LookPath("cursor")
	if err != nil {
		return fmt.Errorf("cursor CLI not found in PATH: %w", err)
	}

	if session.Project != "" {
		if err := os.Chdir(session.Project); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not chdir to %s: %v\n", session.Project, err)
		}
	}

	argv := buildArgv(session.ID)
	return syscall.Exec(cursorPath, argv, os.Environ())
}

func (r *cursorResumer) execAoE(session *model.Session) error {
	dir := session.Project
	if dir == "" {
		dir = "."
	}
	title := session.ShortProject() + " (cursor)"
	return resume.ExecInAoE(string(model.ToolCursor), dir, title)
}

func (r *cursorResumer) execTmux(session *model.Session) error {
	dir := session.Project
	if dir == "" {
		dir = "."
	}
	name := resume.TmuxSessionName(string(model.ToolCursor), session.ID)
	argv := buildArgv(session.ID)
	return resume.ExecInTmux(name, dir, argv)
}
