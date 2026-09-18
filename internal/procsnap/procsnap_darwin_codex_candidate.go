//go:build darwin

package procsnap

import (
	"sort"
)

func codexCandidatePIDs(procs map[int]procInfo) []int {
	var out []int
	for pid, proc := range procs {
		if proc.Command == "codex" {
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}
