package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/psacconier/sessions/internal/model"
)

// historyEntry represents a single line in ~/.claude/history.jsonl.
type historyEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"` // Unix milliseconds
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

// sessionLine represents a single line in a session JSONL file.
type sessionLine struct {
	Type       string          `json:"type"`
	Message    json.RawMessage `json:"message"`
	UUID       string          `json:"uuid"`
	Timestamp  string          `json:"timestamp"` // ISO 8601
	Model      string          `json:"model"`
	CostUSD    float64         `json:"costUSD"`
	DurationMs int64           `json:"durationMs"`
	StopReason string          `json:"stopReason"`
	CWD        string          `json:"cwd"`
	GitBranch  string          `json:"gitBranch"`
}

// messagePayload holds the role and content from the "message" field.
type messagePayload struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// parseHistoryLine parses a single line from history.jsonl into a historyEntry.
func parseHistoryLine(line []byte) (historyEntry, error) {
	var entry historyEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return entry, fmt.Errorf("parse history line: %w", err)
	}
	return entry, nil
}

// parseSessionFile reads a session JSONL file and returns parsed messages,
// the model used, and the git branch (from the first line that has one).
func parseSessionFile(path string) ([]model.Message, string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("open session file %s: %w", path, err)
	}
	defer f.Close()

	var messages []model.Message
	var sessionModel string
	var gitBranch string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB lines
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var sl sessionLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue // skip malformed lines
		}

		// Skip summary lines
		if sl.Type == "summary" {
			continue
		}

		// Only process user and assistant messages
		if sl.Type != "user" && sl.Type != "assistant" {
			continue
		}

		// Capture git branch from first line that has one
		if gitBranch == "" && sl.GitBranch != "" {
			gitBranch = sl.GitBranch
		}

		// Capture model from assistant messages
		if sl.Type == "assistant" && sessionModel == "" && sl.Model != "" {
			sessionModel = sl.Model
		}

		// Parse the message payload
		var payload messagePayload
		if err := json.Unmarshal(sl.Message, &payload); err != nil {
			continue
		}

		ts := parseTimestamp(sl.Timestamp)

		role := model.Role(payload.Role)
		content := extractContent(payload.Content)

		msg := model.Message{
			Role:      role,
			Content:   content,
			Timestamp: ts,
		}

		// Extract tool calls from assistant content blocks
		if sl.Type == "assistant" {
			msg.ToolCalls = extractToolCalls(payload.Content)
		}

		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return messages, sessionModel, gitBranch, fmt.Errorf("scan session file %s: %w", path, err)
	}

	return messages, sessionModel, gitBranch, nil
}

// extractContent handles both string content and array-of-blocks content.
// Returns concatenated text from all text blocks.
func extractContent(content interface{}) string {
	if content == nil {
		return ""
	}

	// Try string first
	if s, ok := content.(string); ok {
		return s
	}

	// Try array of content blocks
	blocks, ok := content.([]interface{})
	if !ok {
		return ""
	}

	var parts []string
	for _, block := range blocks {
		m, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := m["type"].(string)
		if blockType == "text" {
			if text, ok := m["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// extractToolCalls extracts tool_use blocks from assistant content.
func extractToolCalls(content interface{}) []model.ToolCall {
	if content == nil {
		return nil
	}

	blocks, ok := content.([]interface{})
	if !ok {
		return nil
	}

	var calls []model.ToolCall
	for _, block := range blocks {
		m, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := m["type"].(string)
		if blockType == "tool_use" {
			name, _ := m["name"].(string)
			inputRaw, _ := json.Marshal(m["input"])
			input := string(inputRaw)
			if len(input) > 200 {
				input = input[:200] + "..."
			}
			calls = append(calls, model.ToolCall{
				Name:  name,
				Input: input,
			})
		}
	}
	return calls
}

// parseTimestamp parses an ISO 8601 timestamp string.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try standard formats
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

// projectDirName converts an absolute project path to the Claude directory name.
// /Users/foo/bar -> -Users-foo-bar
func projectDirName(projectPath string) string {
	return strings.ReplaceAll(projectPath, "/", "-")
}

// projectPathFromDir converts a Claude project directory name back to an absolute path.
// -Users-foo-bar -> /Users/foo/bar
func projectPathFromDir(dirName string) string {
	if dirName == "" {
		return ""
	}
	// The dir name starts with "-" which corresponds to the leading "/"
	// Replace all "-" with "/"
	return strings.ReplaceAll(dirName, "-", "/")
}
