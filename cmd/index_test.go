package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/psacc/omnisess/internal/index"
)

// withTempIndex points OMNISESS_INDEX_PATH at a fresh temp SQLite file for
// the duration of the test.
func withTempIndex(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-index.sqlite")
	t.Setenv("OMNISESS_INDEX_PATH", dbPath)
	return dbPath
}

// resetIndexFlags resets the cmd/index flags between tests.
func resetIndexFlags() {
	flagIndexAll = false
	flagIndexFull = false
	flagIndexRebuild = false
}

func TestRunIndex_RequiresAll(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetIndexFlags()
	// flagIndexAll = false
	err := runIndex(newNoopCmd(), nil)
	if err == nil {
		t.Errorf("expected --all required error")
	}
}

func TestRunIndex_NoSessionsHappy(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	// HOME is jailed to an empty tmp dir, so Claude returns no sessions.
	var buf strings.Builder
	if err := runIndexTo(&buf); err != nil {
		t.Errorf("runIndexTo: %v", err)
	}
	if !strings.Contains(buf.String(), "Indexed 0 session(s)") {
		t.Errorf("expected zero-session report, got: %q", buf.String())
	}
}

func TestRunIndex_OpenFailure(t *testing.T) {
	// Point the index at a path whose parent is a regular file (so mkdir fails).
	regular := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNISESS_INDEX_PATH", filepath.Join(regular, "nested", "index.sqlite"))
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	var buf strings.Builder
	if err := runIndexTo(&buf); err == nil {
		t.Errorf("expected open-index error")
	}
}

func TestRunIndex_CobraAdapter(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	silenceOutput(t)
	if err := runIndex(newNoopCmd(), nil); err != nil {
		t.Errorf("runIndex (cobra adapter): %v", err)
	}
}

// TestOpenDefaultIndex_PathError covers the DefaultPath error branch.
func TestOpenDefaultIndex_PathError(t *testing.T) {
	t.Setenv("OMNISESS_INDEX_PATH", "")
	t.Setenv("HOME", "")
	// On Linux UserCacheDir also honours XDG_CACHE_HOME; unset both.
	t.Setenv("XDG_CACHE_HOME", "")
	if _, err := openDefaultIndex(); err == nil {
		t.Errorf("expected DefaultPath error when HOME unset")
	}
}

// TestRunIndex_DefaultPathError covers runIndexTo's openDefaultIndex error
// branch by unsetting HOME — but only after registering the --all flag.
func TestRunIndex_DefaultPathError(t *testing.T) {
	t.Setenv("OMNISESS_INDEX_PATH", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	var buf strings.Builder
	if err := runIndexTo(&buf); err == nil {
		t.Errorf("expected openDefaultIndex error")
	}
}

// seedFakeClaudeSessions writes minimal Claude JSONL files + a history.jsonl
// entry under the jailed $HOME so that runIndexTo iterates real session data.
func seedFakeClaudeSessions(t *testing.T, n int) {
	t.Helper()
	home := os.Getenv("HOME")
	projDir := filepath.Join(home, ".claude", "projects", "-tmp-fake-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	histPath := filepath.Join(home, ".claude", "history.jsonl")
	histFile, err := os.OpenFile(histPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer histFile.Close()
	for i := 0; i < n; i++ {
		id := makeUUID(i)
		jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}},"uuid":"a1","timestamp":"2026-05-15T09:00:01.000Z","model":"claude-opus-4-7"}` + "\n"
		if err := os.WriteFile(filepath.Join(projDir, id+".jsonl"), []byte(jsonl), 0o600); err != nil {
			t.Fatal(err)
		}
		histLine := `{"display":"hi","timestamp":1747300000000,"project":"/tmp/fake/proj","sessionId":"` + id + `"}` + "\n"
		if _, err := histFile.WriteString(histLine); err != nil {
			t.Fatal(err)
		}
	}
}

func makeUUID(i int) string {
	// Deterministic UUID-shaped string with unique suffix per i (zero-padded).
	return "fake0000-0000-0000-0000-" + padNum(i, 12)
}

func padNum(n, width int) string {
	s := ""
	if n == 0 {
		s = "0"
	}
	for n > 0 {
		s = string(rune('0'+(n%10))) + s
		n /= 10
	}
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func TestRunIndex_WithSessions(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	var buf strings.Builder
	if err := runIndexTo(&buf); err != nil {
		t.Fatalf("runIndexTo: %v", err)
	}
	if !strings.Contains(buf.String(), "Indexed 1 session(s)") {
		t.Errorf("expected 1 indexed session, got: %q", buf.String())
	}
}

// TestIndexSourceTo_ProgressBoundary covers the "indexed N sessions..."
// progress line by configuring a mock source that returns 50 sessions, each
// resolvable to the same on-disk seeded file. EnsureSession is fast against
// SQLite so this stays under the 1s test budget.
func TestIndexSourceTo_ProgressBoundary(t *testing.T) {
	withTempIndex(t)
	// Seed ONE real file; the mock source returns 50 unique session IDs all
	// pointing to that file. The cache hit on the second call keeps the
	// loop fast.
	seedFakeClaudeSessions(t, 1)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	// Reuse the same file path: List returns 50 sessions with the same ID,
	// each EnsureSession after the first is a cache hit (mtime+size unchanged).
	src := &fakeClaudeMultiSrc{ids: replicate(makeUUID(0), 50)}
	var buf strings.Builder
	n := indexSourceTo(&buf, src, idx, false, false)
	if n != 50 {
		t.Errorf("expected 50 indexed, got %d", n)
	}
	if !strings.Contains(buf.String(), "indexed 50 sessions...") {
		t.Errorf("expected progress at 50: %q", buf.String())
	}
}

func replicate(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// TestRunIndex_FullFlag covers the --full propagation.
func TestRunIndex_FullFlag(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	flagIndexFull = true
	var buf strings.Builder
	if err := runIndexTo(&buf); err != nil {
		t.Fatalf("runIndexTo: %v", err)
	}
}

// TestRunIndex_RebuildFlag covers the --rebuild propagation.
func TestRunIndex_RebuildFlag(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	resetFlags()
	resetIndexFlags()
	flagIndexAll = true
	flagIndexRebuild = true
	var buf strings.Builder
	if err := runIndexTo(&buf); err != nil {
		t.Fatalf("runIndexTo: %v", err)
	}
}

// TestIndexSourceTo_ListError covers the "warning: list ..." branch by
// passing a source whose List returns an error.
func TestIndexSourceTo_ListError(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var buf strings.Builder
	n := indexSourceTo(&buf, &errSource{}, idx, false, false)
	if n != 0 {
		t.Errorf("expected 0 indexed on list error, got %d", n)
	}
	if !strings.Contains(buf.String(), "warning: list") {
		t.Errorf("expected list warning, got: %q", buf.String())
	}
}

// TestIndexOneSession_GetErrorPath covers the Get-returns-error branch.
func TestIndexOneSession_GetErrorPath(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var buf strings.Builder
	if ok := indexOneSession(&buf, &getErrSource{}, idx, "anyid", false, false); ok {
		t.Errorf("expected false from Get-error")
	}
}

// TestIndexOneSession_GetNilPath covers the Get-returns-nil branch via the
// existing digestGetNilSrc mock.
func TestIndexOneSession_GetNilPath(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var buf strings.Builder
	if ok := indexOneSession(&buf, &digestGetNilSrc{}, idx, "anyid", false, false); ok {
		t.Errorf("expected false from nil session")
	}
}

// TestIndexOneSession_NoFilePath covers the empty-file-path branch.
// The mock Claude session ID has no on-disk file under jailed $HOME,
// so claude.SessionFilePath returns empty.
func TestIndexOneSession_NoFilePath(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var buf strings.Builder
	if ok := indexOneSession(&buf, &getSessionSource{}, idx, "no-such-id", false, false); ok {
		t.Errorf("expected false when SessionFilePath empty")
	}
}

// TestIndexOneSession_EnsureSessionError covers the EnsureSession-failure
// warning by closing the index db first, then calling indexOneSession with
// a session that does resolve to a real file.
func TestIndexOneSession_EnsureSessionError(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	_ = idx.Close()
	id := makeUUID(0)
	var buf strings.Builder
	if ok := indexOneSession(&buf, &fakeClaudeSrc{}, idx, id, false, false); ok {
		t.Errorf("expected false when EnsureSession errs")
	}
	if !strings.Contains(buf.String(), "warning: index") {
		t.Errorf("expected index warning, got: %q", buf.String())
	}
}

// ensure index.Open is referenced from this test file (silences unused-import
// warning when fakeClaudeSrc is the only consumer).
var _ = index.Open
