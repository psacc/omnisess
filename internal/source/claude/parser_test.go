package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

func TestParseHistoryLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantID   string
		wantProj string
		wantDisp string
		wantTS   int64
		wantErr  bool
	}{
		{
			name:     "valid entry",
			line:     `{"display":"fix the bug","timestamp":1708000000000,"project":"/Users/foo/myproject","sessionId":"abc12345-1234-5678-9abc-def012345678"}`,
			wantID:   "abc12345-1234-5678-9abc-def012345678",
			wantProj: "/Users/foo/myproject",
			wantDisp: "fix the bug",
			wantTS:   1708000000000,
		},
		{
			name:    "malformed JSON",
			line:    `{invalid json`,
			wantErr: true,
		},
		{
			name:     "empty session ID is still valid parse",
			line:     `{"display":"test","timestamp":1708000000000,"project":"/tmp","sessionId":""}`,
			wantID:   "",
			wantProj: "/tmp",
			wantDisp: "test",
			wantTS:   1708000000000,
		},
		{
			name:     "missing optional fields",
			line:     `{"sessionId":"abc123"}`,
			wantID:   "abc123",
			wantProj: "",
			wantDisp: "",
			wantTS:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseHistoryLine([]byte(tt.line))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.SessionID != tt.wantID {
				t.Errorf("SessionID = %q, want %q", entry.SessionID, tt.wantID)
			}
			if entry.Project != tt.wantProj {
				t.Errorf("Project = %q, want %q", entry.Project, tt.wantProj)
			}
			if entry.Display != tt.wantDisp {
				t.Errorf("Display = %q, want %q", entry.Display, tt.wantDisp)
			}
			if entry.Timestamp != tt.wantTS {
				t.Errorf("Timestamp = %d, want %d", entry.Timestamp, tt.wantTS)
			}
		})
	}
}

func TestParseSessionFile_Simple(t *testing.T) {
	path := filepath.Join("testdata", "session_simple.jsonl")
	messages, mdl, branch, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// First message: user
	if messages[0].Role != model.RoleUser {
		t.Errorf("messages[0].Role = %q, want %q", messages[0].Role, model.RoleUser)
	}
	if messages[0].Content != "hello, can you help me fix a bug?" {
		t.Errorf("messages[0].Content = %q", messages[0].Content)
	}
	if messages[0].Timestamp.IsZero() {
		t.Error("messages[0].Timestamp should not be zero")
	}

	// Second message: assistant
	if messages[1].Role != model.RoleAssistant {
		t.Errorf("messages[1].Role = %q, want %q", messages[1].Role, model.RoleAssistant)
	}
	if messages[1].Content != "Sure! I'd be happy to help. What bug are you seeing?" {
		t.Errorf("messages[1].Content = %q", messages[1].Content)
	}

	if mdl != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want claude-sonnet-4-20250514", mdl)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
}

func TestParseSessionFile_WithTools(t *testing.T) {
	path := filepath.Join("testdata", "session_with_tools.jsonl")
	messages, mdl, branch, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// First assistant message should have one tool call (Read)
	if len(messages[1].ToolCalls) != 1 {
		t.Fatalf("messages[1].ToolCalls = %d, want 1", len(messages[1].ToolCalls))
	}
	if messages[1].ToolCalls[0].Name != "Read" {
		t.Errorf("tool call name = %q, want Read", messages[1].ToolCalls[0].Name)
	}

	// Second assistant message should have one tool call (Edit)
	if len(messages[2].ToolCalls) != 1 {
		t.Fatalf("messages[2].ToolCalls = %d, want 1", len(messages[2].ToolCalls))
	}
	if messages[2].ToolCalls[0].Name != "Edit" {
		t.Errorf("tool call name = %q, want Edit", messages[2].ToolCalls[0].Name)
	}

	if mdl != "claude-opus-4-20250514" {
		t.Errorf("model = %q, want claude-opus-4-20250514", mdl)
	}
	if branch != "feat/config" {
		t.Errorf("branch = %q, want feat/config", branch)
	}
}

func TestParseSessionFile_WithArrayContent(t *testing.T) {
	path := filepath.Join("testdata", "session_with_array_content.jsonl")
	messages, _, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}

	// Should have 2 messages (user + assistant). Summary line should be skipped.
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Assistant message should have concatenated text from both text blocks
	want := "The architecture has three layers:\n1. API layer\n2. Service layer\n3. Data layer"
	if messages[1].Content != want {
		t.Errorf("messages[1].Content = %q, want %q", messages[1].Content, want)
	}
}

func TestParseSessionFile_Nonexistent(t *testing.T) {
	_, _, _, err := parseSessionFile("testdata/nonexistent.jsonl")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestParseSessionFile_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	content := "\n" +
		`{"type":"user","message":{"role":"user","content":"test"},"timestamp":"2024-02-15T10:00:00Z","cwd":"/tmp"}` + "\n" +
		"\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response"}]},"timestamp":"2024-02-15T10:00:01Z","model":"test-model"}` + "\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, mdl, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if mdl != "test-model" {
		t.Errorf("model = %q, want test-model", mdl)
	}
}

func TestExtractContent(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		want    string
	}{
		{
			name:    "nil content",
			content: nil,
			want:    "",
		},
		{
			name:    "string content",
			content: "hello world",
			want:    "hello world",
		},
		{
			name:    "empty string",
			content: "",
			want:    "",
		},
		{
			name: "array of text blocks",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "first"},
				map[string]interface{}{"type": "text", "text": "second"},
			},
			want: "first\nsecond",
		},
		{
			name: "array with tool_use blocks filtered",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "some text"},
				map[string]interface{}{"type": "tool_use", "name": "Read"},
			},
			want: "some text",
		},
		{
			name:    "empty array",
			content: []interface{}{},
			want:    "",
		},
		{
			name:    "unexpected type returns empty",
			content: 42,
			want:    "",
		},
		{
			name: "array with non-map elements skipped",
			content: []interface{}{
				"not a map",
				map[string]interface{}{"type": "text", "text": "valid"},
			},
			want: "valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContent(tt.content)
			if got != tt.want {
				t.Errorf("extractContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractToolCalls(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		want    int // number of tool calls
	}{
		{
			name:    "nil content",
			content: nil,
			want:    0,
		},
		{
			name:    "string content no tools",
			content: "hello",
			want:    0,
		},
		{
			name: "with tool_use blocks",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "some text"},
				map[string]interface{}{"type": "tool_use", "name": "Read", "input": map[string]interface{}{"path": "/foo"}},
				map[string]interface{}{"type": "tool_use", "name": "Edit", "input": map[string]interface{}{"path": "/bar"}},
			},
			want: 2,
		},
		{
			name: "no tool_use blocks",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "just text"},
			},
			want: 0,
		},
		{
			name:    "empty array",
			content: []interface{}{},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolCalls(tt.content)
			if len(got) != tt.want {
				t.Errorf("extractToolCalls() returned %d calls, want %d", len(got), tt.want)
			}
		})
	}
}

func TestExtractToolCalls_Names(t *testing.T) {
	content := []interface{}{
		map[string]interface{}{"type": "tool_use", "name": "Read", "input": map[string]interface{}{"path": "/foo"}},
		map[string]interface{}{"type": "tool_use", "name": "Edit", "input": map[string]interface{}{"path": "/bar"}},
	}

	calls := extractToolCalls(content)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "Read" {
		t.Errorf("calls[0].Name = %q, want Read", calls[0].Name)
	}
	if calls[1].Name != "Edit" {
		t.Errorf("calls[1].Name = %q, want Edit", calls[1].Name)
	}
}

func TestExtractToolCalls_LargeInput(t *testing.T) {
	// Inputs are NOT truncated — the parser stores the full JSON because
	// file paths and Write/Edit payloads can exceed the historic 200-byte cap.
	largeInput := make(map[string]interface{})
	longStr := ""
	for i := 0; i < 300; i++ {
		longStr += "x"
	}
	largeInput["data"] = longStr

	content := []interface{}{
		map[string]interface{}{"type": "tool_use", "name": "Write", "input": largeInput},
	}

	calls := extractToolCalls(content)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if len(calls[0].Input) < 300 {
		t.Errorf("expected full untruncated input (>=300 bytes), got length %d", len(calls[0].Input))
	}
	if strings.Contains(calls[0].Input, "...") {
		t.Errorf("expected no truncation marker in untruncated input, got: %q", calls[0].Input)
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		isZero bool
		year   int
	}{
		{
			name:   "RFC3339",
			input:  "2024-02-15T10:00:00Z",
			isZero: false,
			year:   2024,
		},
		{
			name:   "RFC3339Nano",
			input:  "2024-02-15T10:00:00.123456789Z",
			isZero: false,
			year:   2024,
		},
		{
			name:   "RFC3339 with millis",
			input:  "2024-02-15T10:00:00.000Z",
			isZero: false,
			year:   2024,
		},
		{
			name:   "Claude Code timestamp format",
			input:  "2026-02-18T11:31:44.598Z",
			isZero: false,
			year:   2026,
		},
		{
			name:   "empty string",
			input:  "",
			isZero: true,
		},
		{
			name:   "garbage string",
			input:  "not-a-timestamp",
			isZero: true,
		},
		{
			name:   "unix timestamp string (not supported)",
			input:  "1708000000",
			isZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestamp(tt.input)
			if tt.isZero {
				if !got.IsZero() {
					t.Errorf("parseTimestamp(%q) = %v, want zero", tt.input, got)
				}
				return
			}
			if got.IsZero() {
				t.Fatalf("parseTimestamp(%q) returned zero, want non-zero", tt.input)
			}
			if got.Year() != tt.year {
				t.Errorf("parseTimestamp(%q).Year() = %d, want %d", tt.input, got.Year(), tt.year)
			}
		})
	}
}

func TestParseTimestamp_UTC(t *testing.T) {
	ts := parseTimestamp("2024-02-15T10:00:00Z")
	if ts.Location() != time.UTC {
		t.Errorf("expected UTC location, got %v", ts.Location())
	}
}

func TestProjectDirName(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "absolute path",
			path: "/Users/foo/bar",
			want: "-Users-foo-bar",
		},
		{
			name: "root",
			path: "/",
			want: "-",
		},
		{
			name: "empty",
			path: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectDirName(tt.path)
			if got != tt.want {
				t.Errorf("projectDirName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestProjectPathFromDir(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		want    string
	}{
		{
			name:    "standard dir name",
			dirName: "-Users-foo-bar",
			want:    "/Users/foo/bar",
		},
		{
			name:    "root",
			dirName: "-",
			want:    "/",
		},
		{
			name:    "empty",
			dirName: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectPathFromDir(tt.dirName)
			if got != tt.want {
				t.Errorf("projectPathFromDir(%q) = %q, want %q", tt.dirName, got, tt.want)
			}
		})
	}
}

func TestProjectDirName_RoundTrip(t *testing.T) {
	paths := []string{
		"/Users/foo/myproject",
		"/Users/paolo/prj/sessions",
		"/tmp/test",
	}

	for _, path := range paths {
		dirName := projectDirName(path)
		back := projectPathFromDir(dirName)
		if back != path {
			t.Errorf("roundtrip failed: %q -> %q -> %q", path, dirName, back)
		}
	}
}

func TestExtractSnippet(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		matchIdx  int
		matchLen  int
		targetLen int
		wantExact string // if set, expect exact match
		wantHas   string // if set, snippet must contain this
	}{
		{
			name:      "content shorter than target",
			content:   "short content",
			matchIdx:  0,
			matchLen:  5,
			targetLen: 200,
			wantExact: "short content",
		},
		{
			name:      "match at start of long content",
			content:   "hello world this is a very long string that exceeds the target length significantly and keeps going on",
			matchIdx:  0,
			matchLen:  5,
			targetLen: 30,
			wantHas:   "hello",
		},
		{
			name:      "match in middle",
			content:   "aaaaaaaaaa MATCH bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			matchIdx:  11,
			matchLen:  5,
			targetLen: 20,
			wantHas:   "MATCH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSnippet(tt.content, tt.matchIdx, tt.matchLen, tt.targetLen)
			if tt.wantExact != "" {
				if got != tt.wantExact {
					t.Errorf("extractSnippet() = %q, want %q", got, tt.wantExact)
				}
				return
			}
			if tt.wantHas != "" {
				found := false
				for i := 0; i <= len(got)-len(tt.wantHas); i++ {
					if got[i:i+len(tt.wantHas)] == tt.wantHas {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extractSnippet() = %q, expected to contain %q", got, tt.wantHas)
				}
			}
		})
	}
}

func TestExtractSnippet_EllipsisMarkers(t *testing.T) {
	// Long content, match in the middle
	content := "aaaaaaaaaaaaaaaaaaaaa MATCH bbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	got := extractSnippet(content, 22, 5, 20)

	// Should have leading and trailing ellipsis
	if got[:3] != "..." {
		t.Errorf("expected leading '...', got prefix %q", got[:3])
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("expected trailing '...', got suffix %q", got[len(got)-3:])
	}
}

func TestExtractSessionIDFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "standard path",
			path: "/Users/foo/.claude/projects/-Users-foo-bar/abc12345.jsonl",
			want: "abc12345",
		},
		{
			name: "uuid path",
			path: "/some/path/abc12345-1234-5678-9abc-def012345678.jsonl",
			want: "abc12345-1234-5678-9abc-def012345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionIDFromPath(tt.path)
			if got != tt.want {
				t.Errorf("extractSessionIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestParseSessionFile_ToolUseResult exercises tool_use/tool_result pairing,
// toolUseResult (object and bare-string) handling, structuredPatch line
// counting, file-I/O extraction, error flagging, orphan tool_use, and
// assistant `usage` token capture. Covers the fixture matrix in tasks.md §4.7
// at the parser layer.
func TestParseSessionFile_ToolUseResult(t *testing.T) {
	path := filepath.Join("testdata", "tool_use_result.jsonl")
	messages, mdl, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}
	// Fixture exercises both inner-`message.model` (line 1) and top-level
	// `model` (later lines). First non-empty wins; inner model is captured.
	if mdl != "claude-opus-4-7" {
		t.Errorf("model = %q, want claude-opus-4-7", mdl)
	}

	// Collect all tool calls keyed by ID for easy assertions.
	byID := make(map[string]model.ToolCall)
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			byID[tc.ID] = tc
		}
	}

	// Read pair: file_path captured from input AND nested toolUseResult.file.filePath
	if tc, ok := byID["toolu_R1"]; !ok {
		t.Fatal("missing toolu_R1")
	} else {
		if tc.Name != "Read" || tc.FileOp != "read" {
			t.Errorf("R1 name/op = %q/%q", tc.Name, tc.FileOp)
		}
		if tc.FilePath != "/Users/x/proj/a.go" {
			t.Errorf("R1 FilePath = %q", tc.FilePath)
		}
		if tc.IsError {
			t.Errorf("R1 IsError = true, want false")
		}
		if tc.Output == "" {
			t.Errorf("R1 Output empty; expected toolUseResult json")
		}
	}

	// Edit with structuredPatch: 2 added, 1 removed
	if tc, ok := byID["toolu_E1"]; !ok {
		t.Fatal("missing toolu_E1")
	} else {
		if tc.FileOp != "edit" || tc.FilePath != "/Users/x/proj/a.go" {
			t.Errorf("E1 op/path = %q/%q", tc.FileOp, tc.FilePath)
		}
		if tc.FileLinesAdded != 2 {
			t.Errorf("E1 FileLinesAdded = %d, want 2", tc.FileLinesAdded)
		}
		if tc.FileLinesRemoved != 1 {
			t.Errorf("E1 FileLinesRemoved = %d, want 1", tc.FileLinesRemoved)
		}
	}

	// Bash: no file extraction in PR1
	if tc, ok := byID["toolu_B1"]; !ok {
		t.Fatal("missing toolu_B1")
	} else if tc.Name != "Bash" || tc.FilePath != "" || tc.FileOp != "" {
		t.Errorf("B1 name/path/op = %q/%q/%q (want Bash/empty/empty)", tc.Name, tc.FilePath, tc.FileOp)
	}

	// Read with is_error=true on tool_result
	if tc, ok := byID["toolu_R2"]; !ok {
		t.Fatal("missing toolu_R2")
	} else if !tc.IsError {
		t.Errorf("R2 IsError = false, want true (tool_result.is_error)")
	}

	// Skill with success: false
	if tc, ok := byID["toolu_S1"]; !ok {
		t.Fatal("missing toolu_S1")
	} else if !tc.IsError {
		t.Errorf("S1 IsError = false, want true (toolUseResult.success=false)")
	}

	// Bash with bare-string "Error:" toolUseResult
	if tc, ok := byID["toolu_B2"]; !ok {
		t.Fatal("missing toolu_B2")
	} else if !tc.IsError {
		t.Errorf("B2 IsError = false, want true (toolUseResult bare-string Error:)")
	}

	// Write with content
	if tc, ok := byID["toolu_W1"]; !ok {
		t.Fatal("missing toolu_W1")
	} else {
		if tc.FileOp != "write" || tc.FilePath != "/Users/x/proj/b.go" {
			t.Errorf("W1 op/path = %q/%q", tc.FileOp, tc.FilePath)
		}
		if tc.FileContentSize != len("hello world") {
			t.Errorf("W1 FileContentSize = %d, want %d", tc.FileContentSize, len("hello world"))
		}
	}

	// Orphan tool_use (no tool_result) should still exist
	if tc, ok := byID["toolu_ORPHAN"]; !ok {
		t.Fatal("missing toolu_ORPHAN (orphan tool_use)")
	} else {
		if tc.IsError {
			t.Errorf("orphan IsError = true, want false")
		}
		if tc.Output != "" {
			t.Errorf("orphan Output = %q, want empty", tc.Output)
		}
	}

	// Usage tokens summed across assistant messages.
	totalIn := 0
	totalOut := 0
	totalCacheCreate := 0
	totalCacheRead := 0
	for _, m := range messages {
		if m.Role == model.RoleAssistant {
			totalIn += m.UsageInputTokens
			totalOut += m.UsageOutputTokens
			totalCacheCreate += m.UsageCacheCreationInputTokens
			totalCacheRead += m.UsageCacheReadInputTokens
		}
	}
	// Fixture has 3 assistant messages with usage: 100+50+10=160; 40+20+5=65
	if totalIn < 160 {
		t.Errorf("totalInputTokens = %d, want >= 160", totalIn)
	}
	if totalOut < 65 {
		t.Errorf("totalOutputTokens = %d, want >= 65", totalOut)
	}
	if totalCacheCreate != 1000 {
		t.Errorf("totalCacheCreate = %d, want 1000", totalCacheCreate)
	}
	if totalCacheRead != 500 {
		t.Errorf("totalCacheRead = %d, want 500", totalCacheRead)
	}
}

// TestExtractToolResults covers the user-content tool_result extraction helper.
func TestExtractToolResults(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		wantIDs []string
	}{
		{
			name:    "nil content",
			content: nil,
			wantIDs: nil,
		},
		{
			name:    "string content",
			content: "ignore me",
			wantIDs: nil,
		},
		{
			name: "single tool_result",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_X", "content": "hi"},
			},
			wantIDs: []string{"toolu_X"},
		},
		{
			name: "skip non-tool_result blocks",
			content: []interface{}{
				map[string]interface{}{"type": "text", "text": "hi"},
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_X", "content": "hi"},
			},
			wantIDs: []string{"toolu_X"},
		},
		{
			name: "skip empty tool_use_id",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "", "content": "hi"},
			},
			wantIDs: nil,
		},
		{
			name: "skip non-map blocks",
			content: []interface{}{
				"a string block",
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_Y", "content": "ok"},
			},
			wantIDs: []string{"toolu_Y"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolResults(tt.content)
			if len(got) != len(tt.wantIDs) {
				t.Errorf("len(got)=%d want %d (%v)", len(got), len(tt.wantIDs), got)
			}
			for _, id := range tt.wantIDs {
				if _, ok := got[id]; !ok {
					t.Errorf("missing id %q", id)
				}
			}
		})
	}
}

// TestExtractToolResultErrors covers is_error extraction including all skip paths.
func TestExtractToolResultErrors(t *testing.T) {
	tests := []struct {
		name    string
		content interface{}
		want    map[string]bool
	}{
		{name: "nil", content: nil, want: map[string]bool{}},
		{name: "string", content: "x", want: map[string]bool{}},
		{
			name: "is_error true",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_X", "is_error": true},
			},
			want: map[string]bool{"toolu_X": true},
		},
		{
			name: "is_error false",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_X", "is_error": false},
			},
			want: map[string]bool{},
		},
		{
			name: "non-bool is_error",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_X", "is_error": "no"},
			},
			want: map[string]bool{},
		},
		{
			name: "non-tool_result skipped",
			content: []interface{}{
				map[string]interface{}{"type": "text"},
			},
			want: map[string]bool{},
		},
		{
			name: "empty tool_use_id skipped",
			content: []interface{}{
				map[string]interface{}{"type": "tool_result", "tool_use_id": "", "is_error": true},
			},
			want: map[string]bool{},
		},
		{
			name: "non-map block skipped",
			content: []interface{}{
				42,
				map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_Y", "is_error": true},
			},
			want: map[string]bool{"toolu_Y": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolResultErrors(tt.content)
			if len(got) != len(tt.want) {
				t.Errorf("len=%d want %d (got: %v)", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("got[%q]=%v want %v", k, got[k], v)
				}
			}
		})
	}
}

// TestApplyToolUseResult covers the per-shape branches of applyToolUseResult.
func TestApplyToolUseResult(t *testing.T) {
	t.Run("empty raw", func(t *testing.T) {
		var tc model.ToolCall
		applyToolUseResult(&tc, nil)
		if tc.Output != "" || tc.IsError {
			t.Errorf("expected no-op for empty raw, got %+v", tc)
		}
	})
	t.Run("bare string non-error", func(t *testing.T) {
		var tc model.ToolCall
		applyToolUseResult(&tc, []byte(`"hello"`))
		if tc.IsError {
			t.Errorf("non-error string should not set IsError")
		}
	})
	t.Run("bare string Error:", func(t *testing.T) {
		var tc model.ToolCall
		applyToolUseResult(&tc, []byte(`"Error: Exit code 1"`))
		if !tc.IsError {
			t.Errorf("Error: prefix should set IsError")
		}
	})
	t.Run("non-json garbage", func(t *testing.T) {
		var tc model.ToolCall
		applyToolUseResult(&tc, []byte(`{not json}`))
		// Output is still set; no panic.
		if tc.Output != "{not json}" {
			t.Errorf("Output = %q", tc.Output)
		}
	})
	t.Run("success:false", func(t *testing.T) {
		tc := model.ToolCall{Name: "Skill"}
		applyToolUseResult(&tc, []byte(`{"success":false,"commandName":"x"}`))
		if !tc.IsError {
			t.Errorf("success:false should set IsError")
		}
	})
	t.Run("Read with nested file.filePath", func(t *testing.T) {
		tc := model.ToolCall{Name: "Read"}
		applyToolUseResult(&tc, []byte(`{"type":"text","file":{"filePath":"/a/b.go"}}`))
		if tc.FilePath != "/a/b.go" {
			t.Errorf("FilePath = %q", tc.FilePath)
		}
	})
	t.Run("Read with non-map file ignored", func(t *testing.T) {
		tc := model.ToolCall{Name: "Read", FilePath: "/from-input.go"}
		applyToolUseResult(&tc, []byte(`{"file":"not a map"}`))
		if tc.FilePath != "/from-input.go" {
			t.Errorf("FilePath should stay from input, got %q", tc.FilePath)
		}
	})
	t.Run("Write top-level filePath wins", func(t *testing.T) {
		tc := model.ToolCall{Name: "Write", FilePath: "/from-input.go"}
		applyToolUseResult(&tc, []byte(`{"filePath":"/from-result.go"}`))
		if tc.FilePath != "/from-result.go" {
			t.Errorf("FilePath = %q, want /from-result.go", tc.FilePath)
		}
	})
	t.Run("Edit structuredPatch with context ignored", func(t *testing.T) {
		tc := model.ToolCall{Name: "Edit"}
		applyToolUseResult(&tc, []byte(`{"structuredPatch":[{"lines":[" ctx","+a","+b","-c"]}]}`))
		if tc.FileLinesAdded != 2 || tc.FileLinesRemoved != 1 {
			t.Errorf("added=%d removed=%d", tc.FileLinesAdded, tc.FileLinesRemoved)
		}
	})
	t.Run("Edit structuredPatch non-array ignored", func(t *testing.T) {
		tc := model.ToolCall{Name: "Edit"}
		applyToolUseResult(&tc, []byte(`{"structuredPatch":"oops"}`))
		if tc.FileLinesAdded != 0 || tc.FileLinesRemoved != 0 {
			t.Errorf("expected zero counts, got +%d -%d", tc.FileLinesAdded, tc.FileLinesRemoved)
		}
	})
	t.Run("non-Read/Write/Edit name ignored for file fields", func(t *testing.T) {
		tc := model.ToolCall{Name: "Grep"}
		applyToolUseResult(&tc, []byte(`{"filePath":"/x","file":{"filePath":"/y"}}`))
		if tc.FilePath != "" {
			t.Errorf("Grep should not get FilePath, got %q", tc.FilePath)
		}
	})
}

// TestCountStructuredPatchLines covers helper edges directly so coverage is 100%.
func TestCountStructuredPatchLines(t *testing.T) {
	tests := []struct {
		name             string
		patches          []interface{}
		wantAdd, wantRem int
	}{
		{name: "empty", patches: nil, wantAdd: 0, wantRem: 0},
		{
			name: "non-map hunk",
			patches: []interface{}{
				"not a hunk",
			},
		},
		{
			name: "missing lines key",
			patches: []interface{}{
				map[string]interface{}{},
			},
		},
		{
			name: "lines wrong type",
			patches: []interface{}{
				map[string]interface{}{"lines": "should be array"},
			},
		},
		{
			name: "non-string line entry",
			patches: []interface{}{
				map[string]interface{}{"lines": []interface{}{1, "+a"}},
			},
			wantAdd: 1,
		},
		{
			name: "empty string line entry",
			patches: []interface{}{
				map[string]interface{}{"lines": []interface{}{"", "+a", "-b"}},
			},
			wantAdd: 1,
			wantRem: 1,
		},
		{
			name: "context line ignored",
			patches: []interface{}{
				map[string]interface{}{"lines": []interface{}{" ctx", "+a", "+b", "-c"}},
			},
			wantAdd: 2,
			wantRem: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, r := countStructuredPatchLines(tt.patches)
			if a != tt.wantAdd || r != tt.wantRem {
				t.Errorf("got +%d -%d, want +%d -%d", a, r, tt.wantAdd, tt.wantRem)
			}
		})
	}
}

// TestApplyFileFromInput covers the helper directly to ensure all branches
// (non-map, Read/Write/Edit/other names, missing keys) are exercised.
func TestApplyFileFromInput(t *testing.T) {
	t.Run("non-map input", func(t *testing.T) {
		tc := model.ToolCall{Name: "Read"}
		applyFileFromInput(&tc, "not a map")
		if tc.FileOp != "" || tc.FilePath != "" {
			t.Errorf("expected no change, got %+v", tc)
		}
	})
	t.Run("Read missing file_path", func(t *testing.T) {
		tc := model.ToolCall{Name: "Read"}
		applyFileFromInput(&tc, map[string]interface{}{})
		if tc.FileOp != "read" {
			t.Errorf("op = %q, want read", tc.FileOp)
		}
		if tc.FilePath != "" {
			t.Errorf("FilePath = %q, want empty", tc.FilePath)
		}
	})
	t.Run("Write with content", func(t *testing.T) {
		tc := model.ToolCall{Name: "Write"}
		applyFileFromInput(&tc, map[string]interface{}{"file_path": "/a", "content": "hello"})
		if tc.FileOp != "write" || tc.FilePath != "/a" || tc.FileContentSize != 5 {
			t.Errorf("got %+v", tc)
		}
	})
	t.Run("Write without content", func(t *testing.T) {
		tc := model.ToolCall{Name: "Write"}
		applyFileFromInput(&tc, map[string]interface{}{"file_path": "/a"})
		if tc.FileContentSize != 0 {
			t.Errorf("FileContentSize = %d, want 0", tc.FileContentSize)
		}
	})
	t.Run("Edit", func(t *testing.T) {
		tc := model.ToolCall{Name: "Edit"}
		applyFileFromInput(&tc, map[string]interface{}{"file_path": "/a"})
		if tc.FileOp != "edit" || tc.FilePath != "/a" {
			t.Errorf("got %+v", tc)
		}
	})
	t.Run("Bash (no extraction)", func(t *testing.T) {
		tc := model.ToolCall{Name: "Bash"}
		applyFileFromInput(&tc, map[string]interface{}{"command": "ls"})
		if tc.FileOp != "" || tc.FilePath != "" {
			t.Errorf("Bash should not get file fields, got %+v", tc)
		}
	})
}

func TestProjectFromSessionPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "standard claude session path",
			path: "/Users/foo/.claude/projects/-Users-foo-bar/abc12345.jsonl",
			want: "/Users/foo/bar",
		},
		{
			name: "deep project path",
			path: "/home/.claude/projects/-home-user-prj-sessions/session.jsonl",
			want: "/home/user/prj/sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectFromSessionPath(tt.path)
			if got != tt.want {
				t.Errorf("projectFromSessionPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
