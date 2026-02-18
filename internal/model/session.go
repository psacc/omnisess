package model

import "time"

type Tool string

const (
	ToolClaude Tool = "claude"
	ToolCodex  Tool = "codex"
	ToolCursor Tool = "cursor"
	ToolGemini Tool = "gemini"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

type Session struct {
	ID        string    `json:"ID"`
	Tool      Tool      `json:"Tool"`
	Project   string    `json:"Project,omitempty"`
	Branch    string    `json:"Branch,omitempty"`
	Title     string    `json:"Title,omitempty"`
	Summary   string    `json:"Summary,omitempty"`
	Model     string    `json:"Model,omitempty"`
	StartedAt time.Time `json:"StartedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
	Active    bool      `json:"Active"`
	Messages  []Message `json:"Messages,omitempty"`
	Preview   string    `json:"Preview,omitempty"`
}

// QualifiedID returns the tool-prefixed session ID (e.g., "claude:5c3f2742").
func (s Session) QualifiedID() string {
	return string(s.Tool) + ":" + s.ID
}

// ShortID returns first 8 chars of the session ID.
func (s Session) ShortID() string {
	if len(s.ID) > 8 {
		return s.ID[:8]
	}
	return s.ID
}

// ShortProject returns the last two path components (e.g., "finn/b2b-orders-api").
func (s Session) ShortProject() string {
	parts := splitPath(s.Project)
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return s.Project
}

func splitPath(p string) []string {
	var parts []string
	var current string
	for _, c := range p {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

type Message struct {
	Role      Role
	Content   string
	Timestamp time.Time
	ToolCalls []ToolCall
}

type ToolCall struct {
	Name   string
	Input  string // truncated
	Output string // truncated
}

type SearchResult struct {
	Session Session
	Matches []SearchMatch
}

type SearchMatch struct {
	MessageIndex int
	Snippet      string // ~200 char context around match
	Role         Role
}
