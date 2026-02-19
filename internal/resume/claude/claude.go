//go:build !windows

package claude

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/resume"
)

func init() {
	resume.Register(&claudeResumer{})
}

type claudeResumer struct{}

func (r *claudeResumer) Tool() model.Tool { return model.ToolClaude }

func (r *claudeResumer) Modes() []resume.Mode {
	return []resume.Mode{resume.ModeResume, resume.ModeFork, resume.ModeTmux, resume.ModeAoE}
}

// buildArgv constructs the argv slice for the given mode and session ID.
func buildArgv(mode resume.Mode, sessionID string) []string {
	argv := []string{"claude", "--resume", sessionID}
	if mode == resume.ModeFork {
		argv = append(argv, "--fork-session")
	}
	return argv
}

// Exec replaces the current process with "claude --resume <id>" (or
// "claude --resume <id> --fork-session" for fork mode).
// It changes to the session's project directory first so Claude Code
// resolves the correct project context.
func (r *claudeResumer) Exec(session *model.Session, mode resume.Mode) error {
	switch mode {
	case resume.ModeResume, resume.ModeFork:
		return r.execDirect(session, mode)
	case resume.ModeTmux:
		return r.execTmux(session)
	case resume.ModeAoE:
		return r.execAoE(session)
	default:
		return &resume.ErrUnsupportedMode{Tool: model.ToolClaude, Mode: mode}
	}
}

func (r *claudeResumer) execDirect(session *model.Session, mode resume.Mode) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}

	if session.Project != "" {
		if err := os.Chdir(session.Project); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not chdir to %s: %v\n", session.Project, err)
		}
	}

	argv := buildArgv(mode, session.ID)
	return syscall.Exec(claudePath, argv, os.Environ())
}

func (r *claudeResumer) execAoE(session *model.Session) error {
	dir := session.Project
	if dir == "" {
		dir = "."
	}
	title := session.ShortProject() + " (claude)"
	return resume.ExecInAoE(string(model.ToolClaude), dir, title)
}

func (r *claudeResumer) execTmux(session *model.Session) error {
	dir := session.Project
	if dir == "" {
		dir = "."
	}
	name := resume.TmuxSessionName(string(model.ToolClaude), session.ID)
	argv := buildArgv(resume.ModeResume, session.ID)
	return resume.ExecInTmux(name, dir, argv)
}
