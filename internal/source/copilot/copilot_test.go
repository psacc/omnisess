package copilot

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
	s := &copilotSource{}
	if s.Name() != model.ToolCopilot {
		t.Errorf("Name() = %q, want %q", s.Name(), model.ToolCopilot)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func TestEventsPath(t *testing.T) {
	got := eventsPath("/foo/bar")
	want := filepath.Join("/foo/bar", "events.jsonl")
	if got != want {
		t.Errorf("eventsPath = %q, want %q", got, want)
	}
}

func TestSessionIDFromDir(t *testing.T) {
	got := sessionIDFromDir("/foo/bar/sess-001")
	if got != "sess-001" {
		t.Errorf("got %q, want sess-001", got)
	}
}

// ---------------------------------------------------------------------------
// List — error paths
// ---------------------------------------------------------------------------

func TestList_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	s := &copilotSource{}
	_, err := s.List(source.ListOptions{})
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestList_ListSessionDirsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".copilot", "session-state")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755) //nolint:errcheck

	s := &copilotSource{}
	_, err := s.List(source.ListOptions{})
	if err == nil {
		t.Fatal("expected error from listSessionDirs, got nil")
	}
}

// ---------------------------------------------------------------------------
// List — happy path + filters
// ---------------------------------------------------------------------------

func TestList_HappyPath(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Both sessions should carry tool=copilot.
	for _, sess := range sessions {
		if sess.Tool != model.ToolCopilot {
			t.Errorf("sess.Tool = %q", sess.Tool)
		}
	}

	// Find the fixture-with-metadata session.
	var withMeta *model.Session
	for i := range sessions {
		if sessions[i].ID == fixtureSessionID {
			withMeta = &sessions[i]
		}
	}
	if withMeta == nil {
		t.Fatalf("first fixture session not found in %v", sessions)
	}
	if withMeta.Project != "/Users/testuser/prj/myproject" {
		t.Errorf("project = %q", withMeta.Project)
	}
	if !strings.Contains(withMeta.Preview, "compare AGENTS.md") {
		t.Errorf("preview = %q", withMeta.Preview)
	}
	if withMeta.StartedAt.IsZero() {
		t.Error("StartedAt should be parsed from first event")
	}
	if withMeta.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set from file mtime")
	}
}

func TestList_StartedAtFallsBackToMtime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".copilot", "session-state", "no-ts-session")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// No parseable timestamps in any event → peekFirstTimestamp returns zero.
	content := `{"type":"user.message","timestamp":"","data":{"content":"hello"}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "events.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &copilotSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0].StartedAt.IsZero() {
		t.Error("StartedAt should fall back to UpdatedAt when no event has a timestamp")
	}
}

func TestList_ActiveFilterExcludesIdleSessions(t *testing.T) {
	home, eventsPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Backdate every events.jsonl far enough that no session is active.
	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(eventsPath, old, old); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(home, ".copilot", "session-state", fixtureSessionID2, "events.jsonl")
	if err := os.Chtimes(other, old, old); err != nil {
		t.Fatal(err)
	}

	s := &copilotSource{}
	sessions, err := s.List(source.ListOptions{Active: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 active sessions, got %d", len(sessions))
	}
}

func TestList_SinceFilter(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	// 1ns Since: every fixture session is older → all filtered out.
	sessions, err := s.List(source.ListOptions{Since: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 with tiny Since, got %d", len(sessions))
	}
}

// TestList_OnDateFilter is the regression guard for #61: --date must be
// honored by the copilot source the same way it is by claude/cursor/codex.
//
// Sub-tests:
//   - mismatching date (1999-01-01) → all sessions filtered out
//   - matching date (the fixture-copy day, i.e. "today") → fixtures returned
func TestList_OnDateFilter(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)
	s := &copilotSource{}

	// Sanity: with no date filter, both fixtures show up.
	all, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) == 0 {
		t.Fatalf("expected baseline fixtures, got 0")
	}

	t.Run("mismatching date excludes all sessions", func(t *testing.T) {
		now := time.Now()
		sessions, err := s.List(source.ListOptions{
			OnDate: time.Date(1999, 1, 1, 0, 0, 0, 0, now.Location()),
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("expected 0 with OnDate=1999-01-01, got %d", len(sessions))
		}
	})

	t.Run("matching date returns fixtures", func(t *testing.T) {
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		sessions, err := s.List(source.ListOptions{OnDate: today})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(sessions) == 0 {
			t.Errorf("expected fixtures with OnDate=today, got 0")
		}
	})
}

func TestList_ProjectFilter(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}

	matched, err := s.List(source.ListOptions{Project: "myproject"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 session matching project, got %d", len(matched))
	}

	none, err := s.List(source.ListOptions{Project: "nonexistent_project_xyz"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 with non-matching project, got %d", len(none))
	}
}

func TestList_ExcludeProjectsFilter(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	all, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	excluded, err := s.List(source.ListOptions{ExcludeProjects: []string{"myproject"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(excluded) >= len(all) {
		t.Errorf("ExcludeProjects had no effect: %d >= %d", len(excluded), len(all))
	}
}

func TestList_Limit(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sessions, err := s.List(source.ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestGet_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	s := &copilotSource{}
	_, err := s.Get("anything")
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestGet_ExactMatch(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sess, err := s.Get(fixtureSessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.ID != fixtureSessionID {
		t.Errorf("ID = %q", sess.ID)
	}
	if sess.Tool != model.ToolCopilot {
		t.Errorf("Tool = %q", sess.Tool)
	}
	if sess.Project != "/Users/testuser/prj/myproject" {
		t.Errorf("Project = %q", sess.Project)
	}
	if len(sess.Messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(sess.Messages))
	}
	if !strings.Contains(sess.Title, "compare AGENTS.md") {
		t.Errorf("Title = %q", sess.Title)
	}
}

func TestGet_PrefixMatch(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	// Prefix that is unique to one fixture.
	sess, err := s.Get("session-aabb")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.ID != fixtureSessionID {
		t.Errorf("got %q, want %q", sess.ID, fixtureSessionID)
	}
}

func TestGet_AmbiguousPrefix(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	// "session-" is a prefix of both fixtures → ambiguous.
	_, err := s.Get("session-")
	if err == nil {
		t.Fatal("expected ambiguous prefix error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error %q does not mention ambiguous", err.Error())
	}
}

func TestGet_NotFound(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sess, err := s.Get("does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil, got %+v", sess)
	}
}

func TestGet_ParseError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home, eventsPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	if err := os.Chmod(eventsPath, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(eventsPath, 0o644) //nolint:errcheck

	s := &copilotSource{}
	_, err := s.Get(fixtureSessionID)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestGet_FileMtimeRefinesUpdatedAt(t *testing.T) {
	home, eventsPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Bump mtime far into the future relative to embedded message ts.
	future := time.Now().Add(10 * 365 * 24 * time.Hour)
	if err := os.Chtimes(eventsPath, future, future); err != nil {
		t.Fatal(err)
	}

	s := &copilotSource{}
	sess, err := s.Get(fixtureSessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session")
	}
	if sess.UpdatedAt.Year() < future.Year() {
		t.Errorf("UpdatedAt should reflect file mtime, got %v", sess.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// resolveSessionDir — direct
// ---------------------------------------------------------------------------

func TestResolveSessionDir_GlobError(t *testing.T) {
	// Malformed bracket triggers filepath.Glob's syntax error.
	_, _, err := resolveSessionDir("/home/[invalid", "anything")
	if err == nil {
		t.Fatal("expected glob error, got nil")
	}
}

func TestResolveSessionDir_FiltersMatchedNonDirAndEmptyDir(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".copilot", "session-state")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// File matching the prefix — should be filtered out.
	if err := os.WriteFile(filepath.Join(root, "abc-file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Directory matching the prefix but missing events.jsonl — filtered out.
	if err := os.MkdirAll(filepath.Join(root, "abc-empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, fullID, err := resolveSessionDir(home, "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" || fullID != "" {
		t.Errorf("expected no match, got path=%q id=%q", path, fullID)
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	s := &copilotSource{}
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
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".copilot", "session-state")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755) //nolint:errcheck

	s := &copilotSource{}
	_, err := s.Search("query", source.ListOptions{})
	if err == nil {
		t.Fatal("expected error from List, got nil")
	}
}

func TestSearch_Hits(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	results, err := s.Search("AGENTS.md", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected hits, got 0")
	}
	if !strings.Contains(strings.ToLower(results[0].Matches[0].Snippet), "agents.md") {
		t.Errorf("snippet = %q", results[0].Matches[0].Snippet)
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	results, err := s.Search("AGENTS.MD", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected hits for uppercase query")
	}
}

func TestSearch_Miss(t *testing.T) {
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	results, err := s.Search("zzznomatchzzz", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_ParseEventsErrorSkips(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home, eventsPath := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Make one events.jsonl unreadable so parseEvents inside Search() fails
	// for that session — Search must continue and not propagate the error.
	if err := os.Chmod(eventsPath, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(eventsPath, 0o644) //nolint:errcheck

	s := &copilotSource{}
	_, err := s.Search("anything", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search should swallow per-session parse errors, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List/Get/Search across CLI + VS Code subsources
// ---------------------------------------------------------------------------

// setupCombinedHome layers a CLI session-state fixture and a VS Code workspace
// (chatSessions + state.vscdb) under the same HOME.
func setupCombinedHome(t *testing.T) string {
	t.Helper()
	forceVSCodeOSSupported(t)
	home, _ := setupFakeHome(t)
	storage := filepath.Join(home, darwinVSCodeWorkspaceStorageRel, "ws-1")
	if err := os.MkdirAll(filepath.Join(storage, "chatSessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(storage, "workspace.json"),
		[]byte(`{"folder":"file:///Users/foo/myapp"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	chatLine := `{"v":{"sessionId":"` + chatSessionID +
		`","creationDate":1700000000000,"lastMessageDate":1700000005000,` +
		`"requests":[{"message":{"text":"chatSessions hi"},"response":[{"kind":"markdownContent","content":{"value":"chatSessions reply about AGENTS.md"}}]}]}}`
	if err := os.WriteFile(
		filepath.Join(storage, "chatSessions", chatSessionID+".jsonl"),
		[]byte(chatLine+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	vscdbValue := `[{"sessionId":"` + vscdbSessionID +
		`","creationDate":1700000000000,"lastMessageDate":1700000010000,` +
		`"requests":[{"message":{"text":"vscdb prompt"},"response":[{"kind":"markdownContent","content":{"value":"vscdb reply about AGENTS.md"}}]}]}]`
	createVSCDB(t, filepath.Join(storage, "state.vscdb"), vscdbValue)
	return home
}

func TestList_AggregatesAllSubsources(t *testing.T) {
	home := setupCombinedHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// 2 CLI fixtures + 1 chatSessions + 1 vscdb = 4
	if len(sessions) != 4 {
		t.Fatalf("got %d sessions, want 4", len(sessions))
	}
	have := map[string]bool{}
	for _, sess := range sessions {
		have[sess.ID] = true
	}
	if !have[chatSessionID] {
		t.Error("chatSessions session not present")
	}
	if !have[vscdbSessionID] {
		t.Error("vscdb session not present")
	}
}

func TestList_VSCodeFiltersAreApplied(t *testing.T) {
	home := setupCombinedHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	// Project filter that no fixture matches → 0 sessions even though
	// CLI + VS Code fixtures are present.
	sessions, err := s.List(source.ListOptions{Project: "no-match-project-xyz"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestList_DiscoverVSCodeWorkspacesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	forceVSCodeOSSupported(t)
	home, _ := setupFakeHome(t)
	t.Setenv("HOME", home)

	// Make workspaceStorage unreadable so discoverVSCodeWorkspaces errors,
	// but session-state remains OK so List() still succeeds (warning only).
	parent := filepath.Join(home, "Library/Application Support/Code/User")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "workspaceStorage")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755) //nolint:errcheck

	s := &copilotSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List should swallow workspace-discovery errors, got: %v", err)
	}
	// Two CLI fixtures still come through.
	if len(sessions) != 2 {
		t.Errorf("got %d, want 2 (CLI fixtures only)", len(sessions))
	}
}

func TestGet_FallsThroughToChatSessions(t *testing.T) {
	home := setupCombinedHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sess, err := s.Get(chatSessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.ID != chatSessionID {
		t.Errorf("ID = %q", sess.ID)
	}
	if len(sess.Messages) == 0 {
		t.Error("expected messages")
	}
}

func TestGet_FallsThroughToVSCDB(t *testing.T) {
	home := setupCombinedHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	sess, err := s.Get(vscdbSessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session")
	}
	if sess.ID != vscdbSessionID {
		t.Errorf("ID = %q", sess.ID)
	}
}

func TestSearch_HitsAcrossSubsources(t *testing.T) {
	home := setupCombinedHome(t)
	t.Setenv("HOME", home)

	s := &copilotSource{}
	results, err := s.Search("AGENTS.md", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	srcIDs := map[string]bool{}
	for _, r := range results {
		srcIDs[r.Session.ID] = true
	}
	if !srcIDs[chatSessionID] {
		t.Error("expected chatSessions hit")
	}
	if !srcIDs[vscdbSessionID] {
		t.Error("expected vscdb hit")
	}
}

func TestLoadMessagesForSearch_MissReturnsNil(t *testing.T) {
	home := setupCombinedHome(t)
	t.Setenv("HOME", home)

	workspaces, _ := discoverVSCodeWorkspaces(home)
	got := loadMessagesForSearch("session-that-does-not-exist", sessionStateDir(home), workspaces)
	if got != nil {
		t.Errorf("expected nil for unresolvable id, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// extractSnippet — branches
// ---------------------------------------------------------------------------

func TestExtractSnippet_Short(t *testing.T) {
	got := extractSnippet("short", 0, 5, 200)
	if got != "short" {
		t.Errorf("got %q", got)
	}
}

func TestExtractSnippet_MatchAtStart(t *testing.T) {
	content := "MATCH" + strings.Repeat("b", 200)
	got := extractSnippet(content, 0, 5, 20)
	if !strings.Contains(got, "MATCH") || strings.HasPrefix(got, "...") {
		t.Errorf("got %q", got)
	}
}

func TestExtractSnippet_MatchAtEnd(t *testing.T) {
	content := strings.Repeat("a", 200) + "MATCH"
	got := extractSnippet(content, 200, 5, 20)
	if !strings.Contains(got, "MATCH") || strings.HasSuffix(got, "...") {
		t.Errorf("got %q", got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected leading ellipsis, got %q", got)
	}
}

func TestExtractSnippet_MatchInMiddle(t *testing.T) {
	content := strings.Repeat("a", 100) + "MATCH" + strings.Repeat("b", 100)
	got := extractSnippet(content, 100, 5, 20)
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...") {
		t.Errorf("got %q", got)
	}
}

func TestExtractSnippet_ShiftRight(t *testing.T) {
	content := "ab" + "MATCH" + strings.Repeat("z", 300)
	got := extractSnippet(content, 2, 5, 200)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("got %q", got)
	}
}

func TestExtractSnippet_ShiftLeft(t *testing.T) {
	content := strings.Repeat("a", 50) + "MATCH" + strings.Repeat("b", 3)
	got := extractSnippet(content, 50, 5, 40)
	if !strings.Contains(got, "MATCH") {
		t.Errorf("got %q", got)
	}
}
