//go:build darwin

package procsnap

import (
	"path/filepath"
	"testing"
)

const lockFixtureID = "019eb000-0000-7000-8000-00000000a001"

func TestCodexSessions_ThreadLockWithoutOpenRollout(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) {
		path := filepath.Join("testdata", "thread-writer-locks", lockFixtureID+".lock")
		return []byte("p7002\nfcwd\nn/Users/example\nf24\nn" + path + "\n"), nil
	})

	got := codexSessions(codexProcs())
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1: %+v", len(got), got)
	}
	session := got[0]
	if session.PID != 7002 || session.SessionID != lockFixtureID {
		t.Errorf("identity = (%d, %q), want (7002, %q)", session.PID, session.SessionID, lockFixtureID)
	}
	if session.CWD != "/Users/me/prj/foo" || session.Version != "0.138.0" {
		t.Errorf("metadata = (%q, %q), want fixture session metadata", session.CWD, session.Version)
	}
}

func TestCodexSessions_ThreadLockAndRolloutProduceOneSession(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) {
		lock := filepath.Join("testdata", "thread-writer-locks", lockFixtureID+".lock")
		rollout := fixtureRollout("rollout-2026-06-09T12-00-00-" + lockFixtureID + ".jsonl")
		return []byte("p7001\nfcwd\nn/Users/example\nf24\nn" + lock + "\nf25\nn" + rollout + "\n"), nil
	})

	got := codexSessions(codexProcs())
	if len(got) != 1 || got[0].SessionID != lockFixtureID {
		t.Fatalf("got %+v, want one %s session", got, lockFixtureID)
	}
}

func TestCodexSessions_AppServerWithTruncatedCommand(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) {
		lock := filepath.Join("testdata", "thread-writer-locks", lockFixtureID+".lock")
		return []byte("p7002\nfcwd\nn/Users/example\nf24\nn" + lock + "\n"), nil
	})
	procs := codexProcs()
	appServer := procs[7002]
	appServer.Command = "/Users/example/.c"
	appServer.Args = "/Users/example/.codex/bin/codex app-server"
	procs[7002] = appServer

	got := codexSessions(procs)
	if len(got) != 1 || got[0].PID != 7002 || got[0].SessionID != lockFixtureID {
		t.Fatalf("got %+v, want app-server session %s", got, lockFixtureID)
	}
}
