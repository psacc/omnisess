//go:build darwin

package procsnap

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

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
	for range maxAncestorDepth {
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
