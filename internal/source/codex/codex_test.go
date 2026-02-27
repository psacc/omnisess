package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

// ---------------------------------------------------------------------------
// Name
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	s := &codexSource{}
	if s.Name() != model.ToolCodex {
		t.Errorf("Name() = %q, want %q", s.Name(), model.ToolCodex)
	}
}

// ---------------------------------------------------------------------------
// codexDir / historyFilePath — error paths with HOME=""
// ---------------------------------------------------------------------------

func TestCodexDir_Success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := codexDir()
	if err != nil {
		t.Fatalf("codexDir() error: %v", err)
	}
	want := filepath.Join(home, ".codex")
	if dir != want {
		t.Errorf("codexDir() = %q, want %q", dir, want)
	}
}

func TestCodexDir_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := codexDir()
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestHistoryFilePath_Success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := historyFilePath()
	if err != nil {
		t.Fatalf("historyFilePath() error: %v", err)
	}
	want := filepath.Join(home, ".codex", "history.jsonl")
	if p != want {
		t.Errorf("historyFilePath() = %q, want %q", p, want)
	}
}

func TestHistoryFilePath_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := historyFilePath()
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

// ---------------------------------------------------------------------------
// loadHistory — error paths
// ---------------------------------------------------------------------------

func TestLoadHistory_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := loadHistory()
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestLoadHistory_NoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No history.jsonl file
	entries, err := loadHistory()
	if err != nil {
		t.Fatalf("loadHistory() unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for missing file, got %v", entries)
	}
}

func TestLoadHistory_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	histPath := filepath.Join(codexDir, "history.jsonl")
	if err := os.WriteFile(histPath, []byte(`{"session_id":"x","ts":1}`+"\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(histPath, 0o644) //nolint:errcheck

	_, err := loadHistory()
	if err == nil {
		t.Fatal("expected error for unreadable history file, got nil")
	}
}

func TestLoadHistory_EmptyLines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "\n" +
		`{"session_id":"aabbccdd-1234-5678-9abc-000000000000","ts":1739091671,"text":"hello"}` + "\n" +
		"\n"
	if err := os.WriteFile(filepath.Join(codexDir, "history.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := loadHistory()
	if err != nil {
		t.Fatalf("loadHistory() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (empty lines skipped), got %d", len(entries))
	}
}

func TestLoadHistory_MalformedAndEmptyID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `{bad json}` + "\n" +
		`{"session_id":"","ts":1000,"text":"empty id"}` + "\n" +
		`{"session_id":"valid-session-id","ts":1739091671,"text":"good"}` + "\n"
	if err := os.WriteFile(filepath.Join(codexDir, "history.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := loadHistory()
	if err != nil {
		t.Fatalf("loadHistory() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (malformed + empty ID skipped), got %d", len(entries))
	}
}

func TestLoadHistory_Deduplication(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Same session ID, second entry is earlier timestamp
	content := `{"session_id":"dup-session-aaa","ts":1739095000,"text":"later entry"}` + "\n" +
		`{"session_id":"dup-session-aaa","ts":1739091000,"text":"earlier entry"}` + "\n"
	if err := os.WriteFile(filepath.Join(codexDir, "history.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := loadHistory()
	if err != nil {
		t.Fatalf("loadHistory() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 deduplicated entry, got %d", len(entries))
	}
	// text should be from earliest entry
	if entries[0].text != "earlier entry" {
		t.Errorf("text = %q, want 'earlier entry' from earliest", entries[0].text)
	}
	// latest should be from latest timestamp
	wantLatest := time.Unix(1739095000, 0)
	if !entries[0].latest.Equal(wantLatest) {
		t.Errorf("latest = %v, want %v", entries[0].latest, wantLatest)
	}
}

func TestLoadHistory_ScannerError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(codexDir, "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"session_id":"scan-sess","ts":1000,"text":"ok"}` + "\n")
	// Line exceeding 1MB buffer
	bigLine := make([]byte, 2*1024*1024)
	for i := range bigLine {
		bigLine[i] = 'x'
	}
	_, _ = f.Write(bigLine)
	_, _ = f.WriteString("\n")
	f.Close()

	// Should not return error itself (logs warning)
	entries, err := loadHistory()
	if err != nil {
		t.Fatalf("loadHistory() should not return error for scanner warning: %v", err)
	}
	_ = entries
}

// ---------------------------------------------------------------------------
// List — error paths
// ---------------------------------------------------------------------------

func TestList_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	s := &codexSource{}
	_, err := s.List(source.ListOptions{})
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestList_LoadHistoryError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	histPath := filepath.Join(codexDir, "history.jsonl")
	if err := os.WriteFile(histPath, []byte(`{}`+"\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(histPath, 0o644) //nolint:errcheck

	s := &codexSource{}
	_, err := s.List(source.ListOptions{})
	if err == nil {
		t.Fatal("expected error for unreadable history, got nil")
	}
}

func TestList_ActiveFilter(t *testing.T) {
	home, sessPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Set old mtime so session is never detected as active
	oldTime := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(sessPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	s := &codexSource{}
	sessions, err := s.List(source.ListOptions{Active: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	// Old file should be filtered out
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions with Active=true (old mtime), got %d", len(sessions))
	}
}

func TestList_ProjectFilter(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &codexSource{}
	sessions, err := s.List(source.ListOptions{Project: "nonexistent_project_xyz"})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for non-matching project, got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// Get — error paths
// ---------------------------------------------------------------------------

func TestGet_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	s := &codexSource{}
	_, err := s.Get("someid")
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestGet_ResolveError(t *testing.T) {
	// Ambiguous prefix: create two session files with same prefix
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "02", "09")
	// Create a second session file with the same prefix as fixtureSessionID
	sessData, err := os.ReadFile(fixtureSessionFile)
	if err != nil {
		t.Fatal(err)
	}
	secondID := "aabbccdd-9999-5678-9abc-def012345678"
	secondPath := filepath.Join(sessionsDir, "rollout-20260209T100200-"+secondID+".jsonl")
	if err := os.WriteFile(secondPath, sessData, 0o644); err != nil {
		t.Fatal(err)
	}

	s := &codexSource{}
	// Prefix "aabbccdd" matches both fixtureSessionID and secondID
	_, err = s.Get("aabbccdd")
	if err == nil {
		t.Fatal("expected ambiguous prefix error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error %q doesn't mention ambiguous", err.Error())
	}
}

func TestGet_ParseError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home, sessPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Make session file unreadable
	if err := os.Chmod(sessPath, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(sessPath, 0o644) //nolint:errcheck

	s := &codexSource{}
	_, err := s.Get(fixtureSessionID)
	if err == nil {
		t.Fatal("expected error for unreadable session file, got nil")
	}
}

func TestGet_NoMessages(t *testing.T) {
	// Session file with only session_meta, no messages
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "02", "09")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	// history.jsonl with this session
	histContent := `{"session_id":"nomsg00-1234-5678-9abc-def012345678","ts":1739091671,"text":"empty"}` + "\n"
	if err := os.WriteFile(filepath.Join(home, ".codex", "history.jsonl"), []byte(histContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// Session file with only session_meta
	sessContent := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"id":"nomsg00-1234-5678-9abc-def012345678","cwd":"/tmp/nomsg"}}` + "\n"
	sessPath := filepath.Join(sessionsDir, "rollout-20260209T100111-nomsg00-1234-5678-9abc-def012345678.jsonl")
	if err := os.WriteFile(sessPath, []byte(sessContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &codexSource{}
	sess, err := s.Get("nomsg00-1234-5678-9abc-def012345678")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	// No messages → startedAt is zero, updatedAt refined from file
	if sess.StartedAt != (time.Time{}) {
		t.Errorf("StartedAt should be zero with no messages, got %v", sess.StartedAt)
	}
	if sess.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be refined from file modtime")
	}
}

// ---------------------------------------------------------------------------
// Search — error paths
// ---------------------------------------------------------------------------

func TestSearch_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	s := &codexSource{}
	_, err := s.Search("query", source.ListOptions{})
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestSearch_ListError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	histPath := filepath.Join(codexDir, "history.jsonl")
	if err := os.WriteFile(histPath, []byte(`{}`+"\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(histPath, 0o644) //nolint:errcheck

	s := &codexSource{}
	_, err := s.Search("query", source.ListOptions{})
	if err == nil {
		t.Fatal("expected error for unreadable history, got nil")
	}
}

func TestSearch_ProjectFilter(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &codexSource{}
	// Filter by a project path present in the session's cwd
	results, err := s.Search("compare", source.ListOptions{Project: "/Users/testuser"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	// Should find results in sessions with matching cwd
	if len(results) == 0 {
		t.Error("expected results for project filter matching cwd")
	}
}

func TestSearch_ProjectFilterNoMatch(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &codexSource{}
	results, err := s.Search("compare", source.ListOptions{Project: "nonexistent_project_xyz"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching project, got %d", len(results))
	}
}

func TestSearch_ParseError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home, sessPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Make session file unreadable to trigger parseSessionFile error in Search
	if err := os.Chmod(sessPath, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(sessPath, 0o644) //nolint:errcheck

	s := &codexSource{}
	// Search should skip the unreadable session gracefully
	results, err := s.Search("compare", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	_ = results
}

// ---------------------------------------------------------------------------
// resolveCodexSessionFile — glob error and ambiguous prefix
// ---------------------------------------------------------------------------

func TestResolveCodexSessionFile_GlobError(t *testing.T) {
	// homeDir with unclosed bracket causes filepath.Glob to return syntax error
	_, _, err := resolveCodexSessionFile("/home/[invalidbracket", "someid")
	if err == nil {
		t.Fatal("expected glob error for malformed homeDir path, got nil")
	}
}

func TestResolveCodexSessionFile_AmbiguousPrefix(t *testing.T) {
	home, _ := setupFakeHome(t)

	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "02", "09")
	sessData, err := os.ReadFile(fixtureSessionFile)
	if err != nil {
		t.Fatal(err)
	}
	// Create a second session file sharing the same 8-char prefix "aabbccdd"
	secondID := "aabbccdd-ffff-5678-9abc-def012345678"
	secondPath := filepath.Join(sessionsDir, "rollout-20260209T100300-"+secondID+".jsonl")
	if err := os.WriteFile(secondPath, sessData, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err = resolveCodexSessionFile(home, "aabbccdd")
	if err == nil {
		t.Fatal("expected ambiguous error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error %q doesn't mention ambiguous", err.Error())
	}
}

// ---------------------------------------------------------------------------
// extractSnippet — all branches
// ---------------------------------------------------------------------------

func TestExtractSnippet_ShortContent(t *testing.T) {
	content := "short"
	got := extractSnippet(content, 0, 5, 200)
	if got != content {
		t.Errorf("short content should return as-is, got %q", got)
	}
}

func TestExtractSnippet_MatchAtStart(t *testing.T) {
	content := "MATCH" + strings.Repeat("b", 200)
	got := extractSnippet(content, 0, 5, 20)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("snippet %q should contain MATCH", got)
	}
	// start=0 so no leading ellipsis
	if strings.HasPrefix(got, "...") {
		t.Errorf("expected no leading ellipsis for start=0, got %q", got)
	}
}

func TestExtractSnippet_MatchAtEnd(t *testing.T) {
	content := strings.Repeat("a", 200) + "MATCH"
	got := extractSnippet(content, 200, 5, 20)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("snippet %q should contain MATCH", got)
	}
	// end==len → no trailing ellipsis
	if strings.HasSuffix(got, "...") {
		t.Errorf("expected no trailing ellipsis when end==len, got %q", got)
	}
	// start>0 → leading ellipsis
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected leading ellipsis, got %q", got)
	}
}

func TestExtractSnippet_MatchInMiddle(t *testing.T) {
	content := strings.Repeat("a", 100) + "MATCH" + strings.Repeat("b", 100)
	got := extractSnippet(content, 100, 5, 20)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("snippet %q should contain MATCH", got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected leading ellipsis for middle match, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected trailing ellipsis for middle match, got %q", got)
	}
}

func TestExtractSnippet_ShiftRight(t *testing.T) {
	// Match near start so start < 0 → shift right
	content := "ab" + "MATCH" + strings.Repeat("z", 300)
	got := extractSnippet(content, 2, 5, 200)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("snippet %q should contain MATCH", got)
	}
}

func TestExtractSnippet_ShiftLeft(t *testing.T) {
	// Match near end so end > len → shift left
	content := strings.Repeat("a", 50) + "MATCH" + strings.Repeat("b", 3)
	got := extractSnippet(content, 50, 5, 40)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("snippet %q should contain MATCH", got)
	}
}

func TestExtractSnippet_EllipsisMarkers(t *testing.T) {
	content := strings.Repeat("a", 25) + "MATCH" + strings.Repeat("b", 25)
	got := extractSnippet(content, 25, 5, 20)
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected leading '...', got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected trailing '...', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractSessionIDFromPath — stem shorter than 36 chars (fallback path)
// ---------------------------------------------------------------------------

func TestExtractSessionIDFromPath_ShortStem(t *testing.T) {
	// Stem shorter than 36 chars → returns stem as-is
	path := "/some/path/short-id.jsonl"
	got := extractSessionIDFromPath(path)
	if got != "short-id" {
		t.Errorf("extractSessionIDFromPath(%q) = %q, want short-id", path, got)
	}
}

func TestExtractSessionIDFromPath_InvalidUUIDFormat(t *testing.T) {
	// 36 chars but doesn't have dashes at UUID positions → fallback to stem
	path := "/some/path/abcdefghijklmnopqrstuvwxyz1234.jsonl"
	// stem = "abcdefghijklmnopqrstuvwxyz1234" (30 chars) → < 36 → returns stem
	got := extractSessionIDFromPath(path)
	if got != "abcdefghijklmnopqrstuvwxyz1234" {
		t.Errorf("extractSessionIDFromPath short = %q, want abcdefghijklmnopqrstuvwxyz1234", got)
	}
}

func TestExtractSessionIDFromPath_36CharsNotUUID(t *testing.T) {
	// Exactly 36 chars but wrong dash positions
	path := "/path/aaaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbb.jsonl"
	// stem length = 36, but no dashes at right positions → fallback to stem
	got := extractSessionIDFromPath(path)
	if len(got) == 0 {
		t.Error("expected non-empty result")
	}
}

// ---------------------------------------------------------------------------
// readSessionCwd — edge cases
// ---------------------------------------------------------------------------

func TestReadSessionCwd_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readSessionCwd(path)
	if got != "" {
		t.Errorf("expected empty for empty file, got %q", got)
	}
}

func TestReadSessionCwd_MalformedFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed.jsonl")
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readSessionCwd(path)
	if got != "" {
		t.Errorf("expected empty for malformed JSON, got %q", got)
	}
}

func TestReadSessionCwd_WrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrong_type.jsonl")
	// First line is not session_meta
	content := `{"timestamp":"2026-02-09T10:01:12.000Z","type":"response_item","payload":{"type":"message"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readSessionCwd(path)
	if got != "" {
		t.Errorf("expected empty for non-session_meta type, got %q", got)
	}
}

func TestReadSessionCwd_MalformedPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_payload.jsonl")
	// First line is session_meta but payload is a string not object
	content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":"not an object"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readSessionCwd(path)
	if got != "" {
		t.Errorf("expected empty for malformed payload, got %q", got)
	}
}

func TestReadSessionCwd_NonExistent(t *testing.T) {
	got := readSessionCwd("/nonexistent/path/file.jsonl")
	if got != "" {
		t.Errorf("expected empty for non-existent file, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// parseSessionFile — edge cases
// ---------------------------------------------------------------------------

func TestParseSessionFile_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_lines.jsonl")
	content := "\n" +
		`{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"id":"x","cwd":"/tmp/test"}}` + "\n" +
		"\n" +
		`{"timestamp":"2026-02-09T10:01:12.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, cwd, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwd != "/tmp/test" {
		t.Errorf("cwd = %q, want /tmp/test", cwd)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}

func TestParseSessionFile_ResponseItemUnmarshalError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_response.jsonl")
	// response_item with payload that fails to unmarshal into responseItemPayload
	content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"cwd":"/tmp"}}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:12.000Z","type":"response_item","payload":"not an object"}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:13.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"good"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (bad payload skipped), got %d", len(msgs))
	}
}

func TestParseSessionFile_NonMessageResponseItem(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "non_message.jsonl")
	// response_item with type != "message"
	content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"cwd":"/tmp"}}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:12.000Z","type":"response_item","payload":{"type":"function_call","role":"developer","content":[]}}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:13.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"good"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (non-message type skipped), got %d", len(msgs))
	}
}

func TestParseSessionFile_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown_role.jsonl")
	// response_item with unknown role → mapResponseItemRole returns "" → skipped
	content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"cwd":"/tmp"}}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:12.000Z","type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"input_text","text":"system msg"}]}}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:13.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"good"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (unknown role skipped), got %d", len(msgs))
	}
}

func TestParseSessionFile_ScanError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bigline.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"cwd":"/tmp"}}` + "\n")
	bigContent := make([]byte, 11*1024*1024)
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	_, _ = f.Write(bigContent)
	_, _ = f.WriteString("\n")
	f.Close()

	_, _, err = parseSessionFile(path)
	if err == nil {
		t.Fatal("expected error for oversized line, got nil")
	}
}

func TestParseSessionFile_CwdAlreadySet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "double_meta.jsonl")
	// Two session_meta lines — second cwd should be ignored
	content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"cwd":"/first/cwd"}}` + "\n" +
		`{"timestamp":"2026-02-09T10:01:12.000Z","type":"session_meta","payload":{"cwd":"/second/cwd"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cwd, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwd != "/first/cwd" {
		t.Errorf("cwd = %q, want /first/cwd (first wins)", cwd)
	}
}

// ---------------------------------------------------------------------------
// mapResponseItemRole — default/unknown role
// ---------------------------------------------------------------------------

func TestMapResponseItemRole(t *testing.T) {
	tests := []struct {
		role string
		want model.Role
	}{
		{"developer", model.RoleUser},
		{"assistant", model.RoleAssistant},
		{"system", ""},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := mapResponseItemRole(tt.role)
		if got != tt.want {
			t.Errorf("mapResponseItemRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseCodexTimestamp — all branches
// ---------------------------------------------------------------------------

func TestParseCodexTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		isZero bool
		year   int
	}{
		{
			name:   "RFC3339Nano",
			input:  "2026-02-09T10:01:11.966Z",
			isZero: false,
			year:   2026,
		},
		{
			name:   "RFC3339",
			input:  "2026-02-09T10:01:11Z",
			isZero: false,
			year:   2026,
		},
		{
			name:   "millis format",
			input:  "2026-02-09T10:01:11.000Z",
			isZero: false,
			year:   2026,
		},
		{
			name:   "empty string",
			input:  "",
			isZero: true,
		},
		{
			name:   "garbage string",
			input:  "not-a-timestamp",
			isZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCodexTimestamp(tt.input)
			if tt.isZero {
				if !got.IsZero() {
					t.Errorf("parseCodexTimestamp(%q) = %v, want zero", tt.input, got)
				}
				return
			}
			if got.IsZero() {
				t.Fatalf("parseCodexTimestamp(%q) returned zero, want non-zero", tt.input)
			}
			if got.Year() != tt.year {
				t.Errorf("parseCodexTimestamp(%q).Year() = %d, want %d", tt.input, got.Year(), tt.year)
			}
		})
	}
}
