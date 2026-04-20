//go:build darwin

package procsnap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// sessionFile is the parsed shape of ~/.claude/sessions/<PID>.json.
type sessionFile struct {
	PID        int
	SessionID  string
	Name       string
	CWD        string
	StartedAt  time.Time
	Version    string
	Entrypoint string
}

// rawSessionFile mirrors the JSON keys emitted by Claude Code.
type rawSessionFile struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"` // unix millis
	Version    string `json:"version"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
}

// scanSessionDir reads every *.json file in dir and returns the parsed
// entries. Missing directories return an empty slice and nil error.
// Unreadable or malformed files are logged to stderr and skipped.
func scanSessionDir(dir string) ([]sessionFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("procsnap: reading %s: %w", dir, err)
	}

	out := make([]sessionFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "procsnap: reading %s: %v\n", path, err)
			continue
		}
		var raw rawSessionFile
		if err := json.Unmarshal(data, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "procsnap: parsing %s: %v\n", path, err)
			continue
		}
		if raw.PID == 0 || raw.SessionID == "" {
			continue
		}
		out = append(out, sessionFile{
			PID:        raw.PID,
			SessionID:  raw.SessionID,
			Name:       raw.Name,
			CWD:        raw.CWD,
			StartedAt:  time.UnixMilli(raw.StartedAt),
			Version:    raw.Version,
			Entrypoint: raw.Entrypoint,
		})
	}
	return out, nil
}

// killFn is injectable for tests. syscall.Kill(pid, 0) is the portable
// liveness probe: returns nil if the process exists and we can signal it.
var killFn = func(pid int) error { return syscall.Kill(pid, 0) }

// filterAlive returns only entries whose PID is currently a live process.
func filterAlive(in []sessionFile) []sessionFile {
	out := make([]sessionFile, 0, len(in))
	for _, e := range in {
		if err := killFn(e.PID); err == nil {
			out = append(out, e)
		}
	}
	return out
}
