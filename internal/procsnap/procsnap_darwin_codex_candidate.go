//go:build darwin

package procsnap

import (
	"path/filepath"
	"sort"
	"strings"
)

func codexCandidatePIDs(procs map[int]procInfo) []int {
	var out []int
	for pid, proc := range procs {
		if codexExecutable(proc) {
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}

func codexExecutable(proc procInfo) bool {
	if filepath.Base(proc.Command) == "codex" {
		return true
	}
	args := strings.TrimSpace(proc.Args)
	executable := strings.TrimSuffix(args, " app-server")
	return executable != args && filepath.IsAbs(proc.Command) &&
		strings.HasPrefix(executable, proc.Command) && filepath.Base(executable) == "codex"
}
