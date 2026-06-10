//go:build darwin

package procsnap

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestScanSessionDir_HappyPath(t *testing.T) {
	entries, err := scanSessionDir(filepath.Join("testdata", "sessions"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries (garbage.json skipped), got %d", len(entries))
	}
	byPID := map[int]sessionFile{}
	for _, e := range entries {
		byPID[e.PID] = e
	}
	got := byPID[52333]
	if got.SessionID != "eaa9deaf-6c97-4233-9629-9c425ebf4457" {
		t.Errorf("wrong sessionId: %q", got.SessionID)
	}
	if got.Status != "busy" {
		t.Errorf("status not parsed: %q", got.Status)
	}
	if got.Name != "refactor auth" {
		t.Errorf("name not parsed: %q", got.Name)
	}
	if got.Entrypoint != "cli" {
		t.Errorf("entrypoint: %q", got.Entrypoint)
	}
	wantStart := time.UnixMilli(1776680734830)
	if !got.StartedAt.Equal(wantStart) {
		t.Errorf("startedAt = %v, want %v", got.StartedAt, wantStart)
	}

	desk := byPID[9999]
	if desk.Entrypoint != "claude-desktop" {
		t.Errorf("desktop entrypoint: %q", desk.Entrypoint)
	}
	if desk.Name != "" {
		t.Errorf("desktop name should be empty, got %q", desk.Name)
	}
}

func TestScanSessionDir_Missing(t *testing.T) {
	entries, err := scanSessionDir(filepath.Join("testdata", "nonexistent"))
	if err != nil {
		t.Fatalf("missing dir must not error, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("missing dir must yield empty slice, got %d", len(entries))
	}
}

func TestScanSessionDir_NotADir(t *testing.T) {
	// Passing a regular file as the dir triggers the non-IsNotExist ReadDir error path.
	_, err := scanSessionDir(filepath.Join("testdata", "sessions", "52333.json"))
	if err == nil {
		t.Fatal("expected error when dir is a regular file, got nil")
	}
}

func TestScanSessionDir_SkipsInvalidEntries(t *testing.T) {
	dir := t.TempDir()
	// Missing sessionId — should be silently skipped (PID/SessionID guard).
	if err := os.WriteFile(filepath.Join(dir, "noid.json"), []byte(`{"pid":42,"sessionId":""}`), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	entries, err := scanSessionDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (invalid entry skipped), got %d", len(entries))
	}
}

func TestScanSessionDir_SkipsNonJSONAndDirs(t *testing.T) {
	dir := t.TempDir()
	// A subdirectory — should be skipped.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}
	// A non-JSON file — should be skipped.
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("setup txt: %v", err)
	}
	// One valid JSON so we know the scanner ran.
	if err := os.WriteFile(filepath.Join(dir, "1.json"), []byte(`{"pid":1,"sessionId":"s1"}`), 0o644); err != nil {
		t.Fatalf("setup json: %v", err)
	}
	entries, err := scanSessionDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestScanSessionDir_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.json")
	if err := os.WriteFile(path, []byte(`{"pid":1,"sessionId":"x"}`), 0o000); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Should not error — unreadable file is skipped with a stderr warning.
	entries, err := scanSessionDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (locked file skipped), got %d", len(entries))
	}
}

func TestKillFnDefault(t *testing.T) {
	// Exercise the real killFn with os.Getpid() so the function-literal body
	// is covered. This guarantees 100% statement coverage.
	if err := killFn(os.Getpid()); err != nil {
		t.Errorf("killFn(self) = %v, want nil", err)
	}
}

func TestFilterAlive(t *testing.T) {
	orig := killFn
	killFn = func(pid int) error {
		if pid == 52333 {
			return nil // alive
		}
		return syscall.ESRCH // dead
	}
	t.Cleanup(func() { killFn = orig })

	in := []sessionFile{
		{PID: 52333, SessionID: "a"},
		{PID: 9999, SessionID: "b"},
		{PID: 42, SessionID: "c"},
	}
	got := filterAlive(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 alive, got %d", len(got))
	}
	if got[0].PID != 52333 {
		t.Errorf("wrong alive PID: %d", got[0].PID)
	}
}
