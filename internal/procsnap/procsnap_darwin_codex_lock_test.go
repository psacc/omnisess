//go:build darwin

package procsnap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lockFixtureID = "019eb000-0000-7000-8000-00000000a001"

func lockSessions(t *testing.T, sessionsDir, lock, rollout string) ([]Session, string) {
	t.Helper()
	orig := codexSessionsDirFn
	codexSessionsDirFn = func() (string, error) { return sessionsDir, nil }
	t.Cleanup(func() { codexSessionsDirFn = orig })
	stubCodexLsof(t, func([]int) ([]byte, error) {
		raw := "p7002\nfcwd\nn/Users/example\nf24\nn" + lock + "\n"
		if rollout != "" {
			raw += "f25\nn" + rollout + "\n"
		}
		return []byte(raw), nil
	})
	warns := captureWarnings(t)
	return codexSessions(codexProcs()), warns.String()
}

func TestCodexSessions_ThreadLocks(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T) ([]Session, string)
		warn string
	}{
		{name: "metadata", run: func(t *testing.T) ([]Session, string) {
			dir := filepath.Join("testdata", "codex-sessions")
			lock := filepath.Join("testdata", "thread-writer-locks", lockFixtureID+".lock")
			return lockSessions(t, dir, lock, "")
		}},
		{name: "deduplicated rollout", run: func(t *testing.T) ([]Session, string) {
			dir := filepath.Join("testdata", "codex-sessions")
			lock := filepath.Join("testdata", "thread-writer-locks", lockFixtureID+".lock")
			rollout := fixtureRollout("rollout-2026-06-09T12-00-00-" + lockFixtureID + ".jsonl")
			return lockSessions(t, dir, lock, rollout)
		}},
		{name: "symlinked sessions", run: func(t *testing.T) ([]Session, string) {
			root := t.TempDir()
			root, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			home := filepath.Join(root, ".codex")
			locks := filepath.Join(home, "thread-writer-locks")
			external := filepath.Join(root, "external-sessions")
			for _, dir := range []string{home, locks, external} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(external, filepath.Join(home, "sessions")); err != nil {
				t.Fatal(err)
			}
			return lockSessions(t, filepath.Join(home, "sessions"), filepath.Join(locks, lockFixtureID+".lock"), "")
		}},
		{name: "corrupt metadata warns", warn: "reading", run: func(t *testing.T) ([]Session, string) {
			dir := t.TempDir()
			dir, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			rolloutDir := filepath.Join(dir, "2026", "09", "18")
			if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
				t.Fatal(err)
			}
			rollout := filepath.Join(rolloutDir, "rollout-2026-09-18T12-00-00-"+lockFixtureID+".jsonl")
			if err := os.WriteFile(rollout, []byte("not json\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			lock := filepath.Join(filepath.Dir(dir), "thread-writer-locks", lockFixtureID+".lock")
			return lockSessions(t, dir, lock, "")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warns := tt.run(t)
			if len(got) != 1 || got[0].PID != 7002 || got[0].SessionID != lockFixtureID {
				t.Fatalf("got %+v, want one lock-derived session", got)
			}
			if tt.name == "metadata" && (got[0].CWD != "/Users/me/prj/foo" || got[0].Version != "0.138.0") {
				t.Errorf("metadata = (%q, %q)", got[0].CWD, got[0].Version)
			}
			if !strings.Contains(warns, tt.warn) {
				t.Errorf("warnings = %q, want %q", warns, tt.warn)
			}
		})
	}
}
