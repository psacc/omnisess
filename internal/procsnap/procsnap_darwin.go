//go:build darwin

package procsnap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// filterAlive returns only entries whose PID is currently a live process we
// own. EPERM (process exists but owned by another user) is treated as dead:
// a PID we cannot signal is almost certainly a post-crash PID reuse by some
// unrelated system process, not a live Claude session. Claude writes its
// PID file as the same user that runs the process, so the happy path never
// sees EPERM.
func filterAlive(in []sessionFile) []sessionFile {
	out := make([]sessionFile, 0, len(in))
	for _, e := range in {
		if err := killFn(e.PID); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// procInfo is one row from `ps -Ao pid=,ppid=,comm=,args=`.
type procInfo struct {
	PID     int
	PPID    int
	Command string
	Args    string
}

// parsePS parses the output of `ps -Ao pid=,ppid=,comm=,args=` into a
// pid-indexed map. Malformed lines are silently skipped (ps output is
// whitespace-delimited and fixed-position on macOS, so a malformed line
// is almost always a truncation or synthetic environment).
func parsePS(raw []byte) (map[int]procInfo, error) {
	out := make(map[int]procInfo)
	if len(raw) == 0 {
		return out, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		// Parse PID.
		pidEnd := strings.IndexByte(trimmed, ' ')
		if pidEnd <= 0 {
			continue
		}
		pid, err := strconv.Atoi(trimmed[:pidEnd])
		if err != nil {
			continue
		}
		rest := strings.TrimLeft(trimmed[pidEnd:], " ")
		// Parse PPID.
		ppidEnd := strings.IndexByte(rest, ' ')
		if ppidEnd <= 0 {
			continue
		}
		ppid, err := strconv.Atoi(rest[:ppidEnd])
		if err != nil {
			continue
		}
		rest = strings.TrimLeft(rest[ppidEnd:], " ")
		// Parse comm (next whitespace-delimited field).
		commEnd := strings.IndexByte(rest, ' ')
		if commEnd <= 0 {
			// No args column — comm is the whole remainder.
			out[pid] = procInfo{PID: pid, PPID: ppid, Command: rest}
			continue
		}
		command := rest[:commEnd]
		args := strings.TrimLeft(rest[commEnd:], " ")
		out[pid] = procInfo{PID: pid, PPID: ppid, Command: command, Args: args}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// maxAncestorDepth bounds the walk against pathological PPID cycles.
// A real Unix process tree is never deeper than a few dozen levels.
const maxAncestorDepth = 64

// walkAncestors returns the chain of processes above startPID, ordered
// leaf-to-root. Stops at PID 1, an orphan (PPID missing from procs),
// or a detected cycle.
func walkAncestors(startPID int, procs map[int]procInfo) []Ancestor {
	var out []Ancestor
	self, ok := procs[startPID]
	if !ok {
		return out
	}
	seen := map[int]bool{startPID: true}
	ppid := self.PPID
	for depth := 0; depth < maxAncestorDepth; depth++ {
		if ppid == 0 {
			return out
		}
		if seen[ppid] {
			return out
		}
		seen[ppid] = true
		parent, ok := procs[ppid]
		if !ok {
			return out
		}
		out = append(out, Ancestor{
			PID:     parent.PID,
			Command: parent.Command,
			Args:    parent.Args,
		})
		if parent.PID == 1 {
			return out
		}
		ppid = parent.PPID
	}
	return out
}
