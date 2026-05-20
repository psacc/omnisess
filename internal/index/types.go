// Package index implements the derived SQLite cache over AI session
// transcripts. The schema is OpenTelemetry GenAI-aligned where applicable;
// file-I/O extensions live under the `omnisess.file.*` namespace.
//
// The package is intentionally source-agnostic: it depends on the unified
// `internal/model` types, never on any source package. Source parsers are
// the contract.
package index

import "time"

// SessionStats holds per-session statistics returned by QuerySession.
type SessionStats struct {
	ConversationID         string         `json:"conversation_id"`
	ProviderName           string         `json:"provider_name"`
	RequestModel           string         `json:"request_model,omitempty"`
	ResponseModel          string         `json:"response_model,omitempty"`
	StartedAt              time.Time      `json:"started_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	TotalInputTokens       int            `json:"total_input_tokens"`
	TotalOutputTokens      int            `json:"total_output_tokens"`
	TotalCacheCreateTokens int            `json:"total_cache_create_tokens"`
	TotalCacheReadTokens   int            `json:"total_cache_read_tokens"`
	ToolCallCount          int            `json:"tool_call_count"`
	ErrorCount             int            `json:"error_count"`
	HasFullPayloads        bool           `json:"has_full_payloads"`
	Tools                  []ToolCount    `json:"tools"`
	Files                  []FileActivity `json:"files"`
}

// ToolCount aggregates tool_calls rows by tool_name.
type ToolCount struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Count      int    `json:"count"`
	ErrorCount int    `json:"error_count"`
}

// FileActivity aggregates tool_calls rows by file_path.
type FileActivity struct {
	Path         string `json:"path"`
	Reads        int    `json:"reads"`
	Writes       int    `json:"writes"`
	Edits        int    `json:"edits"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
}

// AggregateStats holds cross-session statistics returned by QueryWindow.
type AggregateStats struct {
	WindowStart            time.Time      `json:"window_start"`
	WindowEnd              time.Time      `json:"window_end"`
	SessionCount           int            `json:"session_count"`
	TotalInputTokens       int            `json:"total_input_tokens"`
	TotalOutputTokens      int            `json:"total_output_tokens"`
	TotalCacheCreateTokens int            `json:"total_cache_create_tokens"`
	TotalCacheReadTokens   int            `json:"total_cache_read_tokens"`
	ToolCallCount          int            `json:"tool_call_count"`
	ErrorCount             int            `json:"error_count"`
	Tools                  []ToolCount    `json:"tools"`
	Files                  []FileActivity `json:"files"`
}
