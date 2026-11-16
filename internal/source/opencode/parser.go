package opencode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/psacc/omnisess/internal/model"
)

// timestampedJSON is the `time` envelope shared by v2 message payloads.
type timestampedJSON struct {
	Time struct {
		Created int64 `json:"created"` // unix milliseconds
	} `json:"time"`
}

// v2UserMessage is the payload of a "user" row in session_message.
type v2UserMessage struct {
	Text string `json:"text"`
}

// v2ContentBlock is one element of an assistant message's content array.
// Only the fields omnisess renders are decoded; step/snapshot blocks are
// ignored by the type switch.
type v2ContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	State struct {
		Status  string           `json:"status"`
		Input   json.RawMessage  `json:"input"`
		Output  string           `json:"output"`
		Content []v2ContentBlock `json:"content"`
	} `json:"state"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
}

// v2AssistantMessage is the payload of an "assistant" row in session_message.
type v2AssistantMessage struct {
	Model struct {
		ID         string `json:"id"`
		ModelID    string `json:"modelID"`
		ProviderID string `json:"providerID"`
	} `json:"model"`
	Content []v2ContentBlock `json:"content"`
}

// readV2Messages returns the messages of a v2 session ordered by sequence.
func readV2Messages(db *sql.DB, sessionID string) ([]model.Message, error) {
	rows, err := db.Query(
		`SELECT type, data FROM session_message WHERE session_id=? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var (
			typ  string
			data string
		)
		if err := rows.Scan(&typ, &data); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		if msg := parseV2Message(typ, data); msg != nil {
			messages = append(messages, *msg)
		}
	}
	return messages, rows.Err()
}

// parseV2Message converts one session_message row to a model.Message.
// Types omnisess does not render (system, shell, synthetic, compaction, idle)
// and malformed payloads return nil.
func parseV2Message(typ, data string) *model.Message {
	switch typ {
	case "user":
		var payload struct {
			timestampedJSON
			v2UserMessage
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return nil
		}
		if payload.Text == "" {
			return nil
		}
		return &model.Message{
			Role:      model.RoleUser,
			Content:   payload.Text,
			Timestamp: unixMilli(payload.Time.Created),
		}

	case "assistant":
		var payload struct {
			timestampedJSON
			v2AssistantMessage
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return nil
		}
		msg := model.Message{
			Role:      model.RoleAssistant,
			Timestamp: unixMilli(payload.Time.Created),
		}
		var texts []string
		for _, block := range payload.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					texts = append(texts, block.Text)
				}
			case "tool":
				msg.ToolCalls = append(msg.ToolCalls, toolCallFromV2Block(block))
			}
		}
		msg.Content = strings.Join(texts, "\n")
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			return nil
		}
		return &msg
	}
	return nil
}

// toolCallFromV2Block maps an inlined tool part to model.ToolCall.
// Completed tools carry their result either in state.output or, for tools
// that return structured blocks, in state.content.
func toolCallFromV2Block(block v2ContentBlock) model.ToolCall {
	call := model.ToolCall{
		ID:    block.ID,
		Name:  block.Name,
		Input: strings.TrimSpace(string(block.State.Input)),
	}
	call.Output = toolOutput(block)
	call.IsError = block.State.Status == "error"
	return call
}

// toolOutput normalises the two shapes OpenCode uses for tool results.
func toolOutput(block v2ContentBlock) string {
	if block.State.Output != "" {
		return block.State.Output
	}
	var parts []string
	for _, c := range block.State.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// legacyMessage is the payload of a row in the legacy "message" table.
type legacyMessage struct {
	Role string `json:"role"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
}

// legacyPart is the payload of a row in the legacy "part" table.
type legacyPart struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Tool   string `json:"tool"`
	CallID string `json:"callID"`
	State  struct {
		Status  string           `json:"status"`
		Input   json.RawMessage  `json:"input"`
		Output  string           `json:"output"`
		Content []v2ContentBlock `json:"content"`
	} `json:"state"`
}

// messageRow is one row of the legacy "message" table.
type messageRow struct {
	id   string
	data string
}

// scanMessageRows drains the message query, preserving row order.
func scanMessageRows(rows *sql.Rows) ([]messageRow, error) {
	var out []messageRow
	for rows.Next() {
		var r messageRow
		if err := rows.Scan(&r.id, &r.data); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// readLegacyMessages returns the messages of a pre-v2 session. Message bodies
// live in the "part" table, one or more rows per message.
func readLegacyMessages(db *sql.DB, sessionID string) ([]model.Message, error) {
	rows, err := db.Query(
		`SELECT id, data FROM message WHERE session_id=? ORDER BY time_created, id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	messageRows, err := scanMessageRows(rows)
	if err != nil {
		return nil, err
	}

	var messages []model.Message
	for _, r := range messageRows {
		parts, err := readLegacyParts(db, r.id)
		if err != nil {
			return nil, err
		}
		if msg := parseLegacyMessage(r.data, parts); msg != nil {
			messages = append(messages, *msg)
		}
	}
	return messages, nil
}

// readLegacyParts returns the parts of one legacy message in write order.
func readLegacyParts(db *sql.DB, messageID string) ([]legacyPart, error) {
	rows, err := db.Query(
		`SELECT data FROM part WHERE message_id=? ORDER BY time_created, id`, messageID)
	if err != nil {
		return nil, fmt.Errorf("query parts: %w", err)
	}
	defer rows.Close()

	var parts []legacyPart
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan part: %w", err)
		}
		var part legacyPart
		if err := json.Unmarshal([]byte(data), &part); err != nil {
			continue // skip malformed parts, keep the rest of the message
		}
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

// parseLegacyMessage converts a legacy message row plus its parts to a
// model.Message. Unknown roles return nil.
func parseLegacyMessage(data string, parts []legacyPart) *model.Message {
	var payload legacyMessage
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil
	}
	var role model.Role
	switch payload.Role {
	case "user":
		role = model.RoleUser
	case "assistant":
		role = model.RoleAssistant
	default:
		return nil
	}

	msg := model.Message{
		Role:      role,
		Timestamp: unixMilli(payload.Time.Created),
	}
	var texts []string
	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		case "tool":
			msg.ToolCalls = append(msg.ToolCalls, model.ToolCall{
				ID:      part.CallID,
				Name:    part.Tool,
				Input:   strings.TrimSpace(string(part.State.Input)),
				Output:  legacyToolOutput(part),
				IsError: part.State.Status == "error",
			})
		}
	}
	msg.Content = strings.Join(texts, "\n")
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil
	}
	return &msg
}

// legacyToolOutput mirrors toolOutput for the legacy part shape. `state.content`
// is not part of the legacy schema, but some legacy rows carry it, so the
// shared block decoder is reused.
func legacyToolOutput(part legacyPart) string {
	if part.State.Output != "" {
		return part.State.Output
	}
	var texts []string
	for _, c := range part.State.Content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n")
}
