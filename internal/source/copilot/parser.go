package copilot

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// eventLine represents a single line in events.jsonl.
//
// Known event types include:
//   - "user.message"            — a user turn
//   - "assistant.message"       — an assistant turn (may stream as multiple
//     consecutive lines that should be concatenated into a single message)
//   - "tool.execution_start"    — skipped
//   - "tool.execution_complete" — skipped
type eventLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"` // ISO 8601
	Data      json.RawMessage `json:"data"`
}

// eventData carries the textual content of user / assistant messages.
type eventData struct {
	Content string `json:"content"`
}

// vscodeMetadata captures the fields we care about in vscode.metadata.json.
type vscodeMetadata struct {
	WorkspaceFolder string `json:"workspaceFolder"`
}

// sessionStateDir returns ~/.copilot/session-state.
func sessionStateDir(homeDir string) string {
	return filepath.Join(homeDir, ".copilot", "session-state")
}

// listSessionDirs returns the per-session directories under ~/.copilot/session-state.
// A directory qualifies only if it contains an events.jsonl file.
func listSessionDirs(homeDir string) ([]string, error) {
	root := sessionStateDir(homeDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read copilot session-state dir: %w", err)
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		eventsPath := filepath.Join(root, e.Name(), "events.jsonl")
		if _, err := os.Stat(eventsPath); err != nil {
			continue
		}
		dirs = append(dirs, filepath.Join(root, e.Name()))
	}
	return dirs, nil
}

// readMetadata returns the workspaceFolder recorded in vscode.metadata.json
// for a session-state directory. Returns "" if the file is missing or malformed.
func readMetadata(sessionDir string) string {
	path := filepath.Join(sessionDir, "vscode.metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var meta vscodeMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.WorkspaceFolder
}

// peekFirstUserMessage returns the content of the first user.message event
// in the session's events.jsonl. Used as a preview / title without parsing
// the full file. Returns "" if no user message is found or the file is empty.
func peekFirstUserMessage(eventsPath string) string {
	f, err := os.Open(eventsPath)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev eventLine
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type != "user.message" {
			continue
		}
		var data eventData
		if err := json.Unmarshal(ev.Data, &data); err != nil {
			continue
		}
		text := strings.TrimSpace(data.Content)
		if text != "" {
			return text
		}
	}
	return ""
}

// peekFirstTimestamp returns the timestamp of the first event with a parseable
// timestamp. Used as StartedAt without parsing the full file.
func peekFirstTimestamp(eventsPath string) time.Time {
	f, err := os.Open(eventsPath)
	if err != nil {
		return time.Time{}
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev eventLine
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		ts := parseCopilotTimestamp(ev.Timestamp)
		if !ts.IsZero() {
			return ts
		}
	}
	return time.Time{}
}

// parseEvents reads events.jsonl and returns all conversation messages.
// Consecutive assistant.message events are merged into a single message
// to mirror the streaming behaviour of the Copilot CLI.
func parseEvents(eventsPath string) ([]model.Message, error) {
	f, err := os.Open(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("open copilot events file %s: %w", eventsPath, err)
	}
	defer f.Close() //nolint:errcheck

	var messages []model.Message

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev eventLine
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}

		role := mapEventRole(ev.Type)
		if role == "" {
			continue
		}

		var data eventData
		if err := json.Unmarshal(ev.Data, &data); err != nil {
			continue
		}
		content := strings.TrimSpace(data.Content)
		if content == "" {
			continue
		}

		ts := parseCopilotTimestamp(ev.Timestamp)

		// Merge consecutive assistant.message events (streaming).
		if role == model.RoleAssistant && len(messages) > 0 &&
			messages[len(messages)-1].Role == model.RoleAssistant {
			last := &messages[len(messages)-1]
			last.Content += content
			if !ts.IsZero() {
				last.Timestamp = ts
			}
			continue
		}

		messages = append(messages, model.Message{
			Role:      role,
			Content:   content,
			Timestamp: ts,
		})
	}

	if err := scanner.Err(); err != nil {
		return messages, fmt.Errorf("scan copilot events file %s: %w", eventsPath, err)
	}

	return messages, nil
}

// mapEventRole maps an events.jsonl event type to a model.Role.
// Unknown types return "" so the caller skips the event.
func mapEventRole(t string) model.Role {
	switch t {
	case "user.message":
		return model.RoleUser
	case "assistant.message":
		return model.RoleAssistant
	default:
		return ""
	}
}

// parseCopilotTimestamp parses an ISO 8601 timestamp string from an events.jsonl line.
func parseCopilotTimestamp(s string) time.Time {
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
