package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// /Users/paolo.sacconier/prj/foo -> -Users-paolo-sacconier-prj-foo
func projectDirName(projectPath string) string {
	encoded := strings.ReplaceAll(projectPath, "/", "-")
	encoded = strings.ReplaceAll(encoded, ".", "-")
	return encoded
}

// encodeDirComponent encodes a single directory component the way Claude does:
// replace "." with "-".
func encodeDirComponent(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

// projectPathFromDir converts a Claude project directory name back to an absolute path.
// -Users-paolo-sacconier-prj-finn-b2b-orders-api -> /Users/paolo.sacconier/prj/finn/b2b-orders-api
//
// Because Claude encodes both "/" and "." as "-", naive replacement corrupts
// paths that contain hyphens or dots. This function uses a greedy filesystem
// walk: at each directory level it reads the actual children and matches the
// longest encoded prefix, correctly resolving ambiguity.
//
// Falls back to naive decode if the greedy walk fails (e.g., directory no
// longer exists on disk).
func projectPathFromDir(dirName string) string {
	if dirName == "" {
		return ""
	}

	// Strip the leading "-" that corresponds to the root "/".
	encoded := strings.TrimPrefix(dirName, "-")
	if encoded == "" {
		return "/"
	}

	// Use the home directory as an anchor to speed up the walk:
	// most paths start with the home dir.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		homeEncoded := encodePathForClaude(homeDir)
		if encoded == homeEncoded {
			return homeDir
		}
		if strings.HasPrefix(encoded, homeEncoded+"-") {
			suffix := strings.TrimPrefix(encoded, homeEncoded+"-")
			if result := resolvePathGreedyClaude(homeDir, suffix); result != "" {
				return result
			}
		}
	}

	// Full walk from root.
	if result := resolvePathGreedyClaude("/", encoded); result != "" {
		return result
	}

	// Fallback: naive decode (replace all "-" with "/").
	return "/" + strings.ReplaceAll(encoded, "-", "/")
}

// encodePathForClaude encodes a path the way Claude does:
// strip leading "/", replace "/" and "." with "-".
func encodePathForClaude(p string) string {
	encoded := strings.TrimPrefix(p, "/")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	encoded = strings.ReplaceAll(encoded, ".", "-")
	return encoded
}

// resolvePathGreedyClaude walks the filesystem greedily to decode an encoded
// suffix back into a real path. At each directory level it lists children,
// encodes each child name, and picks the longest match against the remaining
// encoded string. This correctly handles directories whose real names contain
// hyphens or dots.
func resolvePathGreedyClaude(base string, encoded string) string {
	if encoded == "" {
		return base
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}

	type candidate struct {
		path      string
		remaining string
	}
	var candidates []candidate

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childEncoded := encodeDirComponent(e.Name())
		if encoded == childEncoded {
			// Exact full match.
			return filepath.Join(base, e.Name())
		}
		if strings.HasPrefix(encoded, childEncoded+"-") {
			remaining := strings.TrimPrefix(encoded, childEncoded+"-")
			candidates = append(candidates, candidate{
				path:      filepath.Join(base, e.Name()),
				remaining: remaining,
			})
		}
	}

	// Sort by longest match first (shortest remaining).
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if len(candidates[j].remaining) < len(candidates[i].remaining) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	for _, c := range candidates {
		if result := resolvePathGreedyClaude(c.path, c.remaining); result != "" {
			return result
		}
	}

	return ""
}
