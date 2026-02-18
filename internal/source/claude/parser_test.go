package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/psacconier/sessions/internal/model"
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
	// Input longer than 200 chars should be truncated
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
	if len(calls[0].Input) > 210 { // 200 + "..."
		t.Errorf("expected truncated input, got length %d", len(calls[0].Input))
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
