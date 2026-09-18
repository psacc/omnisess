//go:build darwin

package procsnap

import (
	"path/filepath"
	"testing"
)

func TestCodexSessions_TruncatedCommand(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{name: "plain path", args: "/Users/example/.codex/bin/codex app-server"},
		{name: "spaced path", args: "/Applications/AI Tools/Codex.app/Contents/Resources/codex app-server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubCodexDir(t)
			stubCodexLsof(t, func([]int) ([]byte, error) {
				lock := filepath.Join("testdata", "thread-writer-locks", lockFixtureID+".lock")
				return []byte("p7002\nfcwd\nn/Users/example\nf24\nn" + lock + "\n"), nil
			})
			procs := codexProcs()
			appServer := procs[7002]
			appServer.Command = "/Applications/AI"
			appServer.Args = tt.args
			procs[7002] = appServer
			got := codexSessions(procs)
			if len(got) != 1 || got[0].PID != 7002 || got[0].SessionID != lockFixtureID {
				t.Fatalf("got %+v, want app-server session", got)
			}
		})
	}
}
