package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// copyTree copies the fixture tree into dst, preserving no metadata.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture tree: %v", err)
	}
}

// setupSessions points HOME at a temp dir containing a copy of the fixture
// session tree and returns the sessions root.
func setupSessions(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")

	root := filepath.Join(home, ".pi", "agent", "sessions")
	copyTree(t, filepath.Join("testdata", "sessions"), root)
	return root
}

// touch sets a file's modification time.
func touch(t *testing.T, path string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatal(err)
	}
}

// stubRunner pins the process probe so tests don't depend on host processes.
func stubRunner(t *testing.T, running bool) {
	t.Helper()
	orig := isToolRunning
	isToolRunning = func(string) bool { return running }
	t.Cleanup(func() { isToolRunning = orig })
}

func sessionIDs(sessions []model.Session) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}

const (
	alphaID = "01900000-0000-7000-8000-000000000001"
	betaID  = "01900000-0000-7000-8000-000000000002"
	topID   = "01900000-0000-7000-8000-000000000003"
)

func fileFor(root, id string) string {
	matches, _ := filepath.Glob(filepath.Join(root, "*", "*"+id+".jsonl"))
	if len(matches) == 1 {
		return matches[0]
	}
	return filepath.Join(root, "top-level.jsonl")
}

// ---------------------------------------------------------------------------
// Name / paths / discovery
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	s := &piSource{}
	if s.Name() != model.ToolPi {
		t.Errorf("Name() = %q, want %q", s.Name(), model.ToolPi)
	}
}

func TestAgentDir(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got, want := agentDir("/home/foo"), "/home/foo/.pi/agent"; got != want {
		t.Errorf("agentDir() = %q, want %q", got, want)
	}

	t.Setenv("PI_CODING_AGENT_DIR", "/custom/agent")
	if got, want := agentDir("/home/foo"), "/custom/agent"; got != want {
		t.Errorf("agentDir() with env = %q, want %q", got, want)
	}
}

func TestSessionsRoot(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", "")
	want := filepath.Join("/home/foo", ".pi", "agent", "sessions")
	if got := sessionsRoot("/home/foo"); got != want {
		t.Errorf("sessionsRoot() = %q, want %q", got, want)
	}
}

func TestDiscoverSessionFiles(t *testing.T) {
	root := setupSessions(t)

	files, err := discoverSessionFiles(root)
	if err != nil {
		t.Fatalf("discoverSessionFiles: %v", err)
	}
	// 5 non-session jsonl files sit next to the fixture sessions and must
	// still be returned here — List filters them by content.
	if len(files) != 6 {
		t.Errorf("got %d files, want 6: %v", len(files), files)
	}
	for _, f := range files {
		if !strings.HasSuffix(f, ".jsonl") {
			t.Errorf("non-jsonl file returned: %s", f)
		}
	}
}

func TestDiscoverSessionFiles_MissingRoot(t *testing.T) {
	files, err := discoverSessionFiles(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("discoverSessionFiles: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil files, got %v", files)
	}
}

func TestDiscoverSessionFiles_ReadDirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	root := setupSessions(t)
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := discoverSessionFiles(root); err == nil {
		t.Fatal("expected permission error, got nil")
	}
}

func TestDiscoverSessionFiles_SkipsUnreadableSubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	root := setupSessions(t)
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	files, err := discoverSessionFiles(root)
	if err != nil {
		t.Fatalf("discoverSessionFiles: %v", err)
	}
	if len(files) != 6 {
		t.Errorf("got %d files, want 6", len(files))
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if _, err := (&piSource{}).List(source.ListOptions{}); err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestList_DiscoverError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	root := setupSessions(t)
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := (&piSource{}).List(source.ListOptions{}); err == nil {
		t.Fatal("expected discover error, got nil")
	}
}

func TestList_HappyPath(t *testing.T) {
	root := setupSessions(t)
	stubRunner(t, false)

	alpha := fileFor(root, alphaID)
	beta := fileFor(root, betaID)
	top := filepath.Join(root, "top-level.jsonl")
	touch(t, alpha, time.Date(2026, 1, 5, 23, 0, 0, 0, time.UTC))
	touch(t, beta, time.Date(2026, 1, 4, 23, 0, 0, 0, time.UTC))
	touch(t, top, time.Date(2026, 1, 3, 23, 0, 0, 0, time.UTC))

	sessions, err := (&piSource{}).List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := sessionIDs(sessions), []string{alphaID, betaID, topID}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}

	a := sessions[0]
	if a.Tool != model.ToolPi || a.Project != "/Users/foo/prj/demo" {
		t.Errorf("session = %+v", a)
	}
	if a.Title != "Parser work" {
		t.Errorf("Title = %q, want session name", a.Title)
	}
	if a.Model != "anthropic/claude-sonnet-5-5" {
		t.Errorf("Model = %q", a.Model)
	}
	if want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC); !a.StartedAt.Equal(want) {
		t.Errorf("StartedAt = %v, want %v", a.StartedAt, want)
	}

	b := sessions[1]
	if b.Title != "Other project question" || b.Project != "/Users/foo/prj/other" {
		t.Errorf("beta = %+v", b)
	}
	if b.Model != "bare-model" {
		t.Errorf("beta Model = %q, want bare-model", b.Model)
	}
	if a.Active || b.Active {
		t.Error("Active = true, want false when the tool is not running")
	}
}

func TestList_Active(t *testing.T) {
	root := setupSessions(t)
	origRunner, origNow := isToolRunning, nowFn
	t.Cleanup(func() { isToolRunning, nowFn = origRunner, origNow })
	isToolRunning = func(string) bool { return true }

	now := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return now }
	touch(t, fileFor(root, alphaID), now.Add(-time.Minute))
	touch(t, fileFor(root, betaID), now.Add(-time.Hour))
	touch(t, filepath.Join(root, "top-level.jsonl"), now.Add(-time.Hour))

	sessions, err := (&piSource{}).List(source.ListOptions{Active: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := sessionIDs(sessions), []string{alphaID}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("active = %v, want %v", got, want)
	}
}

func TestList_Filters(t *testing.T) {
	root := setupSessions(t)
	stubRunner(t, false)

	alpha := fileFor(root, alphaID)
	beta := fileFor(root, betaID)
	top := filepath.Join(root, "top-level.jsonl")
	now := time.Date(2026, 1, 6, 0, 30, 0, 0, time.UTC)
	nowFn = func() time.Time { return now }
	t.Cleanup(func() { nowFn = time.Now })

	tests := []struct {
		name string
		opts source.ListOptions
		want []string
	}{
		{"since 2h", source.ListOptions{Since: 2 * time.Hour}, []string{alphaID}},
		{"date jan 5", source.ListOptions{OnDate: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)}, []string{alphaID}},
		{"date jan 4", source.ListOptions{OnDate: time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)}, []string{betaID}},
		{"project", source.ListOptions{Project: "other"}, []string{betaID}},
		{"exclude project", source.ListOptions{ExcludeProjects: []string{"other"}}, []string{alphaID, topID}},
		{"limit", source.ListOptions{Limit: 2}, []string{alphaID, betaID}},
		{"active but not running", source.ListOptions{Active: true}, nil},
	}
	mtimes := map[string]time.Time{
		alpha: time.Date(2026, 1, 5, 23, 0, 0, 0, time.UTC),
		beta:  time.Date(2026, 1, 4, 23, 0, 0, 0, time.UTC),
		top:   time.Date(2026, 1, 3, 23, 0, 0, 0, time.UTC),
	}
	for path, ts := range mtimes {
		touch(t, path, ts)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions, err := (&piSource{}).List(tt.opts)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if got := sessionIDs(sessions); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("sessions = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestGet_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if _, err := (&piSource{}).Get(alphaID); err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestGet_DiscoverError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	root := setupSessions(t)
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := (&piSource{}).Get(alphaID); err == nil {
		t.Fatal("expected discover error, got nil")
	}
}

func TestGet_HappyPath(t *testing.T) {
	root := setupSessions(t)
	stubRunner(t, false)
	touch(t, fileFor(root, alphaID), time.Date(2026, 1, 5, 23, 0, 0, 0, time.UTC))

	sess, err := (&piSource{}).Get(alphaID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Title != "Parser work" || sess.Model != "anthropic/claude-sonnet-5-5" {
		t.Errorf("session = %+v", sess)
	}
	if len(sess.Messages) != 5 {
		t.Fatalf("got %d messages, want 5: %+v", len(sess.Messages), sess.Messages)
	}

	if m := sess.Messages[0]; m.Role != model.RoleUser || m.Content != "Fix the parser" {
		t.Errorf("msg0 = %+v", m)
	}
	assistant := sess.Messages[1]
	if assistant.Role != model.RoleAssistant || assistant.Content != "Reading the parser." {
		t.Errorf("msg1 = %+v", assistant)
	}
	if len(assistant.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].Output != "package parser" || assistant.ToolCalls[0].Name != "read" {
		t.Errorf("read call = %+v", assistant.ToolCalls[0])
	}
	if !assistant.ToolCalls[1].IsError || assistant.ToolCalls[1].Output != "FAIL" {
		t.Errorf("bash call = %+v", assistant.ToolCalls[1])
	}
	if assistant.UsageInputTokens != 10 || assistant.UsageOutputTokens != 20 ||
		assistant.UsageCacheReadInputTokens != 3 || assistant.UsageCacheCreationInputTokens != 4 {
		t.Errorf("usage = %+v", assistant)
	}
	if assistant.ToolCalls[0].Input != `{"path":"/Users/foo/prj/demo/parser.go"}` {
		t.Errorf("input = %q", assistant.ToolCalls[0].Input)
	}

	if m := sess.Messages[2]; m.Role != model.RoleTool || m.Content != "$ git status\nclean" {
		t.Errorf("bash execution = %+v", m)
	}
	if m := sess.Messages[3]; m.Content != "Parser fixed." {
		t.Errorf("msg3 = %+v", m)
	}
	if m := sess.Messages[4]; m.Content != "plain string prompt" {
		t.Errorf("msg4 = %+v", m)
	}

	// The file mtime is later than the last message, so it wins.
	if want := time.Date(2026, 1, 5, 23, 0, 0, 0, time.UTC); !sess.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", sess.UpdatedAt, want)
	}
}

func TestGet_Exact(t *testing.T) {
	setupSessions(t)
	sess, err := (&piSource{}).Get(alphaID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil || sess.ID != alphaID {
		t.Fatalf("session = %+v", sess)
	}
}

func TestGet_UniquePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")

	root := filepath.Join(home, ".pi", "agent", "sessions", "--Users-foo-prj--")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session","id":"custom-session-id","timestamp":"2026-01-02T03:04:05.000Z","cwd":"/Users/foo/prj"}
{"type":"message","id":"m0","timestamp":"2026-01-02T03:04:08.000Z","message":{"role":"user","content":"hi","timestamp":1767323048000}}
`
	if err := os.WriteFile(filepath.Join(root, "s.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	sess, err := (&piSource{}).Get("custom-session")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil || sess.ID != "custom-session-id" {
		t.Fatalf("prefix Get = %+v", sess)
	}
}

func TestGet_AmbiguousPrefix(t *testing.T) {
	setupSessions(t)
	_, err := (&piSource{}).Get("01900000-0000-7000")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguous", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	setupSessions(t)
	sess, err := (&piSource{}).Get("01900000-0000-7000-8000-00000000ffff")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil session, got %+v", sess)
	}
}

func TestGet_ParseError(t *testing.T) {
	root := setupSessions(t)
	path := fileFor(root, alphaID)

	// A valid short header followed by an over-long line: header resolution
	// succeeds, the full parse fails.
	header := fmt.Sprintf(`{"type":"session","id":%q,"timestamp":"2026-01-02T03:04:05.000Z","cwd":"/Users/foo/prj/demo"}`+"\n", alphaID)
	body := strings.Repeat("x", 300) + "\n"
	if err := os.WriteFile(path, []byte(header+body), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := maxLineBytes
	maxLineBytes = 200
	t.Cleanup(func() { maxLineBytes = orig })

	if _, err := (&piSource{}).Get(alphaID); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestGet_Active(t *testing.T) {
	root := setupSessions(t)
	origRunner, origNow := isToolRunning, nowFn
	t.Cleanup(func() { isToolRunning, nowFn = origRunner, origNow })
	isToolRunning = func(string) bool { return true }
	now := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return now }
	touch(t, fileFor(root, alphaID), now.Add(-time.Minute))

	sess, err := (&piSource{}).Get(alphaID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !sess.Active {
		t.Error("Active = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if _, err := (&piSource{}).Search("x", source.ListOptions{}); err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestSearch_DiscoverError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	root := setupSessions(t)
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := (&piSource{}).Search("x", source.ListOptions{}); err == nil {
		t.Fatal("expected discover error, got nil")
	}
}

func TestSearch_HappyPath(t *testing.T) {
	setupSessions(t)
	results, err := (&piSource{}).Search("parser", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Session.ID != alphaID {
		t.Fatalf("results = %+v", results)
	}
	if len(results[0].Matches) < 2 {
		t.Errorf("got %d matches, want at least 2", len(results[0].Matches))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	setupSessions(t)
	results, err := (&piSource{}).Search("definitely-absent", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSearch_SkipsUnparseableSession(t *testing.T) {
	root := setupSessions(t)
	// A valid header, fewer than peekMaxLines readable entries, then one
	// over-long line: List's bounded peek succeeds, the full parse fails.
	var b strings.Builder
	fmt.Fprintf(&b, `{"type":"session","id":%q,"timestamp":"2026-01-02T03:04:05.000Z","cwd":"/Users/foo/prj/demo"}`+"\n", alphaID)
	b.WriteString(`{"type":"message","id":"m0","timestamp":"2026-01-02T03:04:08.000Z","message":{"role":"user","content":"find me","timestamp":1767323048000}}` + "\n")
	for i := 0; i < peekMaxLines-1; i++ {
		b.WriteString(`{"type":"label","id":"e","parentId":null,"timestamp":"2026-01-02T03:04:09.000Z"}` + "\n")
	}
	b.WriteString(strings.Repeat("x", 300) + "\n")
	if err := os.WriteFile(fileFor(root, alphaID), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := maxLineBytes
	maxLineBytes = 200
	t.Cleanup(func() { maxLineBytes = orig })

	got, err := (&piSource{}).Search("find me", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0 (unparseable session skipped)", len(got))
	}
}

func TestList_StatErrorSkipped(t *testing.T) {
	root := setupSessions(t)
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, "dangling.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	stubRunner(t, false)

	sessions, err := (&piSource{}).List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("got %d sessions, want 3 (dangling symlink skipped)", len(sessions))
	}
}

func TestList_NoHeaderTimestamp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	stubRunner(t, false)

	dir := filepath.Join(home, ".pi", "agent", "sessions", "--Users-foo-prj--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session","id":"no-ts","cwd":"/Users/foo/prj"}
{"type":"message","id":"m0","timestamp":"2026-01-02T03:04:08.000Z","message":{"role":"user","content":"hi","timestamp":1767323048000}}
`
	path := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	touch(t, path, mtime)

	sessions, err := (&piSource{}).List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if !sessions[0].StartedAt.Equal(mtime) {
		t.Errorf("StartedAt = %v, want mtime fallback %v", sessions[0].StartedAt, mtime)
	}
}
