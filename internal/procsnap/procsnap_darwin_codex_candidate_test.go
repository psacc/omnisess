//go:build darwin

package procsnap

import (
	"path/filepath"
	"testing"
)

func TestCodexSessions_TruncatedCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    string
		want    bool
	}{
		{name: "plain path", command: "/Users/example/.", args: "/Users/example/.codex/bin/codex app-server", want: true},
		{name: "spaced path", command: "/Applications/AI", args: "/Applications/AI Tools/Codex.app/Contents/Resources/codex app-server", want: true},
		{name: "argument only", command: "tail", args: "tail -f /tmp/codex", want: false},
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
			appServer.Command = tt.command
			appServer.Args = tt.args
			procs[7002] = appServer
			got := codexSessions(procs)
			if tt.want && (len(got) != 1 || got[0].PID != 7002 || got[0].SessionID != lockFixtureID) {
				t.Fatalf("got %+v, want app-server session", got)
			}
			if !tt.want && len(got) != 0 {
				t.Fatalf("got %+v, want no session", got)
			}
		})
	}
}
