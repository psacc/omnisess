package claude

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

// historyEntry represents a single line in ~/.claude/history.jsonl.
type historyEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"` // Unix milliseconds
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

// sessionLine represents a single line in a session JSONL file.
type sessionLine struct {
	Type          string          `json:"type"`
	Message       json.RawMessage `json:"message"`
	UUID          string          `json:"uuid"`
	Timestamp     string          `json:"timestamp"` // ISO 8601
	Model         string          `json:"model"`
	CostUSD       float64         `json:"costUSD"`
	DurationMs    int64           `json:"durationMs"`
	StopReason    string          `json:"stopReason"`
	CWD           string          `json:"cwd"`
	GitBranch     string          `json:"gitBranch"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

// messagePayload holds the role, content, model, and usage block from the
// "message" field. Newer Claude Code versions (~2.1.143+) put `model` inside
// the message payload rather than at the top level of the JSONL line.
type messagePayload struct {
	Role    string          `json:"role"`
	Content interface{}     `json:"content"`
	Model   string          `json:"model"`
	Usage   json.RawMessage `json:"usage"`
}

// usagePayload holds the usage block from an assistant message.
type usagePayload struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
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
//
// Two-pass: first pass builds Messages and remembers which assistant ToolCalls
// each line carries; second pass walks user lines for tool_result blocks and
// the per-line `toolUseResult` payload, pairing them back to the preceding
// assistant call by tool_use_id.
func parseSessionFile(path string) ([]model.Message, string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("open session file %s: %w", path, err)
	}
	defer f.Close()

	var messages []model.Message
	var sessionModel string
	var gitBranch string

	// toolCallIndex maps tool_use_id → (messageIdx, callIdx) so the second pass
	// can patch ToolCall.Output / IsError / file fields by ID.
	type idx struct {
		msg  int
		call int
	}
	toolCallIndex := make(map[string]idx)

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

		// Parse the message payload
		var payload messagePayload
		if err := json.Unmarshal(sl.Message, &payload); err != nil {
			continue
		}

		// Capture model from assistant messages: prefer top-level `model`
		// (older Claude) then fall back to `message.model` (newer Claude
		// Code ~2.1.143+).
		if sl.Type == "assistant" && sessionModel == "" {
			switch {
			case sl.Model != "":
				sessionModel = sl.Model
			case payload.Model != "":
				sessionModel = payload.Model
			}
		}

		ts := parseTimestamp(sl.Timestamp)

		role := model.Role(payload.Role)
		content := extractContent(payload.Content)

		msg := model.Message{
			Role:      role,
			Content:   content,
			Timestamp: ts,
		}

		// Capture usage tokens from assistant messages.
		if sl.Type == "assistant" && len(payload.Usage) > 0 {
			var u usagePayload
			if err := json.Unmarshal(payload.Usage, &u); err == nil {
				msg.UsageInputTokens = u.InputTokens
				msg.UsageOutputTokens = u.OutputTokens
				msg.UsageCacheCreationInputTokens = u.CacheCreationInputTokens
				msg.UsageCacheReadInputTokens = u.CacheReadInputTokens
			}
		}

		// Extract tool calls from assistant content blocks
		if sl.Type == "assistant" {
			msg.ToolCalls = extractToolCalls(payload.Content)
			mIdx := len(messages)
			for cIdx, tc := range msg.ToolCalls {
				if tc.ID != "" {
					toolCallIndex[tc.ID] = idx{msg: mIdx, call: cIdx}
				}
			}
		}

		// On user lines: pair tool_result blocks (and the per-line
		// toolUseResult JSON field) back to the preceding assistant ToolCall.
		if sl.Type == "user" {
			results := extractToolResults(payload.Content)
			isErrors := extractToolResultErrors(payload.Content)
			// toolUseResult (sibling field) — prefer it over tool_result.content.
			var tureID string
			// The toolUseResult field doesn't carry the tool_use_id itself;
			// in real Claude JSONL, a user line contains at most one tool_result
			// block (paired with one toolUseResult). Pull the id from the single
			// tool_result if present.
			if len(results) == 1 {
				for id := range results {
					tureID = id
					break
				}
			}
			for id, output := range results {
				ref, ok := toolCallIndex[id]
				if !ok {
					continue
				}
				tc := &messages[ref.msg].ToolCalls[ref.call]
				tc.Output = output
				if isErrors[id] {
					tc.IsError = true
				}
			}
			if len(sl.ToolUseResult) > 0 && tureID != "" {
				ref, ok := toolCallIndex[tureID]
				if ok {
					tc := &messages[ref.msg].ToolCalls[ref.call]
					applyToolUseResult(tc, sl.ToolUseResult)
				}
			}
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
// Captures ID, raw input JSON, name, and pre-fills file-I/O fields for
// Read/Write/Edit. file metadata is later patched in via applyToolUseResult.
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
		if blockType != "tool_use" {
			continue
		}
		name, _ := m["name"].(string)
		id, _ := m["id"].(string)
		inputAny := m["input"]
		inputRaw, _ := json.Marshal(inputAny)
		tc := model.ToolCall{
			Name:  name,
			ID:    id,
			Input: string(inputRaw),
		}
		// File-I/O extraction from tool_use.input for Read/Write/Edit.
		applyFileFromInput(&tc, inputAny)
		calls = append(calls, tc)
	}
	return calls
}

// extractToolResults walks user-message content blocks and returns a map of
// tool_use_id → tool_result.content rendered as a JSON string. Used as the
// fallback when no `toolUseResult` sibling field is present.
func extractToolResults(content interface{}) map[string]string {
	out := make(map[string]string)
	blocks, ok := content.([]interface{})
	if !ok {
		return out
	}
	for _, block := range blocks {
		m, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] != "tool_result" {
			continue
		}
		id, _ := m["tool_use_id"].(string)
		if id == "" {
			continue
		}
		raw, _ := json.Marshal(m["content"])
		out[id] = string(raw)
	}
	return out
}

// extractToolResultErrors returns a set of tool_use_ids whose tool_result
// block has `is_error: true`.
func extractToolResultErrors(content interface{}) map[string]bool {
	out := make(map[string]bool)
	blocks, ok := content.([]interface{})
	if !ok {
		return out
	}
	for _, block := range blocks {
		m, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] != "tool_result" {
			continue
		}
		id, _ := m["tool_use_id"].(string)
		if id == "" {
			continue
		}
		if isErr, ok := m["is_error"].(bool); ok && isErr {
			out[id] = true
		}
	}
	return out
}

// applyFileFromInput pre-fills file-I/O fields based on tool name and the
// `file_path`/`content` keys of `tool_use.input`. Only handles Read, Write,
// Edit (no Bash heuristics in PR1).
func applyFileFromInput(tc *model.ToolCall, input interface{}) {
	m, ok := input.(map[string]interface{})
	if !ok {
		return
	}
	switch tc.Name {
	case "Read":
		if fp, ok := m["file_path"].(string); ok {
			tc.FilePath = fp
		}
		tc.FileOp = "read"
	case "Write":
		if fp, ok := m["file_path"].(string); ok {
			tc.FilePath = fp
		}
		tc.FileOp = "write"
		if c, ok := m["content"].(string); ok {
			tc.FileContentSize = len(c)
		}
	case "Edit":
		if fp, ok := m["file_path"].(string); ok {
			tc.FilePath = fp
		}
		tc.FileOp = "edit"
	}
}

// applyToolUseResult patches a ToolCall with data from the per-line
// `toolUseResult` JSONL field. Handles three shapes:
//   - Bare JSON string starting with "Error:" — sets IsError=true and stores
//     the string as Output (JSON-quoted).
//   - Object with `success: false` — sets IsError=true.
//   - Object with file fields (filePath / file.filePath / structuredPatch) —
//     fills/overrides ToolCall file fields.
//
// In all cases the raw JSON becomes ToolCall.Output (preferring the rich
// toolUseResult over tool_result.content).
func applyToolUseResult(tc *model.ToolCall, raw json.RawMessage) {
	tc.Output = string(raw)
	if len(raw) == 0 {
		return
	}
	// First, try to decode as a string (bare-string error case).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.HasPrefix(s, "Error:") {
			tc.IsError = true
		}
		return
	}
	// Then try as an object.
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}
	// success: false → error.
	if v, ok := obj["success"].(bool); ok && !v {
		tc.IsError = true
	}
	// Prefer top-level filePath for Write/Edit; nested file.filePath for Read.
	switch tc.Name {
	case "Read":
		if fileObj, ok := obj["file"].(map[string]interface{}); ok {
			if fp, ok := fileObj["filePath"].(string); ok && fp != "" {
				tc.FilePath = fp
			}
		}
	case "Write", "Edit":
		if fp, ok := obj["filePath"].(string); ok && fp != "" {
			tc.FilePath = fp
		}
	}
	// structuredPatch line counts (Edit only).
	if tc.Name == "Edit" {
		if patches, ok := obj["structuredPatch"].([]interface{}); ok {
			added, removed := countStructuredPatchLines(patches)
			tc.FileLinesAdded = added
			tc.FileLinesRemoved = removed
		}
	}
}

// countStructuredPatchLines walks a toolUseResult.structuredPatch array
// (each hunk has a "lines" key with prefixes " ", "+", "-") and returns
// the (added, removed) totals.
func countStructuredPatchLines(patches []interface{}) (int, int) {
	added, removed := 0, 0
	for _, p := range patches {
		hunk, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		lines, ok := hunk["lines"].([]interface{})
		if !ok {
			continue
		}
		for _, l := range lines {
			s, ok := l.(string)
			if !ok || s == "" {
				continue
			}
			switch s[0] {
			case '+':
				added++
			case '-':
				removed++
			}
		}
	}
	return added, removed
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
// /Users/example/prj/foo -> -Users-example-prj-foo
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
// -Users-example-prj-my-app -> /Users/example/prj/my-app
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
