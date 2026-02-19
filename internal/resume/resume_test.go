package resume

import (
	"fmt"
	"testing"

	"github.com/psacconier/sessions/internal/model"
)

// mockResumer implements Resumer for testing.
type mockResumer struct {
	tool  model.Tool
	modes []Mode
}

func (m *mockResumer) Tool() model.Tool { return m.tool }
func (m *mockResumer) Modes() []Mode    { return m.modes }
func (m *mockResumer) Exec(session *model.Session, mode Mode) error {
	return fmt.Errorf("mock exec called")
}

func TestRegisterAndGet(t *testing.T) {
	// Save and restore registry.
	original := registry
	t.Cleanup(func() { registry = original })
	registry = map[model.Tool]Resumer{}

	r := &mockResumer{tool: model.ToolClaude, modes: []Mode{ModeResume}}
	Register(r)

	got, ok := Get(model.ToolClaude)
	if !ok {
		t.Fatal("Get(ToolClaude) returned false, want true")
	}
	if got.Tool() != model.ToolClaude {
		t.Errorf("Tool() = %q, want %q", got.Tool(), model.ToolClaude)
	}
}

func TestGet_NotRegistered(t *testing.T) {
	original := registry
	t.Cleanup(func() { registry = original })
	registry = map[model.Tool]Resumer{}

	_, ok := Get(model.ToolCursor)
	if ok {
		t.Error("Get(ToolCursor) returned true for unregistered tool")
	}
}

func TestModes(t *testing.T) {
	original := registry
	t.Cleanup(func() { registry = original })
	registry = map[model.Tool]Resumer{}

	r := &mockResumer{
		tool:  model.ToolClaude,
		modes: []Mode{ModeResume, ModeFork},
	}
	Register(r)

	tests := []struct {
		name    string
		tool    model.Tool
		wantLen int
		wantNil bool
	}{
		{
			name:    "registered tool returns modes",
			tool:    model.ToolClaude,
			wantLen: 2,
		},
		{
			name:    "unregistered tool returns nil",
			tool:    model.ToolCursor,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Modes(tt.tool)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Modes(%q) = %v, want nil", tt.tool, got)
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("Modes(%q) returned %d modes, want %d", tt.tool, len(got), tt.wantLen)
			}
		})
	}
}

func TestRegisterOverwrites(t *testing.T) {
	original := registry
	t.Cleanup(func() { registry = original })
	registry = map[model.Tool]Resumer{}

	r1 := &mockResumer{tool: model.ToolClaude, modes: []Mode{ModeResume}}
	r2 := &mockResumer{tool: model.ToolClaude, modes: []Mode{ModeResume, ModeFork}}

	Register(r1)
	Register(r2)

	got, ok := Get(model.ToolClaude)
	if !ok {
		t.Fatal("Get(ToolClaude) returned false")
	}
	if len(got.Modes()) != 2 {
		t.Errorf("expected overwritten resumer with 2 modes, got %d", len(got.Modes()))
	}
}

func TestErrUnsupportedMode(t *testing.T) {
	err := &ErrUnsupportedMode{Tool: model.ToolClaude, Mode: ModeTmux}
	want := "claude does not support resume mode \"tmux\""
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
