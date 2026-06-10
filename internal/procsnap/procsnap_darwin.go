//go:build darwin

package procsnap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// psRunnerFn is injectable. Real impl runs `ps -Ao pid=,ppid=,comm=,args=`
// which emits one row per process with no header.
var psRunnerFn = func() ([]byte, error) {
	cmd := exec.Command("ps", "-Ao", "pid=,ppid=,comm=,args=")
	return cmd.Output()
}

// sessionsDirFn is injectable. Real impl resolves ~/.claude/sessions/.
var sessionsDirFn = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "sessions"), nil
}

// Enumerate scans the Claude session registry, drops stale PID files,
// walks ancestor chains, and returns a snapshot. On macOS it never
// returns ErrUnsupported.
func Enumerate() (Snapshot, error) {
	dir, err := sessionsDirFn()
	if err != nil {
		return Snapshot{}, fmt.Errorf("procsnap: resolving sessions dir: %w", err)
	}

	files, err := scanSessionDir(dir)
	if err != nil {
		return Snapshot{}, err
	}
	alive := filterAlive(files)

	var procs map[int]procInfo
	raw, psErr := psRunnerFn()
	if psErr != nil {
		fmt.Fprintf(os.Stderr, "procsnap: ps failed: %v\n", psErr)
		procs = map[int]procInfo{}
	} else {
		// parsePS returns a partial map on error; best-effort is the right move
		// here — a truncated proc table beats an empty one.
		procs, _ = parsePS(raw)
	}

	sessions := make([]Session, 0, len(alive))
	for _, f := range alive {
		sessions = append(sessions, Session{
			Tool:       ToolClaude,
			PID:        f.PID,
			SessionID:  f.SessionID,
			Name:       f.Name,
			CWD:        f.CWD,
			StartedAt:  f.StartedAt,
			Entrypoint: f.Entrypoint,
			Version:    f.Version,
			Ancestors:  walkAncestors(f.PID, procs),
		})
	}
	sessions = append(sessions, codexSessions(procs)...)
	return Snapshot{Sessions: sessions, Built: time.Now()}, nil
}
