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
		args := strings.TrimSpace(proc.Args)
		if filepath.Base(proc.Command) == "codex" || args == "codex" ||
			strings.HasPrefix(args, "codex ") || strings.Contains(args, "/codex ") ||
			strings.HasSuffix(args, "/codex") {
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}
