package index

import "time"

// Session is the source-agnostic payload an Index writer needs to populate
// session_metadata + tool_calls rows. Callers convert their source-specific
// model.Session into this shape before calling EnsureSession.
//
// The split matters for two reasons:
//  1. index never imports any source package (parallels the cmd → source flow)
//  2. it makes the writer trivially testable with synthetic inputs
type Session struct {
	ConversationID string
	ProviderName   string // 'anthropic' for Claude
	RequestModel   string
	ResponseModel  string
	StartedAt      time.Time
	UpdatedAt      time.Time

	// Token usage rolled up from all assistant messages.
	TotalInputTokens       int
	TotalOutputTokens      int
	TotalCacheCreateTokens int
	TotalCacheReadTokens   int

	ToolCalls []ToolCallRow
}

// ToolCallRow corresponds to one row in the tool_calls table.
type ToolCallRow struct {
	ToolCallID       string
	ToolName         string
	ToolType         string // 'function' | 'mcp' | 'skill' | 'agent'
	OperationName    string // default 'execute_tool'
	IsError          bool
	Timestamp        time.Time
	FilePath         string
	FileOp           string // 'read' | 'write' | 'edit' | ''
	FileLinesAdded   int
	FileLinesRemoved int
	FileContentSize  int
	ArgumentsJSON    string // populated only when EnsureSession is called with includeFullPayloads=true
	ResultJSON       string // populated only when EnsureSession is called with includeFullPayloads=true
}
