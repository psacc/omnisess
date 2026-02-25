package detect

import (
	"os"
	"os/exec"
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

// TestIsToolRunning_KnownTools exercises the switch branches for each known
// tool name. We don't assert running/not-running (environment-dependent)
// — we only verify the function doesn't panic and returns a bool.
func TestIsToolRunning_KnownTools(t *testing.T) {
	tools := []string{"claude", "cursor", "codex", "gemini"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			_ = IsToolRunning(tool)
		})
	}
}

// TestIsSessionTreeRecentlyModified tests the unexported helper directly.
func TestIsSessionTreeRecentlyModified(t *testing.T) {
	dir := t.TempDir()

	t.Run("main file recently modified", func(t *testing.T) {
		sessionFile := filepath.Join(dir, "session1.jsonl")
		if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !isSessionTreeRecentlyModified(sessionFile, time.Hour) {
			t.Error("expected true when main session file is recent")
		}
	})

	t.Run("main file old, subagent recent", func(t *testing.T) {
		sessionFile := filepath.Join(dir, "session2.jsonl")
		if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Backdate the main file.
		old := time.Now().Add(-1 * time.Hour)
		if err := os.Chtimes(sessionFile, old, old); err != nil {
			t.Fatal(err)
		}

		// Create a recent subagent file.
		subDir := filepath.Join(dir, "session2", "subagents")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		agentFile := filepath.Join(subDir, "agent-001.jsonl")
		if err := os.WriteFile(agentFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		if !isSessionTreeRecentlyModified(sessionFile, 5*time.Minute) {
			t.Error("expected true when a subagent file is recent")
		}
	})

	t.Run("main file old, subagent also old", func(t *testing.T) {
		sessionFile := filepath.Join(dir, "session3.jsonl")
		if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-1 * time.Hour)
		if err := os.Chtimes(sessionFile, old, old); err != nil {
			t.Fatal(err)
		}

		subDir := filepath.Join(dir, "session3", "subagents")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		agentFile := filepath.Join(subDir, "agent-001.jsonl")
		if err := os.WriteFile(agentFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(agentFile, old, old); err != nil {
			t.Fatal(err)
		}

		if isSessionTreeRecentlyModified(sessionFile, 5*time.Minute) {
			t.Error("expected false when all files are old")
		}
	})

	t.Run("main file old, no subagents directory", func(t *testing.T) {
		sessionFile := filepath.Join(dir, "session4.jsonl")
		if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-1 * time.Hour)
		if err := os.Chtimes(sessionFile, old, old); err != nil {
			t.Fatal(err)
		}

		if isSessionTreeRecentlyModified(sessionFile, 5*time.Minute) {
			t.Error("expected false when main file is old and no subagents exist")
		}
	})

	t.Run("nonexistent session file, no subagents", func(t *testing.T) {
		sessionFile := filepath.Join(dir, "nonexistent.jsonl")
		if isSessionTreeRecentlyModified(sessionFile, time.Hour) {
			t.Error("expected false for nonexistent session file with no subagents")
		}
	})
}

// TestIsSessionActive_ToolRunning exercises the branch where IsToolRunning
// returns true so that isSessionTreeRecentlyModified is also evaluated.
// We spawn a long-sleep subprocess named "codex" and verify IsSessionActive
// evaluates the file-recency check.
func TestIsSessionActive_ToolRunning(t *testing.T) {
	// Write a shell script that just sleeps, so pgrep -x codex can find it.
	binDir := t.TempDir()
	fakeBin := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Start the fake "codex" process.
	cmd := exec.Command(fakeBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start fake codex process: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	dir := t.TempDir()

	// Recent session file → should return true.
	recentFile := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(recentFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsSessionActive("codex", recentFile) {
		t.Error("expected IsSessionActive to return true when tool is running and file is recent")
	}

	// Old session file → should return false even though tool is running.
	oldFile := filepath.Join(dir, "old.jsonl")
	if err := os.WriteFile(oldFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(oldFile, old, old); err != nil {
		t.Fatal(err)
	}
	if IsSessionActive("codex", oldFile) {
		t.Error("expected IsSessionActive to return false when tool is running but file is old")
	}
}
