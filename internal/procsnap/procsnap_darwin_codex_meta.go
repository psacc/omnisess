//go:build darwin

package procsnap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const rolloutUUIDLen = 36

func parseRolloutFilename(path string) (string, time.Time, bool) {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "rollout-") || !strings.HasSuffix(base, ".jsonl") {
		return "", time.Time{}, false
	}
	core := strings.TrimSuffix(strings.TrimPrefix(base, "rollout-"), ".jsonl")
	if len(core) <= rolloutUUIDLen {
		return "", time.Time{}, false
	}
	id := core[len(core)-rolloutUUIDLen:]
	timestamp := strings.TrimSuffix(core[:len(core)-rolloutUUIDLen], "-")
	started, _ := time.ParseInLocation("2006-01-02T15-04-05", timestamp, time.Local)
	return id, started, true
}

type codexSessionMeta struct {
	ID         string
	CWD        string
	Version    string
	Originator string
	StartedAt  time.Time
}

func applyCodexMeta(session *Session, meta codexSessionMeta) {
	session.SessionID = meta.ID
	session.Entrypoint = meta.Originator
	session.Version = meta.Version
	if !meta.StartedAt.IsZero() {
		session.StartedAt = meta.StartedAt
	}
	if meta.CWD != "" {
		session.CWD = meta.CWD
	}
}

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

func readCodexSessionMeta(path string) (codexSessionMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, min(64*1024, codexMetaMaxLine)), codexMetaMaxLine)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return codexSessionMeta{}, err
		}
		return codexSessionMeta{}, fmt.Errorf("empty rollout file")
	}
	var raw rawCodexMeta
	if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
		return codexSessionMeta{}, fmt.Errorf("parsing session_meta: %w", err)
	}
	if raw.Type != "session_meta" || raw.Payload.ID == "" {
		return codexSessionMeta{}, fmt.Errorf("first line is not a session_meta record")
	}
	started, _ := time.Parse(time.RFC3339, raw.Payload.Timestamp)
	return codexSessionMeta{ID: raw.Payload.ID, CWD: raw.Payload.CWD, Version: raw.Payload.CLIVersion,
		Originator: raw.Payload.Originator, StartedAt: started}, nil
}
