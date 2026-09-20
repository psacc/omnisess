//go:build darwin

package procsnap

import (
	"fmt"
	"path/filepath"
)

func codexProcessSessions(pid int, proc codexProc, sessionsDir string, procs map[int]procInfo) []Session {
	ancestors := walkAncestors(pid, procs)
	seen := map[string]bool{}
	var out []Session
	for _, rollout := range proc.rollouts {
		id, started, filenameOK := parseRolloutFilename(rollout)
		meta, metaErr := readCodexSessionMeta(rollout)
		if metaErr != nil && !filenameOK {
			fmt.Fprintf(warnW, "procsnap: skipping %s: %v\n", rollout, metaErr)
			continue
		}
		session := Session{Tool: ToolCodex, PID: pid, SessionID: id, CWD: proc.cwd,
			StartedAt: started, Ancestors: ancestors}
		if metaErr == nil {
			applyCodexMeta(&session, meta)
		} else {
			fmt.Fprintf(warnW, "procsnap: reading %s: %v\n", rollout, metaErr)
		}
		out = append(out, session)
		seen[session.SessionID] = true
	}
	for _, id := range proc.threadIDs {
		if seen[id] {
			continue
		}
		session := Session{Tool: ToolCodex, PID: pid, SessionID: id, CWD: proc.cwd, Ancestors: ancestors}
		matches, globErr := filepath.Glob(filepath.Join(sessionsDir, "*", "*", "*", "*-"+id+".jsonl"))
		if globErr != nil {
			fmt.Fprintf(warnW, "procsnap: finding metadata for %s: %v\n", id, globErr)
		}
		if len(matches) > 0 {
			meta, metaErr := readCodexSessionMeta(matches[0])
			switch {
			case metaErr != nil:
				fmt.Fprintf(warnW, "procsnap: reading %s: %v\n", matches[0], metaErr)
			case meta.ID != id:
				fmt.Fprintf(warnW, "procsnap: metadata %s identifies session %s, want %s\n", matches[0], meta.ID, id)
			default:
				applyCodexMeta(&session, meta)
			}
		}
		out = append(out, session)
	}
	return out
}
