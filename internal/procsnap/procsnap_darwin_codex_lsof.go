//go:build darwin

package procsnap

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
)

type codexProc struct {
	cwd       string
	rollouts  []string
	threadIDs []string
}

func parseCodexLsof(raw []byte, sessionsDir, lockDir string) (map[int]codexProc, error) {
	out := map[int]codexProc{}
	prefix := sessionsDir + string(filepath.Separator)
	var pid int
	var fd string
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, min(64*1024, codexLsofMaxLine)), codexLsofMaxLine)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			fd = ""
			parsed, err := strconv.Atoi(line[1:])
			if err != nil {
				pid = 0
				continue
			}
			pid = parsed
		case 'f':
			fd = line[1:]
		case 'n':
			if pid == 0 {
				continue
			}
			name := line[1:]
			proc := out[pid]
			switch {
			case fd == "cwd":
				proc.cwd = name
			case strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".jsonl"):
				proc.rollouts = append(proc.rollouts, name)
			case filepath.Dir(name) == lockDir && strings.HasSuffix(name, ".lock"):
				id := strings.TrimSuffix(filepath.Base(name), ".lock")
				if len(id) == rolloutUUIDLen {
					proc.threadIDs = append(proc.threadIDs, id)
				}
			}
			out[pid] = proc
		}
	}
	return out, scanner.Err()
}
