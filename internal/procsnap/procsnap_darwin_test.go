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

func TestParsePS_Basic(t *testing.T) {
	raw := []byte(`    1     0 launchd          /sbin/launchd
  100     1 loginwindow      /System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow console
 3012   100 iTerm2           /Applications/iTerm.app/Contents/MacOS/iTerm2
 5674  3012 zsh              -zsh
52333  5674 claude           /usr/local/bin/claude
`)
	got, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(got))
	}
	p := got[52333]
	if p.PID != 52333 || p.PPID != 5674 || p.Command != "claude" {
		t.Errorf("wrong claude row: %+v", p)
	}
	if got[1].Command != "launchd" || got[1].PPID != 0 {
		t.Errorf("wrong launchd row: %+v", got[1])
	}
}

func TestParsePS_Empty(t *testing.T) {
	got, err := parsePS(nil)
	if err != nil {
		t.Fatalf("empty input must not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty input must yield empty map, got %d", len(got))
	}
}

func TestParsePS_SkipsMalformed(t *testing.T) {
	raw := []byte(`   1     0 launchd          /sbin/launchd
malformed line no fields
   5     1 zsh              -zsh
`)
	got, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 valid rows, got %d", len(got))
	}
}

func TestParsePS_CommandWithoutArgs(t *testing.T) {
	raw := []byte("  42     1 kernel_task\n")
	got, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}
	p, ok := got[42]
	if !ok {
		t.Fatal("row 42 missing")
	}
	if p.Command != "kernel_task" || p.Args != "" {
		t.Errorf("comm-only row wrong: %+v", p)
	}
}

func TestParsePS_ScannerError(t *testing.T) {
	// A single line exceeding the 1MB scanner buffer limit causes sc.Err() to
	// return bufio.ErrTooLong, exercising the error-return path.
	line := make([]byte, 1024*1024+1)
	for i := range line {
		line[i] = 'x'
	}
	line[len(line)-1] = '\n'
	_, err := parsePS(line)
	if err == nil {
		t.Fatal("expected error for oversized line, got nil")
	}
}

func TestParsePS_SkipsMalformedVariants(t *testing.T) {
	// Each case exercises a distinct silent-skip branch inside parsePS.
	cases := []struct {
		name string
		line string
	}{
		// trimmed == "": blank line (spaces only) is silently skipped.
		{"blank_line", "   \n"},
		// pidEnd <= 0: line has no space after trimming (single token).
		{"no_space_at_all", "12345\n"},
		// PID Atoi fails: non-numeric PID field.
		{"non_numeric_pid", "abc  1 zsh -zsh\n"},
		// ppidEnd <= 0: PPID field has no trailing space (only one token left after PID).
		{"ppid_no_space", "  1 2\n"},
		// PPID Atoi fails: non-numeric PPID field.
		{"non_numeric_ppid", "  1 abc zsh -zsh\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePS([]byte(tc.line))
			if err != nil {
				t.Fatalf("parsePS: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("expected 0 rows (malformed skipped), got %d: %v", len(got), got)
			}
		})
	}
}
