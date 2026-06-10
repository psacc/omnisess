//go:build darwin

package procsnap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Codex has no PID registry. The correlation primitive is instead that every
// live codex session holds its rollout JSONL open for the session's lifetime:
// candidate codex PIDs come from the shared ps snapshot, and one lsof call
// maps each PID to the rollout file(s) it has open under ~/.codex/sessions.

// codexSessionsDirFn is injectable. Real impl resolves ~/.codex/sessions/.
var codexSessionsDirFn = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

// codexLsofFn is injectable. Real impl runs `lsof -a -p <p1,p2,...> -F pfn`,
// which emits machine-readable records: p<pid>, then f<fd>/n<name> pairs.
var codexLsofFn = func(pids []int) ([]byte, error) {
	strs := make([]string, len(pids))
	for i, p := range pids {
		strs[i] = strconv.Itoa(p)
	}
	cmd := exec.Command("lsof", "-a", "-p", strings.Join(strs, ","), "-F", "pfn")
	return cmd.Output()
}

// codexMetaMaxLine caps the session_meta first-line read. Real lines run
// tens of KB (embedded base instructions); 4 MiB leaves ample headroom.
// Package var so tests can shrink it to exercise the over-long-line branch.
var codexMetaMaxLine = 4 * 1024 * 1024

// codexSessions enumerates live codex sessions. Any failure degrades to an
// empty result with a stderr warning — codex problems must never break the
// claude listing.
func codexSessions(procs map[int]procInfo) []Session {
	pids := codexCandidatePIDs(procs)
	if len(pids) == 0 {
		return nil
	}
	dir, err := codexSessionsDirFn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "procsnap: resolving codex sessions dir: %v\n", err)
		return nil
	}
	raw, lsofErr := codexLsofFn(pids)
	if len(raw) == 0 {
		// lsof exits non-zero in some benign cases (e.g. a PID exiting
		// mid-scan) while still producing usable output, so an error only
		// matters when nothing came back at all.
		if lsofErr != nil {
			fmt.Fprintf(os.Stderr, "procsnap: lsof failed: %v\n", lsofErr)
		}
		return nil
	}
	byPID := parseCodexLsof(raw, dir)

	var out []Session
	for _, pid := range pids {
		cp := byPID[pid]
		for _, rollout := range cp.rollouts {
			id, started, ok := parseRolloutFilename(rollout)
			if !ok {
				continue
			}
			s := Session{
				Tool:      ToolCodex,
				PID:       pid,
				SessionID: id,
				CWD:       cp.cwd,
				StartedAt: started,
				Ancestors: walkAncestors(pid, procs),
			}
			if meta, err := readCodexSessionMeta(rollout); err == nil {
				s.SessionID = meta.ID
				s.Entrypoint = meta.Originator
				s.Version = meta.Version
				if !meta.StartedAt.IsZero() {
					s.StartedAt = meta.StartedAt
				}
				if meta.CWD != "" {
					s.CWD = meta.CWD
				}
			} else {
				// Filename fallback already populated id/start; cwd stays
				// the lsof-reported process cwd.
				fmt.Fprintf(os.Stderr, "procsnap: reading %s: %v\n", rollout, err)
			}
			out = append(out, s)
		}
	}
	return out
}

// codexCandidatePIDs returns the PIDs whose executable basename is "codex",
// sorted for deterministic output. This matches both the CLI TUI (comm
// "codex") and Codex.app's app-server (comm is the full bundle path).
func codexCandidatePIDs(procs map[int]procInfo) []int {
	var out []int
	for pid, p := range procs {
		if filepath.Base(p.Command) == "codex" {
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}

// codexProc is what lsof reveals about one candidate process.
type codexProc struct {
	cwd      string
	rollouts []string
}

// parseCodexLsof parses `lsof -F pfn` output into a pid-indexed map,
// keeping only the cwd record and open .jsonl files under sessionsDir.
// Rollout filename validation happens later, in codexSessions.
func parseCodexLsof(raw []byte, sessionsDir string) map[int]codexProc {
	out := map[int]codexProc{}
	prefix := sessionsDir + string(filepath.Separator)
	var pid int
	var fd string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			n, err := strconv.Atoi(line[1:])
			if err != nil {
				pid = 0
				continue
			}
			pid = n
		case 'f':
			fd = line[1:]
		case 'n':
			if pid == 0 {
				continue
			}
			name := line[1:]
			cp := out[pid]
			switch {
			case fd == "cwd":
				cp.cwd = name
			case strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".jsonl"):
				cp.rollouts = append(cp.rollouts, name)
			}
			out[pid] = cp
		}
	}
	return out
}

// rolloutUUIDLen is the length of the session UUID embedded in rollout
// filenames: rollout-<YYYY-MM-DDTHH-MM-SS>-<uuid>.jsonl.
const rolloutUUIDLen = 36

// parseRolloutFilename extracts the session ID and start time encoded in a
// rollout filename. The start time is best-effort (zero when unparseable,
// interpreted in local time — codex names files with the local clock); ok
// is false when the name does not carry a plausible session UUID.
func parseRolloutFilename(path string) (id string, started time.Time, ok bool) {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "rollout-") || !strings.HasSuffix(base, ".jsonl") {
		return "", time.Time{}, false
	}
	core := strings.TrimSuffix(strings.TrimPrefix(base, "rollout-"), ".jsonl")
	if len(core) <= rolloutUUIDLen {
		return "", time.Time{}, false
	}
	id = core[len(core)-rolloutUUIDLen:]
	ts := strings.TrimSuffix(core[:len(core)-rolloutUUIDLen], "-")
	started, _ = time.ParseInLocation("2006-01-02T15-04-05", ts, time.Local)
	return id, started, true
}

// codexSessionMeta is the subset of the rollout's first-line session_meta
// record that procsnap consumes.
type codexSessionMeta struct {
	ID         string
	CWD        string
	Version    string
	Originator string
	StartedAt  time.Time
}

// rawCodexMeta mirrors the JSON shape codex writes as the rollout's first line.
type rawCodexMeta struct {
	Type    string `json:"type"`
	Payload struct {
		ID         string `json:"id"`
		Timestamp  string `json:"timestamp"`
		CWD        string `json:"cwd"`
		Originator string `json:"originator"`
		CLIVersion string `json:"cli_version"`
	} `json:"payload"`
}

// readCodexSessionMeta reads and validates the first line of a rollout file.
func readCodexSessionMeta(path string) (codexSessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// The scanner's effective cap is max(initial cap, limit), so the initial
	// buffer must not exceed the limit when tests shrink it.
	sc.Buffer(make([]byte, 0, min(64*1024, codexMetaMaxLine)), codexMetaMaxLine)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return codexSessionMeta{}, err
		}
		return codexSessionMeta{}, fmt.Errorf("empty rollout file")
	}
	var raw rawCodexMeta
	if err := json.Unmarshal(sc.Bytes(), &raw); err != nil {
		return codexSessionMeta{}, fmt.Errorf("parsing session_meta: %w", err)
	}
	if raw.Type != "session_meta" || raw.Payload.ID == "" {
		return codexSessionMeta{}, fmt.Errorf("first line is not a session_meta record")
	}
	started, _ := time.Parse(time.RFC3339, raw.Payload.Timestamp)
	return codexSessionMeta{
		ID:         raw.Payload.ID,
		CWD:        raw.Payload.CWD,
		Version:    raw.Payload.CLIVersion,
		Originator: raw.Payload.Originator,
		StartedAt:  started,
	}, nil
}
