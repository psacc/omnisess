//go:build !windows

package cursor

import (
	"os"
	"testing"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/resume"
)

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
