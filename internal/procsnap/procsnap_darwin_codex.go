//go:build darwin

package procsnap

import "fmt"

func codexSessions(procs map[int]procInfo) []Session {
	pids := codexCandidatePIDs(procs)
	if len(pids) == 0 {
		return nil
	}
	sessionsDir, lockDir, err := codexDirs()
	if err != nil {
		fmt.Fprintf(warnW, "procsnap: resolving codex sessions dir: %v\n", err)
		return nil
	}
	raw, lsofErr := codexLsofFn(pids)
	if len(raw) == 0 {
		if lsofErr != nil {
			fmt.Fprintf(warnW, "procsnap: lsof failed: %v%s\n", lsofErr, lsofErrDetail(lsofErr))
		}
		return nil
	}
	byPID, parseErr := parseCodexLsof(raw, sessionsDir, lockDir)
	if parseErr != nil {
		fmt.Fprintf(warnW, "procsnap: parsing lsof output: %v\n", parseErr)
	}
	var out []Session
	for _, pid := range pids {
		out = append(out, codexProcessSessions(pid, byPID[pid], sessionsDir, procs)...)
	}
	return out
}
