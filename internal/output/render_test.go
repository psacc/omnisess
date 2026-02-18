package output

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/psacconier/sessions/internal/model"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "1234567890",
			maxLen: 10,
			want:   "1234567890",
		},
		{
			name:   "needs truncation",
			input:  "this is a longer string",
			maxLen: 15,
			want:   "this is a lo...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "minutes",
			duration: 30 * time.Minute,
			want:     "30m",
		},
		{
			name:     "zero minutes",
			duration: 0,
			want:     "0m",
		},
		{
			name:     "hours",
			duration: 5 * time.Hour,
			want:     "5h",
		},
		{
			name:     "one hour",
			duration: 1 * time.Hour,
			want:     "1h",
		},
		{
			name:     "23 hours",
			duration: 23 * time.Hour,
			want:     "23h",
		},
		{
			name:     "days",
			duration: 3 * 24 * time.Hour,
			want:     "3d",
		},
		{
			name:     "one day",
			duration: 24 * time.Hour,
			want:     "1d",
		},
		{
			name:     "six days",
			duration: 6 * 24 * time.Hour,
			want:     "6d",
		},
		{
			name:     "weeks",
			duration: 14 * 24 * time.Hour,
			want:     "2w",
		},
		{
			name:     "one week",
			duration: 7 * 24 * time.Hour,
			want:     "1w",
		},
		{
			name:     "sub-minute",
			duration: 30 * time.Second,
			want:     "0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestRenderTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderTable(&buf, nil)
	got := buf.String()
	if !strings.Contains(got, "No sessions found.") {
		t.Errorf("expected 'No sessions found.' in output, got: %q", got)
	}
}

func TestRenderTable_WithSessions(t *testing.T) {
	sessions := []model.Session{
		{
			ID:        "abc12345-1234-5678-9abc-def012345678",
			Tool:      model.ToolClaude,
			Project:   "/Users/foo/myproject",
			Branch:    "main",
			Preview:   "fix the login bug",
			StartedAt: time.Date(2024, 2, 15, 10, 0, 0, 0, time.UTC),
			Active:    true,
		},
		{
			ID:        "def67890-aaaa-bbbb-cccc-111122223333",
			Tool:      model.ToolCursor,
			Project:   "/Users/bar/otherproject",
			Branch:    "feat/dark-mode",
			Preview:   "add dark mode toggle",
			StartedAt: time.Date(2024, 2, 14, 9, 0, 0, 0, time.UTC),
			Active:    false,
		},
	}

	var buf bytes.Buffer
	renderTable(&buf, sessions)
	got := buf.String()

	// Check header
	if !strings.Contains(got, "TOOL") {
		t.Error("expected TOOL header in table output")
	}
	if !strings.Contains(got, "PROJECT") {
		t.Error("expected PROJECT header in table output")
	}

	// Check data rows
	if !strings.Contains(got, "claude") {
		t.Error("expected 'claude' in table output")
	}
	if !strings.Contains(got, "cursor") {
		t.Error("expected 'cursor' in table output")
	}
	if !strings.Contains(got, "ACTIVE") {
		t.Error("expected 'ACTIVE' status in table output")
	}
	if !strings.Contains(got, "fix the login bug") {
		t.Error("expected session preview in table output")
	}
}

func TestRenderJSON(t *testing.T) {
	sessions := []model.Session{
		{
			ID:   "abc12345",
			Tool: model.ToolClaude,
		},
	}

	var buf bytes.Buffer
	renderJSON(&buf, sessions)
	got := buf.String()

	// Should be valid JSON
	var parsed []model.Session
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("renderJSON output is not valid JSON: %v\noutput: %s", err, got)
	}
	if len(parsed) != 1 {
		t.Errorf("expected 1 session in JSON output, got %d", len(parsed))
	}
	if parsed[0].ID != "abc12345" {
		t.Errorf("expected ID abc12345, got %q", parsed[0].ID)
	}
}

func TestRenderSessionDetail(t *testing.T) {
	sess := &model.Session{
		ID:        "abc12345-1234-5678-9abc-def012345678",
		Tool:      model.ToolClaude,
		Project:   "/Users/foo/myproject",
		Branch:    "main",
		Model:     "claude-sonnet-4-20250514",
		StartedAt: time.Date(2024, 2, 15, 10, 0, 0, 0, time.UTC),
		Active:    true,
		Messages: []model.Message{
			{
				Role:      model.RoleUser,
				Content:   "hello",
				Timestamp: time.Date(2024, 2, 15, 10, 0, 0, 0, time.UTC),
			},
			{
				Role:      model.RoleAssistant,
				Content:   "hi there!",
				Timestamp: time.Date(2024, 2, 15, 10, 0, 5, 0, time.UTC),
				ToolCalls: []model.ToolCall{
					{Name: "Read"},
				},
			},
		},
	}

	var buf bytes.Buffer
	renderSessionDetail(&buf, sess)
	got := buf.String()

	// Check session metadata
	if !strings.Contains(got, "abc12345") {
		t.Error("expected short ID in detail output")
	}
	if !strings.Contains(got, "claude") {
		t.Error("expected tool name in detail output")
	}
	if !strings.Contains(got, "/Users/foo/myproject") {
		t.Error("expected project path in detail output")
	}
	if !strings.Contains(got, "main") {
		t.Error("expected branch in detail output")
	}
	if !strings.Contains(got, "claude-sonnet-4-20250514") {
		t.Error("expected model in detail output")
	}
	if !strings.Contains(got, "ACTIVE") {
		t.Error("expected ACTIVE status in detail output")
	}

	// Check messages
	if !strings.Contains(got, "user") {
		t.Error("expected user role in detail output")
	}
	if !strings.Contains(got, "assistant") {
		t.Error("expected assistant role in detail output")
	}
	if !strings.Contains(got, "hello") {
		t.Error("expected user message content in detail output")
	}
	if !strings.Contains(got, "hi there!") {
		t.Error("expected assistant message content in detail output")
	}
	if !strings.Contains(got, "[tool: Read]") {
		t.Error("expected tool call in detail output")
	}
}

func TestRenderSessionDetail_NoBranch(t *testing.T) {
	sess := &model.Session{
		ID:        "abc12345",
		Tool:      model.ToolClaude,
		Project:   "/Users/foo/myproject",
		StartedAt: time.Date(2024, 2, 15, 10, 0, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	renderSessionDetail(&buf, sess)
	got := buf.String()

	if strings.Contains(got, "Branch:") {
		t.Error("expected no Branch line when branch is empty")
	}
	if strings.Contains(got, "Model:") {
		t.Error("expected no Model line when model is empty")
	}
	if strings.Contains(got, "ACTIVE") {
		t.Error("expected no ACTIVE when not active")
	}
}

func TestRenderSearchTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderSearchTable(&buf, nil)
	got := buf.String()
	if !strings.Contains(got, "No matches found.") {
		t.Errorf("expected 'No matches found.' in output, got: %q", got)
	}
}

func TestRenderSearchTable_WithResults(t *testing.T) {
	results := []model.SearchResult{
		{
			Session: model.Session{
				ID:        "abc12345-1234-5678-9abc-def012345678",
				Tool:      model.ToolClaude,
				Project:   "/Users/foo/myproject",
				StartedAt: time.Date(2024, 2, 15, 10, 0, 0, 0, time.UTC),
			},
			Matches: []model.SearchMatch{
				{
					MessageIndex: 0,
					Snippet:      "found the bug in login",
					Role:         model.RoleUser,
				},
				{
					MessageIndex: 1,
					Snippet:      "fixing the bug now",
					Role:         model.RoleAssistant,
				},
			},
		},
	}

	var buf bytes.Buffer
	renderSearchTable(&buf, results)
	got := buf.String()

	if !strings.Contains(got, "claude:abc12345-1234-5678-9abc-def012345678") {
		t.Error("expected qualified ID in search output")
	}
	if !strings.Contains(got, "found the bug in login") {
		t.Error("expected snippet in search output")
	}
	if !strings.Contains(got, "[user]") {
		t.Error("expected role marker in search output")
	}
	if !strings.Contains(got, "[assistant]") {
		t.Error("expected assistant role marker in search output")
	}
}

// TestRenderSessions_Table exercises the public RenderSessions with table format.
// Writes to os.Stdout — no crash is the success criterion.
func TestRenderSessions_Table(t *testing.T) {
	// Redirect stdout to capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	RenderSessions(nil, FormatTable)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "No sessions found.") {
		t.Error("expected 'No sessions found.' from RenderSessions table format")
	}
}

func TestRenderSessions_JSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sessions := []model.Session{{ID: "test123", Tool: model.ToolClaude}}
	RenderSessions(sessions, FormatJSON)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "test123") {
		t.Error("expected session ID in JSON output")
	}
}

func TestRenderSession_Table(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sess := &model.Session{
		ID:        "abc12345",
		Tool:      model.ToolClaude,
		Project:   "/tmp/test",
		StartedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	RenderSession(sess, FormatTable)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "abc12345") {
		t.Error("expected session ID in table detail output")
	}
}

func TestRenderSession_JSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sess := &model.Session{ID: "json-test", Tool: model.ToolClaude}
	RenderSession(sess, FormatJSON)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "json-test") {
		t.Error("expected session ID in JSON detail output")
	}
}

func TestRenderSearchResults_Table(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	RenderSearchResults(nil, FormatTable)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "No matches found.") {
		t.Error("expected 'No matches found.' from RenderSearchResults table format")
	}
}

func TestRenderSearchResults_JSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	results := []model.SearchResult{
		{
			Session: model.Session{ID: "sr-test", Tool: model.ToolClaude},
			Matches: []model.SearchMatch{{Snippet: "found it", Role: model.RoleUser}},
		},
	}
	RenderSearchResults(results, FormatJSON)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "sr-test") {
		t.Error("expected session ID in search results JSON output")
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "clean string unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "preserves tab newline carriage return",
			input: "line1\tindented\nline2\rline3",
			want:  "line1\tindented\nline2\rline3",
		},
		{
			name:  "strips null byte",
			input: "hello\x00world",
			want:  "helloworld",
		},
		{
			name:  "strips ANSI escape sequence",
			input: "hello\x1b[31mred\x1b[0m",
			want:  "hello[31mred[0m",
		},
		{
			name:  "strips bell character",
			input: "alert\x07here",
			want:  "alerthere",
		},
		{
			name:  "strips backspace",
			input: "back\x08space",
			want:  "backspace",
		},
		{
			name:  "strips form feed and vertical tab",
			input: "form\x0cfeed\x0bvtab",
			want:  "formfeedvtab",
		},
		{
			name:  "strips all non-printable control chars",
			input: "\x00\x01\x02\x03\x04\x05\x06\x07\x08keep\x09\x0akeep\x0b\x0c\x0dkeep\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f",
			want:  "keep\t\nkeep\rkeep",
		},
		{
			name:  "preserves unicode",
			input: "caf\u00e9 \u2603 \U0001F600",
			want:  "caf\u00e9 \u2603 \U0001F600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeString(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSession(t *testing.T) {
	sess := &model.Session{
		ID:      "test-id",
		Tool:    model.ToolClaude,
		Title:   "title with \x00null",
		Preview: "preview with \x1bescape",
		Project: "/clean/path",
		Branch:  "feat/\x07bell-branch",
		Model:   "claude-\x00opus",
		Messages: []model.Message{
			{
				Role:    model.RoleUser,
				Content: "content with \x07bell and \x1b[31mANSI\x1b[0m",
			},
			{
				Role:    model.RoleAssistant,
				Content: "clean content",
				ToolCalls: []model.ToolCall{
					{
						Name:   "Read\x00File",
						Input:  "input with \x00null",
						Output: "output with \x08backspace",
					},
				},
			},
		},
	}

	sanitized := sanitizeSession(sess)

	// Original should not be modified.
	if sess.Title != "title with \x00null" {
		t.Error("sanitizeSession modified the original session Title")
	}
	if sess.Messages[0].Content != "content with \x07bell and \x1b[31mANSI\x1b[0m" {
		t.Error("sanitizeSession modified the original message Content")
	}

	// Sanitized copy should have control chars stripped.
	if sanitized.Title != "title with null" {
		t.Errorf("expected sanitized title, got %q", sanitized.Title)
	}
	if sanitized.Preview != "preview with escape" {
		t.Errorf("expected sanitized preview, got %q", sanitized.Preview)
	}
	if sanitized.Branch != "feat/bell-branch" {
		t.Errorf("expected sanitized branch, got %q", sanitized.Branch)
	}
	if sanitized.Model != "claude-opus" {
		t.Errorf("expected sanitized model, got %q", sanitized.Model)
	}
	if sanitized.Messages[0].Content != "content with bell and [31mANSI[0m" {
		t.Errorf("expected sanitized message content, got %q", sanitized.Messages[0].Content)
	}
	if sanitized.Messages[1].ToolCalls[0].Name != "ReadFile" {
		t.Errorf("expected sanitized tool call name, got %q", sanitized.Messages[1].ToolCalls[0].Name)
	}
	if sanitized.Messages[1].ToolCalls[0].Input != "input with null" {
		t.Errorf("expected sanitized tool call input, got %q", sanitized.Messages[1].ToolCalls[0].Input)
	}
	if sanitized.Messages[1].ToolCalls[0].Output != "output with backspace" {
		t.Errorf("expected sanitized tool call output, got %q", sanitized.Messages[1].ToolCalls[0].Output)
	}
}

func TestSanitizeSession_JSONRoundTrip(t *testing.T) {
	// Simulate the worst case: session with all control chars in content.
	content := "start"
	for c := 0; c < 0x20; c++ {
		content += string(rune(c))
	}
	content += "end"

	sess := &model.Session{
		ID:   "roundtrip-test",
		Tool: model.ToolClaude,
		Messages: []model.Message{
			{
				Role:    model.RoleUser,
				Content: content,
			},
		},
	}

	sanitized := sanitizeSession(sess)

	// Encode to JSON.
	jsonBytes, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify it's valid JSON.
	if !json.Valid(jsonBytes) {
		t.Fatalf("sanitized session produced invalid JSON: %s", string(jsonBytes))
	}

	// Verify it round-trips cleanly.
	var parsed model.Session
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\nJSON: %s", err, string(jsonBytes))
	}

	// The content should have tab, newline, carriage return preserved,
	// and all other control chars stripped.
	wantContent := "start\t\n\rend"
	if parsed.Messages[0].Content != wantContent {
		t.Errorf("round-trip content = %q, want %q", parsed.Messages[0].Content, wantContent)
	}
}
