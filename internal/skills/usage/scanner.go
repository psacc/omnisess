// Package usage scans Claude Code session JSONL transcripts for skill invocations.
package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

// commandNameRE matches a slash-command invocation. Skill names may contain
// alphanumerics, hyphens, underscores, and one optional plugin colon prefix.
var commandNameRE = regexp.MustCompile(`<command-name>/([A-Za-z0-9_:-]+)</command-name>`)

// ScanOptions configures Scan.
type ScanOptions struct {
	Files []string  // explicit list of JSONL files to scan
	Since time.Time // discard invocations strictly before this timestamp; zero = no filter
}

// Scan reads all JSONL files and returns merged invocations matching options.
func Scan(opts ScanOptions) ([]skills.Invocation, error) {
	var out []skills.Invocation
	for _, f := range opts.Files {
		invs, err := scanFile(f)
		if err != nil {
			return nil, err
		}
		for _, inv := range invs {
			if !opts.Since.IsZero() && inv.Timestamp.Before(opts.Since) {
				continue
			}
			out = append(out, inv)
		}
	}
	return out, nil
}

type rawLine struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
}

type rawAssistantMsg struct {
	Content []rawContentBlock `json:"content"`
}

type rawContentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type rawUserMsg struct {
	Content interface{} `json:"content"` // may be string or array
}

type skillInput struct {
	Skill string `json:"skill"`
}

func scanFile(path string) ([]skills.Invocation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var out []skills.Invocation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024) // sessions can have large lines
	for sc.Scan() {
		var line rawLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue // skip malformed lines silently (real sessions sometimes have control chars)
		}
		ts := parseTimestamp(line.Timestamp)
		switch line.Type {
		case "assistant":
			var msg rawAssistantMsg
			if err := json.Unmarshal(line.Message, &msg); err != nil {
				continue
			}
			for _, b := range msg.Content {
				if b.Type != "tool_use" || b.Name != "Skill" {
					continue
				}
				var inp skillInput
				if err := json.Unmarshal(b.Input, &inp); err != nil {
					continue
				}
				if inp.Skill == "" {
					continue
				}
				out = append(out, skills.Invocation{
					SkillName: inp.Skill,
					Timestamp: ts,
					Kind:      skills.InvocationModel,
					SessionID: line.SessionID,
				})
			}
		case "user":
			var msg rawUserMsg
			if err := json.Unmarshal(line.Message, &msg); err != nil {
				continue
			}
			var content string
			switch v := msg.Content.(type) {
			case string:
				content = v
			case []interface{}:
				var sb strings.Builder
				for _, item := range v {
					m, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
						sb.WriteString("\n")
					}
				}
				content = sb.String()
			default:
				continue
			}
			for _, m := range commandNameRE.FindAllStringSubmatch(content, -1) {
				out = append(out, skills.Invocation{
					SkillName: m[1],
					Timestamp: ts,
					Kind:      skills.InvocationUser,
					SessionID: line.SessionID,
				})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return out, nil
}

func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// FindSessionFiles returns all *.jsonl files under projectsRoot at any depth.
func FindSessionFiles(projectsRoot string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(projectsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
