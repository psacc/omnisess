package model

import "time"

type Tool string

const (
	ToolClaude  Tool = "claude"
	ToolCodex   Tool = "codex"
	ToolCopilot Tool = "copilot"
	ToolCursor  Tool = "cursor"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

type Session struct {
	ID          string    `json:"ID"`
	Tool        Tool      `json:"Tool"`
	Project     string    `json:"Project,omitempty"`
	Branch      string    `json:"Branch,omitempty"`
	Title       string    `json:"Title,omitempty"`
	Summary     string    `json:"Summary,omitempty"`
	Model       string    `json:"Model,omitempty"`
	StartedAt   time.Time `json:"StartedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
	Active      bool      `json:"Active"`
	Status      string    `json:"Status,omitempty"`      // live-process status (claude registry: busy/idle/waiting/shell/...); set only when Active
	Name        string    `json:"Name,omitempty"`        // configured session name (claude /rename); set only when Active
	Entrypoint  string    `json:"Entrypoint,omitempty"`  // claude: "cli" | "claude-desktop"; set only when Active
	Kind        string    `json:"Kind,omitempty"`        // claude registry kind: "interactive" | "bg"; set only when Active
	TmuxSession string    `json:"TmuxSession,omitempty"` // owning tmux session name; set only when Active

	Messages []Message `json:"Messages,omitempty"`
	Preview  string    `json:"Preview,omitempty"`
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

// ShortProject returns the last two path components (e.g., "team/my-app").
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

	// Usage* fields are populated from assistant-message `usage` blocks
	// in the source JSONL. They are 0 for user/tool/system messages and
	// for sources that do not report token usage.
	UsageInputTokens              int
	UsageOutputTokens             int
	UsageCacheCreationInputTokens int
	UsageCacheReadInputTokens     int
}

type ToolCall struct {
	Name   string
	Input  string // raw JSON of tool_use.input (not truncated)
	Output string // raw JSON of tool_result.content or toolUseResult (not truncated)

	// ID is the stable join key between a tool_use block and its matching
	// tool_result (Claude's tool_use_id, e.g. "toolu_X"). Empty when the
	// source does not provide one.
	ID string

	// IsError is true when the matching tool_result indicated failure.
	// See internal/source/claude/parser.go is_error rules.
	IsError bool

	// File-I/O fields are populated for Read/Write/Edit calls only.
	// FilePath is empty for tools without an obvious file path (e.g. Bash).
	FilePath         string
	FileOp           string // "read" | "write" | "edit" | ""
	FileLinesAdded   int
	FileLinesRemoved int
	FileContentSize  int
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
