//go:build !windows

package cursor

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
	r := &cursorResumer{}
	if got := r.Tool(); got != model.ToolCursor {
		t.Errorf("Tool() = %q, want %q", got, model.ToolCursor)
	}
}

func TestModes(t *testing.T) {
	r := &cursorResumer{}
	modes := r.Modes()

	want := []resume.Mode{resume.ModeResume, resume.ModeTmux, resume.ModeAoE}
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
	r := &cursorResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolCursor}

	err := r.Exec(sess, resume.ModeFork)
	if err == nil {
		t.Fatal("Exec(ModeFork) returned nil, want error")
	}

	unsup, ok := err.(*resume.ErrUnsupportedMode)
	if !ok {
		t.Fatalf("error type = %T, want *resume.ErrUnsupportedMode", err)
	}
	if unsup.Tool != model.ToolCursor {
		t.Errorf("ErrUnsupportedMode.Tool = %q, want %q", unsup.Tool, model.ToolCursor)
	}
	if unsup.Mode != resume.ModeFork {
		t.Errorf("ErrUnsupportedMode.Mode = %q, want %q", unsup.Mode, resume.ModeFork)
	}
}

func TestExec_MissingBinary(t *testing.T) {
	r := &cursorResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolCursor}

	// Override PATH so "cursor" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	err := r.Exec(sess, resume.ModeResume)
	if err == nil {
		t.Fatal("Exec(ModeResume) returned nil when cursor is not in PATH, want error")
	}
}

func TestExec_MissingTmuxBinary(t *testing.T) {
	r := &cursorResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolCursor, Project: t.TempDir()}

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
	r := &cursorResumer{}
	sess := &model.Session{ID: "test-id", Tool: model.ToolCursor, Project: t.TempDir()}

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
// session.Project is set and the cursor binary is an empty executable.
// Two sub-tests cover: chdir succeeds (valid dir) and chdir fails (missing dir),
// exercising the chdir warning branch. syscall.Exec fails on the empty binary,
// so coverage data is preserved.
func TestExecDirect_WithProject(t *testing.T) {
	r := &cursorResumer{}
	binDir := t.TempDir()
	writeEmptyExec(t, binDir, "cursor")
	t.Setenv("PATH", binDir)

	t.Run("chdir succeeds", func(t *testing.T) {
		sess := &model.Session{ID: "abc-123", Tool: model.ToolCursor, Project: t.TempDir()}
		err := r.Exec(sess, resume.ModeResume)
		if err == nil {
			t.Fatal("Exec returned nil with empty cursor binary, want error (exec format error)")
		}
	})

	t.Run("chdir fails (nonexistent dir)", func(t *testing.T) {
		sess := &model.Session{
			ID:      "abc-123",
			Tool:    model.ToolCursor,
			Project: "/nonexistent_dir_omnisess_test_xyz",
		}
		err := r.Exec(sess, resume.ModeResume)
		if err == nil {
			t.Fatal("Exec returned nil with empty cursor binary, want error (exec format error)")
		}
	})
}

// TestExecAoE_EmptyProject exercises the execAoE branch where session.Project
// is empty, causing dir to be set to ".". The empty aoe executable causes
// syscall.Exec to fail, preserving coverage data.
func TestExecAoE_EmptyProject(t *testing.T) {
	r := &cursorResumer{}
	binDir := t.TempDir()
	writeEmptyExec(t, binDir, "aoe")
	t.Setenv("PATH", binDir)

	sess := &model.Session{ID: "abc-123", Tool: model.ToolCursor, Project: ""}
	err := r.Exec(sess, resume.ModeAoE)
	if err == nil {
		t.Fatal("Exec(ModeAoE) with empty project returned nil, want error")
	}
}

// TestExecTmux_EmptyProject exercises the execTmux branch where session.Project
// is empty, causing dir to be set to ".". The empty tmux executable causes
// syscall.Exec to fail, preserving coverage data.
func TestExecTmux_EmptyProject(t *testing.T) {
	r := &cursorResumer{}
	binDir := t.TempDir()
	writeEmptyExec(t, binDir, "tmux")
	t.Setenv("PATH", binDir)

	sess := &model.Session{ID: "abc-123", Tool: model.ToolCursor, Project: ""}
	err := r.Exec(sess, resume.ModeTmux)
	if err == nil {
		t.Fatal("Exec(ModeTmux) with empty project returned nil, want error")
	}
}

func TestBuildArgv(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      []string
	}{
		{
			name:      "basic session ID",
			sessionID: "abc-123",
			want:      []string{"cursor", "agent", "--resume", "abc-123"},
		},
		{
			name:      "long session ID",
			sessionID: "5c3f2742-a1b2-c3d4-e5f6-789012345678",
			want:      []string{"cursor", "agent", "--resume", "5c3f2742-a1b2-c3d4-e5f6-789012345678"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgv(tt.sessionID)
			if len(got) != len(tt.want) {
				t.Fatalf("buildArgv(%q) returned %d args, want %d: %v", tt.sessionID, len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("buildArgv(%q)[%d] = %q, want %q", tt.sessionID, i, got[i], tt.want[i])
				}
			}
		})
	}
}
