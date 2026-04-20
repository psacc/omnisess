//go:build darwin

package procsnap

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestEnumerate_HappyPath(t *testing.T) {
	// Two PID files in testdata; only 52333 will be alive.
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "sessions"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	origKill := killFn
	killFn = func(pid int) error {
		if pid == 52333 {
			return nil
		}
		return syscall.ESRCH
	}
	t.Cleanup(func() { killFn = origKill })

	origPS := psRunnerFn
	psRunnerFn = func() ([]byte, error) {
		return []byte(`    1     0 launchd          /sbin/launchd
 5674     1 zsh              -zsh
52333  5674 claude           /usr/local/bin/claude
`), nil
	}
	t.Cleanup(func() { psRunnerFn = origPS })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(snap.Sessions) != 1 {
		t.Fatalf("expected 1 live session, got %d", len(snap.Sessions))
	}
	s := snap.Sessions[0]
	if s.PID != 52333 {
		t.Errorf("pid = %d", s.PID)
	}
	if s.Name != "refactor auth" {
		t.Errorf("name = %q", s.Name)
	}
	if len(s.Ancestors) != 2 {
		t.Fatalf("expected 2 ancestors, got %d: %+v", len(s.Ancestors), s.Ancestors)
	}
	if s.Ancestors[0].Command != "zsh" || s.Ancestors[1].Command != "launchd" {
		t.Errorf("ancestors: %+v", s.Ancestors)
	}
	if snap.Built.IsZero() {
		t.Error("Built must be set")
	}
	if !snap.IsActive("eaa9deaf-6c97-4233-9629-9c425ebf4457") {
		t.Error("IsActive must match live session ID")
	}
}

func TestEnumerate_PSFails(t *testing.T) {
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "sessions"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	origKill := killFn
	killFn = func(pid int) error { return nil }
	t.Cleanup(func() { killFn = origKill })

	origPS := psRunnerFn
	psRunnerFn = func() ([]byte, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { psRunnerFn = origPS })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("ps failure must not propagate, got %v", err)
	}
	if len(snap.Sessions) == 0 {
		t.Error("sessions must still be returned (without ancestors) when ps fails")
	}
	if len(snap.Sessions[0].Ancestors) != 0 {
		t.Error("ancestors must be empty when ps failed")
	}
}

func TestEnumerate_NoSessionsDir(t *testing.T) {
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "nonexistent"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("missing dir must not error: %v", err)
	}
	if len(snap.Sessions) != 0 {
		t.Errorf("missing dir must yield empty snapshot, got %d sessions", len(snap.Sessions))
	}
}

func TestEnumerate_HomeDirError(t *testing.T) {
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return "", errors.New("no home")
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	_, err := Enumerate()
	if err == nil {
		t.Error("expected error when home dir cannot be resolved")
	}
}

func TestEnumerate_ScanDirError(t *testing.T) {
	// Pointing sessionsDirFn at a regular file (not a directory) triggers
	// the non-IsNotExist error branch inside scanSessionDir, which must
	// propagate through Enumerate.
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "sessions", "52333.json"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	_, err := Enumerate()
	if err == nil {
		t.Error("expected error when sessions dir is not a directory")
	}
}

func TestSessionsDirFnDefault(t *testing.T) {
	// Exercise the real sessionsDirFn to cover the function-literal body.
	dir, err := sessionsDirFn()
	if err != nil {
		t.Fatalf("sessionsDirFn: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join(".claude", "sessions")) {
		t.Errorf("unexpected sessions dir: %q", dir)
	}
}

func TestSessionsDirFnDefault_HomeUnset(t *testing.T) {
	// On macOS, os.UserHomeDir() returns an error when $HOME is unset.
	// This covers the err != nil branch inside the default sessionsDirFn.
	t.Setenv("HOME", "")
	_, err := sessionsDirFn()
	if err == nil {
		t.Error("expected error when HOME is unset")
	}
}

func TestPSRunnerFnDefault(t *testing.T) {
	// Exercise the real psRunnerFn so the function-literal body is covered.
	// `ps -Ao pid=,ppid=,comm=,args=` must succeed on any macOS test host
	// and always includes PID 1 (launchd).
	raw, err := psRunnerFn()
	if err != nil {
		t.Fatalf("psRunnerFn: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty ps output")
	}
	procs, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS on real ps output: %v", err)
	}
	if _, ok := procs[1]; !ok {
		t.Error("expected PID 1 (launchd) in real ps output")
	}
}
