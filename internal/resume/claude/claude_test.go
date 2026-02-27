//go:build !windows

package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/resume"
)

// writeEmptyExec writes a zero-byte executable file to binDir/name.
// exec.LookPath finds it (execute bit is set), but syscall.Exec fails with
// ENOEXEC, preserving the test process and its coverage data.
func writeEmptyExec(t *testing.T, binDir, name string) {
	t.Helper()
	p := filepath.Join(binDir, name)
	if err := os.WriteFile(p, []byte{}, 0o755); err != nil {
		t.Fatalf("could not write empty exec %s: %v", name, err)
	}
}

func TestTool(t *testing.T) {
	r := &claudeResumer{}
	if got := r.Tool(); got != model.ToolClaude {
		t.Errorf("Tool() = %q, want %q", got, model.ToolClaude)
	}
}

func TestModes(t *testing.T) {
	r := &claudeResumer{}
	modes := r.Modes()

	want := []resume.Mode{resume.ModeResume, resume.ModeFork, resume.ModeTmux, resume.ModeAoE}
	if len(modes) != len(want) {
		t.Fatalf("Modes() returned %d modes, want %d", len(modes), len(want))
	}
	for i, m := range want {
		if modes[i] != m {
			t.Errorf("Modes()[%d] = %q, want %q", i, modes[i], m)
		}
	}
}

func TestExec_UnsupportedMode(t *testing.T) {
	r := &claudeResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolClaude}

	err := r.Exec(sess, resume.ModeOpen)
	if err == nil {
		t.Fatal("Exec(ModeOpen) returned nil, want error")
	}

	unsup, ok := err.(*resume.ErrUnsupportedMode)
	if !ok {
		t.Fatalf("error type = %T, want *resume.ErrUnsupportedMode", err)
	}
	if unsup.Tool != model.ToolClaude {
		t.Errorf("ErrUnsupportedMode.Tool = %q, want %q", unsup.Tool, model.ToolClaude)
	}
	if unsup.Mode != resume.ModeOpen {
		t.Errorf("ErrUnsupportedMode.Mode = %q, want %q", unsup.Mode, resume.ModeOpen)
	}
}

func TestExec_MissingBinary(t *testing.T) {
	r := &claudeResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolClaude}

	// Override PATH so "claude" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	for _, mode := range []resume.Mode{resume.ModeResume, resume.ModeFork} {
		t.Run(string(mode), func(t *testing.T) {
			err := r.Exec(sess, mode)
			if err == nil {
				t.Fatalf("Exec(%s) returned nil when claude is not in PATH, want error", mode)
			}
		})
	}
}

func TestExec_MissingTmuxBinary(t *testing.T) {
	r := &claudeResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolClaude, Project: t.TempDir()}

	// Override PATH so "tmux" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	err := r.Exec(sess, resume.ModeTmux)
	if err == nil {
		t.Fatal("Exec(ModeTmux) returned nil when tmux is not in PATH, want error")
	}
}

func TestExec_MissingAoEBinary(t *testing.T) {
	r := &claudeResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolClaude, Project: t.TempDir()}

	// Override PATH so "aoe" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	err := r.Exec(sess, resume.ModeAoE)
	if err == nil {
		t.Fatal("Exec(ModeAoE) returned nil when aoe is not in PATH, want error")
	}
}

// TestExecDirect_WithProject exercises the execDirect code path when
// session.Project is set and the claude binary is an empty executable.
// Two sub-tests cover: chdir succeeds (valid dir) and chdir fails (missing dir),
// exercising the chdir warning branch. syscall.Exec fails on the empty binary,
// so coverage data is preserved.
func TestExecDirect_WithProject(t *testing.T) {
	r := &claudeResumer{}
	binDir := t.TempDir()
	writeEmptyExec(t, binDir, "claude")
	t.Setenv("PATH", binDir)

	t.Run("chdir succeeds", func(t *testing.T) {
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(origDir) })
		sess := &model.Session{ID: "abc-123", Tool: model.ToolClaude, Project: t.TempDir()}
		execErr := r.Exec(sess, resume.ModeResume)
		if execErr == nil {
			t.Fatal("Exec returned nil with empty claude binary, want error (exec format error)")
		}
	})

	t.Run("chdir fails (nonexistent dir)", func(t *testing.T) {
		sess := &model.Session{
			ID:      "abc-123",
			Tool:    model.ToolClaude,
			Project: "/nonexistent_dir_omnisess_test_xyz",
		}
		err := r.Exec(sess, resume.ModeResume)
		if err == nil {
			t.Fatal("Exec returned nil with empty claude binary, want error (exec format error)")
		}
	})
}

// TestExecAoE_EmptyProject exercises the execAoE branch where session.Project
// is empty, causing dir to be set to ".". The empty aoe executable causes
// syscall.Exec to fail, preserving coverage data.
func TestExecAoE_EmptyProject(t *testing.T) {
	r := &claudeResumer{}
	binDir := t.TempDir()
	writeEmptyExec(t, binDir, "aoe")
	t.Setenv("PATH", binDir)

	sess := &model.Session{ID: "abc-123", Tool: model.ToolClaude, Project: ""}
	err := r.Exec(sess, resume.ModeAoE)
	if err == nil {
		t.Fatal("Exec(ModeAoE) with empty project returned nil, want error")
	}
}

// TestExecTmux_EmptyProject exercises the execTmux branch where session.Project
// is empty, causing dir to be set to ".". The empty tmux executable causes
// syscall.Exec to fail, preserving coverage data.
func TestExecTmux_EmptyProject(t *testing.T) {
	r := &claudeResumer{}
	binDir := t.TempDir()
	writeEmptyExec(t, binDir, "tmux")
	t.Setenv("PATH", binDir)

	sess := &model.Session{ID: "abc-123", Tool: model.ToolClaude, Project: ""}
	err := r.Exec(sess, resume.ModeTmux)
	if err == nil {
		t.Fatal("Exec(ModeTmux) with empty project returned nil, want error")
	}
}

func TestBuildArgv(t *testing.T) {
	tests := []struct {
		name      string
		mode      resume.Mode
		sessionID string
		want      []string
	}{
		{
			name:      "resume mode",
			mode:      resume.ModeResume,
			sessionID: "abc-123",
			want:      []string{"claude", "--resume", "abc-123"},
		},
		{
			name:      "fork mode",
			mode:      resume.ModeFork,
			sessionID: "abc-123",
			want:      []string{"claude", "--resume", "abc-123", "--fork-session"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgv(tt.mode, tt.sessionID)
			if len(got) != len(tt.want) {
				t.Fatalf("buildArgv(%s, %q) returned %d args, want %d: %v", tt.mode, tt.sessionID, len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("buildArgv(%s, %q)[%d] = %q, want %q", tt.mode, tt.sessionID, i, got[i], tt.want[i])
				}
			}
		})
	}
}
