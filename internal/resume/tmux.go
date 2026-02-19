//go:build !windows

package resume

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// ExecInTmux creates a new tmux session and attaches to it, replacing the
// current process. sessionName is used as the tmux session identifier,
// projectDir sets the working directory, and argv is the command to run
// inside the session (e.g. ["claude", "--resume", "abc123"]).
//
// On success this function never returns (the process is replaced).
func ExecInTmux(sessionName, projectDir string, argv []string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	command := shelljoin(argv)

	// tmux new-session -s <name> -c <dir> <command>
	// This creates the session AND attaches in one step.
	tmuxArgv := []string{
		"tmux",
		"new-session",
		"-s", sessionName,
		"-c", projectDir,
		command,
	}

	return syscall.Exec(tmuxPath, tmuxArgv, os.Environ())
}

// TmuxSessionName builds a deterministic tmux session name from a tool name
// and session ID. The result is safe for use as a tmux session identifier
// (no dots or colons, which tmux treats specially).
func TmuxSessionName(tool, sessionID string) string {
	// Use first 8 chars of session ID to keep it short.
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("sessions_%s_%s", tool, short)
}

// shelljoin concatenates argv elements into a single shell-safe string.
// Each element is single-quoted to prevent word splitting and glob expansion.
func shelljoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		// Replace single quotes inside the argument: ' -> '\''
		escaped := strings.ReplaceAll(arg, "'", `'\''`)
		quoted[i] = "'" + escaped + "'"
	}
	return strings.Join(quoted, " ")
}
