//go:build !windows

package resume

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// aoeSession represents a single entry from `aoe list --json`.
type aoeSession struct {
	ID          string `json:"id"`
	Tool        string `json:"tool"`
	ProjectPath string `json:"project_path"`
}

// FindAoESession searches the running AoE sessions for one matching the given
// tool and project directory. It returns the AoE session ID if found, or an
// empty string if no match exists. An error is returned only on execution
// failures (missing binary, malformed JSON, etc.).
func FindAoESession(tool, projectDir string) (string, error) {
	aoePath, err := exec.LookPath("aoe")
	if err != nil {
		return "", fmt.Errorf("aoe not found in PATH: %w", err)
	}

	out, err := exec.Command(aoePath, "list", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("aoe list --json failed: %w", err)
	}

	var sessions []aoeSession
	if err := json.Unmarshal(out, &sessions); err != nil {
		return "", fmt.Errorf("parsing aoe list output: %w", err)
	}

	// Normalize the target project directory for comparison.
	normDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		normDir = projectDir // fall back to raw path
	}

	for _, s := range sessions {
		if s.Tool != tool {
			continue
		}
		candidate, err := filepath.EvalSymlinks(s.ProjectPath)
		if err != nil {
			candidate = s.ProjectPath
		}
		if candidate == normDir {
			return s.ID, nil
		}
	}

	return "", nil
}

// ExecInAoE opens (or attaches to) an AoE session for the given tool and
// project directory. If an existing AoE session matches tool+projectDir, the
// process is replaced with `aoe session attach <id>`. Otherwise a new session
// is created via `aoe add -c <tool> --title <title> --launch <projectDir>`.
//
// On success this function never returns (the process is replaced).
func ExecInAoE(tool, projectDir, title string) error {
	aoePath, err := exec.LookPath("aoe")
	if err != nil {
		return fmt.Errorf("aoe not found in PATH: %w", err)
	}

	// Try to find an existing AoE session for this tool+project.
	existingID, err := FindAoESession(tool, projectDir)
	if err != nil {
		// Non-fatal: fall through to creating a new session.
		fmt.Fprintf(os.Stderr, "warning: could not query AoE sessions: %v\n", err)
	}

	var argv []string
	if existingID != "" {
		argv = []string{"aoe", "session", "attach", existingID}
	} else {
		argv = []string{"aoe", "add", "-c", tool, "--title", title, "--launch", projectDir}
	}

	return syscall.Exec(aoePath, argv, os.Environ())
}
