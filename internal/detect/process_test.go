package detect

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
			input:  "this is a longer string that should be truncated",
			maxLen: 20,
			want:   "this is a longer ...",
		},
		{
			name:   "newlines replaced with spaces",
			input:  "line one\nline two\nline three",
			maxLen: 50,
			want:   "line one line two line three",
		},
		{
			name:   "newlines replaced then truncated",
			input:  "first line\nsecond line\nthird line",
			maxLen: 20,
			want:   "first line second...",
		},
		{
			name:   "carriage returns stripped",
			input:  "hello\r\nworld",
			maxLen: 50,
			want:   "hello world",
		},
		{
			name:   "leading and trailing whitespace trimmed",
			input:  "  hello world  ",
			maxLen: 50,
			want:   "hello world",
		},
		{
			name:   "maxLen 3 edge case",
			input:  "abcdef",
			maxLen: 3,
			want:   "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestIsFileRecentlyModified(t *testing.T) {
	dir := t.TempDir()

	// Create a file and verify it's recently modified
	recentFile := filepath.Join(dir, "recent.txt")
	if err := os.WriteFile(recentFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsFileRecentlyModified(recentFile, 5*time.Second) {
		t.Error("expected recently created file to be considered recently modified")
	}

	// Create a file and backdate its mtime
	oldFile := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(oldFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	if IsFileRecentlyModified(oldFile, 2*time.Minute) {
		t.Error("expected old file to NOT be considered recently modified")
	}

	// Non-existent file
	if IsFileRecentlyModified(filepath.Join(dir, "nonexistent.txt"), time.Hour) {
		t.Error("expected non-existent file to return false")
	}
}

func TestIsSessionActive(t *testing.T) {
	// Use a process name that definitely doesn't exist
	dir := t.TempDir()
	f := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// "xyznonexistent_tool_99" shouldn't match any running process
	if IsSessionActive("xyznonexistent_tool_99", f) {
		t.Error("expected IsSessionActive to return false for non-existent tool process")
	}
}

func TestIsToolRunning(t *testing.T) {
	// Unknown tool name should return false
	if IsToolRunning("unknown_tool_that_does_not_exist") {
		t.Error("expected unknown tool to not be running")
	}
}
