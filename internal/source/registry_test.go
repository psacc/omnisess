package source

import (
	"testing"

	"github.com/psacc/omnisess/internal/model"
)

// mockSource implements Source for testing.
type mockSource struct {
	name model.Tool
}

func (m *mockSource) Name() model.Tool { return m.name }
func (m *mockSource) List(_ ListOptions) ([]model.Session, error) {
	return nil, nil
}
func (m *mockSource) Get(_ string) (*model.Session, error) {
	return nil, nil
}
func (m *mockSource) Search(_ string, _ ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

func TestRegisterAndAll(t *testing.T) {
	// Save original registry and restore after test
	original := registry
	t.Cleanup(func() { registry = original })

	// Reset registry
	registry = nil

	s1 := &mockSource{name: "test-tool-1"}
	s2 := &mockSource{name: "test-tool-2"}

	Register(s1)
	Register(s2)

	all := All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d sources, want 2", len(all))
	}
	if all[0].Name() != "test-tool-1" {
		t.Errorf("All()[0].Name() = %q, want test-tool-1", all[0].Name())
	}
	if all[1].Name() != "test-tool-2" {
		t.Errorf("All()[1].Name() = %q, want test-tool-2", all[1].Name())
	}
}

func TestByName(t *testing.T) {
	original := registry
	t.Cleanup(func() { registry = original })

	registry = nil

	s1 := &mockSource{name: "alpha"}
	s2 := &mockSource{name: "beta"}
	s3 := &mockSource{name: "alpha"}

	Register(s1)
	Register(s2)
	Register(s3)

	tests := []struct {
		name    string
		tool    model.Tool
		wantLen int
	}{
		{
			name:    "filter to alpha",
			tool:    "alpha",
			wantLen: 2,
		},
		{
			name:    "filter to beta",
			tool:    "beta",
			wantLen: 1,
		},
		{
			name:    "empty name returns all",
			tool:    "",
			wantLen: 3,
		},
		{
			name:    "non-existent tool",
			tool:    "gamma",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ByName(tt.tool)
			if len(got) != tt.wantLen {
				t.Errorf("ByName(%q) returned %d sources, want %d", tt.tool, len(got), tt.wantLen)
			}
		})
	}
}

func TestByName_CorrectSources(t *testing.T) {
	original := registry
	t.Cleanup(func() { registry = original })

	registry = nil

	s1 := &mockSource{name: "alpha"}
	s2 := &mockSource{name: "beta"}
	Register(s1)
	Register(s2)

	got := ByName("beta")
	if len(got) != 1 {
		t.Fatalf("expected 1 source, got %d", len(got))
	}
	if got[0].Name() != "beta" {
		t.Errorf("expected beta, got %q", got[0].Name())
	}
}
