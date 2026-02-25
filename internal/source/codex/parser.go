package codex

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

// historyEntry represents a single line in ~/.codex/history.jsonl.
type historyEntry struct {
	SessionID string `json:"session_id"` // UUID
	Ts        int64  `json:"ts"`         // Unix seconds
	Text      string `json:"text"`       // user prompt (truncated)
}

// sessionLine represents a single line in a Codex session JSONL file.
// Lines are one of: session_meta, response_item, event_msg, or other (skipped).
type sessionLine struct {
	Timestamp string          `json:"timestamp"` // ISO 8601
	Type      string          `json:"type"`      // "session_meta", "response_item", "event_msg", ...
	Payload   json.RawMessage `json:"payload"`
}

// sessionMetaPayload holds the fields from a session_meta line's payload.
type sessionMetaPayload struct {
	CWD string `json:"cwd"`
}

// responseItemPayload holds the fields from a response_item line's payload.
// Only lines with payload.type == "message" carry conversation content.
type responseItemPayload struct {
	Type    string            `json:"type"` // "message", etc.
	Role    string            `json:"role"` // "developer" → RoleUser, "assistant" → RoleAssistant
	Content []responseContent `json:"content"`
}

// responseContent is a single element of a response_item payload's content array.
type responseContent struct {
	Type string `json:"type"` // "input_text", "text", etc.
	Text string `json:"text"`
}

// eventMsgPayload holds the fields from an event_msg line's payload.
// payload.type is "user_message" or "agent_message"; payload.message is the text.
type eventMsgPayload struct {
	Type    string `json:"type"`    // "user_message" or "agent_message"
	Message string `json:"message"` // simple string content
}

// parseHistoryLine parses a single line from ~/.codex/history.jsonl.
func parseHistoryLine(line []byte) (*historyEntry, error) {
	var entry historyEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil, fmt.Errorf("parse codex history line: %w", err)
	}
	return &entry, nil
}

// findSessionFile locates the JSONL session file for a given session ID by
// globbing ~/.codex/sessions/YYYY/MM/DD/*-<sessionID>.jsonl.
// The path has 4 levels under sessions/ (year/month/day/file), so the pattern
// uses 4 wildcards. Returns the first match or empty string if not found.
func findSessionFile(homeDir, sessionID string) string {
	pattern := filepath.Join(homeDir, ".codex", "sessions", "*", "*", "*", "*-"+sessionID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// extractSessionIDFromPath extracts the UUID from a Codex session file path.
// Path pattern: .../rollout-<datetime>-<uuid>.jsonl
// The UUID is everything after the last '-' in the base filename (without .jsonl),
// but Codex UUIDs are formatted as 8-4-4-4-12 hex so we split on the first '-'
// after "rollout-<datetime>-" prefix.
//
// Simpler: the session ID is the last 36 characters of the stem (UUID length).
func extractSessionIDFromPath(path string) string {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, ".jsonl")
	// UUID is 36 chars: 8-4-4-4-12 with dashes = 32 hex + 4 dashes
	if len(stem) >= 36 {
		candidate := stem[len(stem)-36:]
		// Basic UUID format check: position 8, 13, 18, 23 should be '-'
		if len(candidate) == 36 &&
			candidate[8] == '-' && candidate[13] == '-' &&
			candidate[18] == '-' && candidate[23] == '-' {
			return candidate
		}
	}
	return stem
}

// parseSessionFile reads a Codex session JSONL file and returns all conversation
// messages and the cwd (project path from session_meta). It handles both
// response_item and event_msg line types.
func parseSessionFile(path string) ([]model.Message, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open codex session file %s: %w", path, err)
	}
	defer f.Close()

	var messages []model.Message
	var cwd string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var sl sessionLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue // skip malformed lines
		}

		ts := parseCodexTimestamp(sl.Timestamp)

		switch sl.Type {
		case "session_meta":
			if cwd == "" {
				var meta sessionMetaPayload
				if err := json.Unmarshal(sl.Payload, &meta); err == nil {
					cwd = meta.CWD
				}
			}

		case "response_item":
			var rip responseItemPayload
			if err := json.Unmarshal(sl.Payload, &rip); err != nil {
				continue
			}
			if rip.Type != "message" {
				continue
			}
			role := mapResponseItemRole(rip.Role)
			if role == "" {
				continue
			}
			content := extractResponseContent(rip.Content)
			messages = append(messages, model.Message{
				Role:      role,
				Content:   content,
				Timestamp: ts,
			})

		case "event_msg":
			var emp eventMsgPayload
			if err := json.Unmarshal(sl.Payload, &emp); err != nil {
				continue
			}
			role := mapEventMsgRole(emp.Type)
			if role == "" {
				continue
			}
			if emp.Message == "" {
				continue
			}
			messages = append(messages, model.Message{
				Role:      role,
				Content:   emp.Message,
				Timestamp: ts,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return messages, cwd, fmt.Errorf("scan codex session file %s: %w", path, err)
	}

	return messages, cwd, nil
}

// mapResponseItemRole maps a response_item payload role to a model.Role.
// "developer" → RoleUser, "assistant" → RoleAssistant, others → "".
func mapResponseItemRole(role string) model.Role {
	switch role {
	case "developer":
		return model.RoleUser
	case "assistant":
		return model.RoleAssistant
	default:
		return ""
	}
}

// mapEventMsgRole maps an event_msg payload type to a model.Role.
// "user_message" → RoleUser, "agent_message" → RoleAssistant, others → "".
func mapEventMsgRole(msgType string) model.Role {
	switch msgType {
	case "user_message":
		return model.RoleUser
	case "agent_message":
		return model.RoleAssistant
	default:
		return ""
	}
}

// extractResponseContent concatenates text from a response_item content array.
func extractResponseContent(content []responseContent) string {
	var parts []string
	for _, c := range content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// parseCodexTimestamp parses an ISO 8601 timestamp string from a Codex session file.
func parseCodexTimestamp(s string) time.Time {
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
