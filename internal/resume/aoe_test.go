//go:build !windows

package resume

import (
	"os"
	"testing"
)

func TestFindAoESession_MissingBinary(t *testing.T) {
	// Override PATH so "aoe" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	_, err := FindAoESession("claude", "/tmp/project")
	if err == nil {
		t.Fatal("FindAoESession returned nil when aoe is not in PATH, want error")
	}
}

func TestExecInAoE_MissingBinary(t *testing.T) {
	// Override PATH so "aoe" cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	err := ExecInAoE("claude", "/tmp/project", "test title")
	if err == nil {
		t.Fatal("ExecInAoE returned nil when aoe is not in PATH, want error")
	}
}
