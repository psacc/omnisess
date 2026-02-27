//go:build !windows

package resume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecInTmux_MissingBinary(t *testing.T) {
	// Override PATH so "tmux" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	err := ExecInTmux("test-session", "/tmp", []string{"echo", "hello"})
	if err == nil {
		t.Fatal("ExecInTmux returned nil when tmux is not in PATH, want error")
	}
}

// TestExecInTmux_ExecFormatError exercises the code path where tmux is found
// by LookPath but syscall.Exec fails. An empty (zero-byte) executable is placed
// on PATH: LookPath finds it (the execute bit is set), but the kernel rejects it
// with ENOEXEC (exec format error). This covers the shelljoin call, the argv
// construction, and the return syscall.Exec(...) statement while keeping the
// test process alive so that coverage data is preserved.
func TestExecInTmux_ExecFormatError(t *testing.T) {
	binDir := t.TempDir()
	// Write a zero-byte file with the execute bit set.
	tmuxPath := filepath.Join(binDir, "tmux")
	if err := os.WriteFile(tmuxPath, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	err := ExecInTmux("sess-test", t.TempDir(), []string{"claude", "--resume", "abc123"})
	if err == nil {
		t.Fatal("ExecInTmux with empty tmux binary returned nil, want error (exec format error)")
	}
}

func TestTmuxSessionName(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		sessionID string
		want      string
	}{
		{
			name:      "short session ID kept as-is",
			tool:      "claude",
			sessionID: "abc123",
			want:      "sessions_claude_abc123",
		},
		{
			name:      "long session ID truncated to 8 chars",
			tool:      "cursor",
			sessionID: "5c3f2742-a1b2-c3d4-e5f6",
			want:      "sessions_cursor_5c3f2742",
		},
		{
			name:      "exactly 8 chars",
			tool:      "claude",
			sessionID: "12345678",
			want:      "sessions_claude_12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TmuxSessionName(tt.tool, tt.sessionID)
			if got != tt.want {
				t.Errorf("TmuxSessionName(%q, %q) = %q, want %q", tt.tool, tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestShelljoin(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "simple args",
			argv: []string{"claude", "--resume", "abc123"},
			want: "'claude' '--resume' 'abc123'",
		},
		{
			name: "arg with single quote",
			argv: []string{"echo", "it's"},
			want: `'echo' 'it'\''s'`,
		},
		{
			name: "arg with spaces",
			argv: []string{"cmd", "hello world"},
			want: "'cmd' 'hello world'",
		},
		{
			name: "single element",
			argv: []string{"cmd"},
			want: "'cmd'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shelljoin(tt.argv)
			if got != tt.want {
				t.Errorf("shelljoin(%v) = %q, want %q", tt.argv, got, tt.want)
			}
		})
	}
}
