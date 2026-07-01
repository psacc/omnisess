//go:build darwin

package procsnap

import (
	"os/exec"
	"strconv"
	"strings"
)

// tmuxRunnerFn is injectable. Real impl runs
// `tmux list-panes -a -F '#{pane_pid} #{session_name}'`, one row per pane.
var tmuxRunnerFn = func() ([]byte, error) {
	cmd := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_pid} #{session_name}")
	return cmd.Output()
}

// tmuxPanes returns a pane_pid -> tmux session name map. tmux missing or not
// running makes the command error; we swallow it and return an empty map so
// the TmuxSession field simply stays unset (cross-platform / no-tmux safe).
func tmuxPanes() map[int]string {
	raw, err := tmuxRunnerFn()
	if err != nil {
		return map[int]string{}
	}
	return parseTmuxPanes(raw)
}

// parseTmuxPanes parses `#{pane_pid} #{session_name}` rows. pane_pid is the
// first whitespace-delimited token; the remainder is the session name kept
// verbatim (names may contain spaces).
func parseTmuxPanes(raw []byte) map[int]string {
	out := map[int]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidStr, name, found := strings.Cut(line, " ")
		if !found || name == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		out[pid] = name
	}
	return out
}

// resolveTmuxSession returns the tmux session owning this process, testing its
// own PID then each ancestor PID in order (first hit wins). This covers panes
// that exec the session directly (pane_pid == session PID) and panes that run
// an intermediate shell (pane_pid is an ancestor). It deliberately never reads
// the tmux server's `-s` arg: that server is the shared ancestor of every
// pane, so its boot session name would leak onto all of them. No match -> "".
func resolveTmuxSession(pid int, ancestors []Ancestor, panes map[int]string) string {
	if name, ok := panes[pid]; ok {
		return name
	}
	for _, a := range ancestors {
		if name, ok := panes[a.PID]; ok {
			return name
		}
	}
	return ""
}
