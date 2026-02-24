package cursor

import (
	"bufio"
	"os"
	"strings"

	"github.com/psacc/omnisess/internal/model"
)

// parseTranscript reads a Cursor agent transcript file and returns structured messages.
//
// Format: plain text with role markers on their own line:
//   - "user:" or "User:" starts a user message (content on subsequent lines)
//   - "assistant:" or "A:" starts an assistant message
//   - "[Tool call: <name>]" starts a tool call block (attaches to preceding assistant)
//   - "[Tool result]" starts a tool result block (attaches to preceding tool call)
//
// Empty lines between sections are treated as separators.
func parseTranscript(path string) ([]model.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type state int
	const (
		stateNone state = iota
		stateUser
		stateAssistant
		stateToolCall
		stateToolResult
	)

	var (
		messages    []model.Message
		current     state
		contentBuf  strings.Builder
		toolCallBuf strings.Builder
		toolResBuf  strings.Builder
		toolName    string
	)

	flushContent := func() {
		text := strings.TrimSpace(contentBuf.String())
		if text == "" && current == stateNone {
			return
		}

		switch current {
		case stateUser:
			messages = append(messages, model.Message{
				Role:    model.RoleUser,
				Content: text,
			})
		case stateAssistant:
			messages = append(messages, model.Message{
				Role:    model.RoleAssistant,
				Content: text,
			})
		case stateToolCall:
			// Attach tool call to the last assistant message, or create one.
			call := model.ToolCall{
				Name:  toolName,
				Input: strings.TrimSpace(toolCallBuf.String()),
			}
			if len(messages) > 0 && messages[len(messages)-1].Role == model.RoleAssistant {
				messages[len(messages)-1].ToolCalls = append(messages[len(messages)-1].ToolCalls, call)
			} else {
				// Orphan tool call — wrap in an assistant message.
				messages = append(messages, model.Message{
					Role:      model.RoleAssistant,
					ToolCalls: []model.ToolCall{call},
				})
			}
			toolCallBuf.Reset()
			toolName = ""
		case stateToolResult:
			// Attach result to the last tool call of the last assistant message.
			result := strings.TrimSpace(toolResBuf.String())
			if len(messages) > 0 {
				last := &messages[len(messages)-1]
				if last.Role == model.RoleAssistant && len(last.ToolCalls) > 0 {
					last.ToolCalls[len(last.ToolCalls)-1].Output = result
				}
			}
			toolResBuf.Reset()
		}

		contentBuf.Reset()
		current = stateNone
	}

	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially large transcript lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for role markers.
		switch {
		case isUserMarker(trimmed):
			flushContent()
			current = stateUser
			// Content after "user:" on the same line.
			after := extractAfterMarker(trimmed, "user:")
			if after == "" {
				after = extractAfterMarker(trimmed, "User:")
			}
			if after != "" {
				contentBuf.WriteString(after)
				contentBuf.WriteByte('\n')
			}

		case isAssistantMarker(trimmed):
			flushContent()
			current = stateAssistant
			after := extractAfterMarker(trimmed, "assistant:")
			if after == "" {
				after = extractAfterMarker(trimmed, "A:")
			}
			if after != "" {
				contentBuf.WriteString(after)
				contentBuf.WriteByte('\n')
			}

		case isToolCallMarker(trimmed):
			flushContent()
			current = stateToolCall
			toolName = extractToolCallName(trimmed)

		case isToolResultMarker(trimmed):
			flushContent()
			current = stateToolResult

		default:
			// Accumulate content for the current section.
			switch current {
			case stateUser, stateAssistant:
				contentBuf.WriteString(line)
				contentBuf.WriteByte('\n')
			case stateToolCall:
				toolCallBuf.WriteString(line)
				toolCallBuf.WriteByte('\n')
			case stateToolResult:
				toolResBuf.WriteString(line)
				toolResBuf.WriteByte('\n')
			}
		}
	}

	// Flush any remaining section.
	flushContent()

	// Clean up Cursor-specific markup from message content.
	for i := range messages {
		messages[i].Content = cleanCursorMarkup(messages[i].Content)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func isUserMarker(s string) bool {
	return s == "user:" || s == "User:" ||
		strings.HasPrefix(s, "user: ") || strings.HasPrefix(s, "User: ")
}

func isAssistantMarker(s string) bool {
	return s == "assistant:" || s == "A:" ||
		strings.HasPrefix(s, "assistant: ") || strings.HasPrefix(s, "A: ")
}

func isToolCallMarker(s string) bool {
	return strings.HasPrefix(s, "[Tool call")
}

func isToolResultMarker(s string) bool {
	return strings.HasPrefix(s, "[Tool result")
}

// extractAfterMarker returns the text after the marker on the same line, if any.
func extractAfterMarker(line, marker string) string {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return ""
	}
	after := line[idx+len(marker):]
	return strings.TrimSpace(after)
}

// cleanCursorMarkup strips Cursor-specific XML-like tags from message content.
func cleanCursorMarkup(s string) string {
	s = strings.ReplaceAll(s, "<user_query>", "")
	s = strings.ReplaceAll(s, "</user_query>", "")
	s = strings.ReplaceAll(s, "[Thinking]", "")
	return strings.TrimSpace(s)
}

// extractToolCallName parses "[Tool call: Bash]" → "Bash".
func extractToolCallName(s string) string {
	// Expected: "[Tool call: <name>]" or "[Tool call:<name>]"
	s = strings.TrimPrefix(s, "[Tool call")
	s = strings.TrimPrefix(s, ":")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "]")
	return strings.TrimSpace(s)
}
