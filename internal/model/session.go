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
	ID        string
	Tool      Tool
	Project   string // absolute path to project directory
	Branch    string // git branch if available
	Title     string // first user message or tool-provided title
	Summary   string // tool-provided summary (Cursor tldr), empty otherwise
	Model     string // AI model used
	StartedAt time.Time
	UpdatedAt time.Time
	Active    bool
	Messages  []Message // populated only by Get(), nil in List()
	Preview   string    // first user message, truncated ~120 chars
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
