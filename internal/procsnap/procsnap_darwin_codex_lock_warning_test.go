//go:build darwin

package procsnap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexProcessSessions_LockWarnings(t *testing.T) {
	tests := []struct {
		name string
		root func(*testing.T) string
		warn string
	}{
		{name: "glob error", root: func(*testing.T) string { return "[" }, warn: "finding metadata"},
		{name: "metadata mismatch", root: mismatchedCodexMetadata, warn: "identifies session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns := captureWarnings(t)
			got := codexProcessSessions(7002, codexProc{threadIDs: []string{lockFixtureID}}, tt.root(t), codexProcs())
			if len(got) != 1 || got[0].SessionID != lockFixtureID {
				t.Fatalf("got %+v, want lock-derived session", got)
			}
			if !strings.Contains(warns.String(), tt.warn) {
				t.Errorf("warnings = %q, want %q", warns.String(), tt.warn)
			}
		})
	}
}

func mismatchedCodexMetadata(t *testing.T) string {
	root := t.TempDir()
	dir := filepath.Join(root, "2026", "09", "18")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(dir, "rollout-2026-09-18T12-00-00-"+lockFixtureID+".jsonl")
	meta := []byte(`{"type":"session_meta","payload":{"id":"019eb000-0000-7000-8000-00000000ffff"}}`)
	if err := os.WriteFile(rollout, meta, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
