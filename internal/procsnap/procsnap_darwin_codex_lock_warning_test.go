//go:build darwin

package procsnap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexProcessSessions_LockWarnings(t *testing.T) {
	t.Run("glob error", func(t *testing.T) {
		warns := captureWarnings(t)
		got := codexProcessSessions(7002, codexProc{threadIDs: []string{lockFixtureID}}, "[", codexProcs())
		if len(got) != 1 || got[0].SessionID != lockFixtureID {
			t.Fatalf("got %+v, want lock-derived session", got)
		}
		if !strings.Contains(warns.String(), "finding metadata") {
			t.Errorf("warnings = %q", warns.String())
		}
	})
	t.Run("metadata mismatch", func(t *testing.T) {
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
		warns := captureWarnings(t)
		got := codexProcessSessions(7002, codexProc{threadIDs: []string{lockFixtureID}}, root, codexProcs())
		if len(got) != 1 || got[0].SessionID != lockFixtureID {
			t.Fatalf("got %+v, want lock-derived session", got)
		}
		if !strings.Contains(warns.String(), "identifies session") {
			t.Errorf("warnings = %q", warns.String())
		}
	})
}
