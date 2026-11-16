package pi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// peekMaxLines caps how far List scans into a session file. Session headers,
// names, model changes and the first user turn all appear near the top;
// transcripts can be many megabytes, so List never reads them in full.
const peekMaxLines = 200

// maxLineBytes bounds a single JSONL line. pi transcripts contain large
// system prompts and tool outputs. A var so tests can force scanner errors
// without allocating a multi-megabyte line; tests that override it must not
// call t.Parallel.
var maxLineBytes = 16 * 1024 * 1024

// piHeader is the first line of every session file.
type piHeader struct {
	Type          string `json:"type"`
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"` // ISO 8601
	CWD           string `json:"cwd"`
	ParentSession string `json:"parentSession"`
}

// piEntry is one append-only line after the header. Only the fields omnisess
// consumes are decoded; unknown entry types are skipped.
type piEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"` // ISO 8601
	Provider  string          `json:"provider"`
	ModelID   string          `json:"modelId"`
	Name      string          `json:"name"`
	Message   json.RawMessage `json:"message"`
}

// agentMessage is the subset of pi's AgentMessage shape omnisess renders.
type agentMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Timestamp  int64           `json:"timestamp"` // unix milliseconds
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	IsError    bool            `json:"isError"`
	Command    string          `json:"command"`
	Output     string          `json:"output"`
	Usage      *struct {
		Input      int `json:"input"`
		Output     int `json:"output"`
		CacheRead  int `json:"cacheRead"`
		CacheWrite int `json:"cacheWrite"`
	} `json:"usage"`
}

// contentBlock is one element of an assistant or tool-result content array.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// piPeek is the cheap metadata List extracts from the top of a session file.
type piPeek struct {
	Preview string // first user message text
	Name    string // session_info name (pi --name)
	Model   string // provider/modelId from model_change
}

// piSessionMeta is the full-file metadata Get returns.
type piSessionMeta struct {
	Name      string
	Model     string
	UpdatedAt time.Time
}

// newScanner builds a JSONL scanner whose initial buffer respects the
// configured line ceiling, so tests can lower maxLineBytes and still force
// bufio.ErrTooLong.
func newScanner(f *os.File) *bufio.Scanner {
	initial := 64 * 1024
	if maxLineBytes < initial {
		initial = maxLineBytes
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initial), maxLineBytes)
	return scanner
}

// readSessionHeader reads and validates only the first line of a session file.
func readSessionHeader(path string) (*piHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	scanner := newScanner(f)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty session file")
	}

	var header piHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("parse session header: %w", err)
	}
	if header.Type != "session" || header.ID == "" {
		return nil, fmt.Errorf("not a pi session file")
	}
	return &header, nil
}

// peekSessionFile reads the session header plus the first user message, name
// and model, stopping as soon as all three are known or peekMaxLines is hit.
func peekSessionFile(path string) (*piHeader, piPeek, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, piPeek{}, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	scanner := newScanner(f)

	if !scanner.Scan() {
		return nil, piPeek{}, fmt.Errorf("empty session file")
	}
	var header piHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, piPeek{}, fmt.Errorf("parse session header: %w", err)
	}
	if header.Type != "session" || header.ID == "" {
		return nil, piPeek{}, fmt.Errorf("not a pi session file")
	}

	peek := piPeek{}
	for lines := 0; lines < peekMaxLines && scanner.Scan(); lines++ {
		var entry piEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines, keep scanning
		}
		switch entry.Type {
		case "session_info":
			peek.Name = entry.Name
		case "model_change":
			if peek.Model == "" {
				peek.Model = joinModel(entry.Provider, entry.ModelID)
			}
		case "message":
			if peek.Preview != "" {
				continue
			}
			var msg agentMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil || msg.Role != "user" {
				continue
			}
			peek.Preview = contentText(msg.Content)
		}
		if peek.Preview != "" && peek.Model != "" && peek.Name != "" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, piPeek{}, fmt.Errorf("scan session file: %w", err)
	}
	return &header, peek, nil
}

// parseSessionFile parses a whole session file. Tool results are attached to
// the assistant ToolCall they answer rather than emitted as their own message.
func parseSessionFile(path string) ([]model.Message, *piSessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	scanner := newScanner(f)

	if !scanner.Scan() {
		return nil, nil, fmt.Errorf("empty session file")
	}
	var header piHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, nil, fmt.Errorf("parse session header: %w", err)
	}
	if header.Type != "session" {
		return nil, nil, fmt.Errorf("not a pi session file")
	}

	type callRef struct {
		msg  int
		call int
	}
	callIndex := make(map[string]callRef)
	meta := &piSessionMeta{}
	var messages []model.Message

	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var entry piEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}

		switch entry.Type {
		case "session_info":
			if entry.Name != "" {
				meta.Name = entry.Name
			}
		case "model_change":
			meta.Model = joinModel(entry.Provider, entry.ModelID)
		case "message":
			var msg agentMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			ts := messageTime(msg.Timestamp, entry.Timestamp)
			if !ts.IsZero() && ts.After(meta.UpdatedAt) {
				meta.UpdatedAt = ts
			}

			switch msg.Role {
			case "user":
				text := contentText(msg.Content)
				if text == "" {
					continue
				}
				messages = append(messages, model.Message{
					Role:      model.RoleUser,
					Content:   text,
					Timestamp: ts,
				})

			case "assistant":
				assistant := model.Message{Role: model.RoleAssistant, Timestamp: ts}
				var texts []string
				for _, block := range decodeBlocks(msg.Content) {
					switch block.Type {
					case "text":
						if block.Text != "" {
							texts = append(texts, block.Text)
						}
					case "toolCall":
						assistant.ToolCalls = append(assistant.ToolCalls, model.ToolCall{
							ID:    block.ID,
							Name:  block.Name,
							Input: strings.TrimSpace(string(block.Arguments)),
						})
					}
				}
				assistant.Content = strings.Join(texts, "\n")
				if msg.Usage != nil {
					assistant.UsageInputTokens = msg.Usage.Input
					assistant.UsageOutputTokens = msg.Usage.Output
					assistant.UsageCacheReadInputTokens = msg.Usage.CacheRead
					assistant.UsageCacheCreationInputTokens = msg.Usage.CacheWrite
				}
				if meta.Model == "" && msg.Provider != "" && msg.Model != "" {
					meta.Model = msg.Provider + "/" + msg.Model
				}
				msgIdx := len(messages)
				for callIdx, call := range assistant.ToolCalls {
					if call.ID != "" {
						callIndex[call.ID] = callRef{msg: msgIdx, call: callIdx}
					}
				}
				messages = append(messages, assistant)

			case "toolResult":
				ref, ok := callIndex[msg.ToolCallID]
				if !ok {
					continue
				}
				call := &messages[ref.msg].ToolCalls[ref.call]
				call.Output = contentText(msg.Content)
				call.IsError = msg.IsError

			case "bashExecution":
				content := "$ " + msg.Command
				if msg.Output != "" {
					content += "\n" + msg.Output
				}
				messages = append(messages, model.Message{
					Role:      model.RoleTool,
					Content:   content,
					Timestamp: ts,
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan session file: %w", err)
	}
	return messages, meta, nil
}

// decodeBlocks decodes an array-of-blocks content field. A scalar string
// (user prompts may be stored that way) yields no blocks.
func decodeBlocks(raw json.RawMessage) []contentBlock {
	if len(raw) == 0 {
		return nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

// contentText returns the concatenated text of a content field, which pi
// writes either as a string or as an array of typed blocks.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var texts []string
	for _, block := range decodeBlocks(raw) {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// messageTime prefers the message's millisecond timestamp and falls back to
// the entry's ISO 8601 timestamp.
func messageTime(millis int64, iso string) time.Time {
	if millis != 0 {
		return time.UnixMilli(millis).UTC()
	}
	return parseISOTime(iso)
}

// parseISOTime parses pi's ISO 8601 timestamps. Invalid input yields the zero
// time.
func parseISOTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// joinModel renders "provider/model", tolerating either half being absent.
func joinModel(provider, id string) string {
	switch {
	case provider != "" && id != "":
		return provider + "/" + id
	case id != "":
		return id
	default:
		return ""
	}
}
