package cmd

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/psacc/omnisess/internal/source"
)

// ---------------------------------------------------------------------------
// Mock sources for controlled testing
// ---------------------------------------------------------------------------

// errSource is a source.Source that always returns an error from List and Search,
// and nil from Get. It is registered once at package-init time and addressed
// only by setting flagTool = errSourceName in individual tests.
const errSourceName = model.Tool("test-error-src")

type errSource struct{}

func (e *errSource) Name() model.Tool { return errSourceName }

func (e *errSource) List(_ source.ListOptions) ([]model.Session, error) {
	return nil, errors.New("mock list error")
}

func (e *errSource) Get(_ string) (*model.Session, error) {
	return nil, nil
}

func (e *errSource) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, errors.New("mock search error")
}

// activeSource is a source.Source that always returns two active sessions from
// List() and two SearchResults from Search(). Returning two guarantees that the
// limit-truncation branch (all = all[:limit]) is exercised when limit=1, even
// when no real local data is present (e.g., in CI with -short / cover-check).
const activeSourceName = model.Tool("test-active-src")

type activeSource struct{}

func (a *activeSource) Name() model.Tool { return activeSourceName }

func (a *activeSource) List(opts source.ListOptions) ([]model.Session, error) {
	makeSess := func(id string) model.Session {
		return model.Session{
			ID:        id,
			Tool:      activeSourceName,
			Project:   "/tmp/test-project",
			Active:    true,
			UpdatedAt: time.Now(),
			StartedAt: time.Now(),
			Preview:   "test active session",
		}
	}
	s1 := makeSess("test-active-session-id-1")
	s2 := makeSess("test-active-session-id-2")
	if opts.Active && !s1.Active {
		return nil, nil
	}
	return []model.Session{s1, s2}, nil
}

func (a *activeSource) Get(_ string) (*model.Session, error) {
	return nil, nil
}

func (a *activeSource) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	makeSess := func(id string) model.Session {
		return model.Session{
			ID:        id,
			Tool:      activeSourceName,
			Project:   "/tmp/test-project",
			UpdatedAt: time.Now(),
		}
	}
	return []model.SearchResult{
		{Session: makeSess("test-active-session-id-1"), Matches: []model.SearchMatch{{Snippet: "match one"}}},
		{Session: makeSess("test-active-session-id-2"), Matches: []model.SearchMatch{{Snippet: "match two"}}},
	}, nil
}

func init() {
	source.Register(&errSource{})
	source.Register(&activeSource{})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resetFlags resets all package-level flags to their zero values between tests.
func resetFlags() {
	flagJSON = false
	flagTool = ""
	flagSince = ""
	flagLimit = 0
	flagProject = ""
}

// silenceOutput redirects stdout/stderr for the duration of the test so that
// output from renderers / warnings doesn't pollute test output.
func silenceOutput(t *testing.T) {
	t.Helper()
	origStdout := os.Stdout
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	t.Cleanup(func() {
		_ = w.Close()
		os.Stdout = origStdout
		os.Stderr = origStderr
		_, _ = io.ReadAll(r)
		_ = r.Close()
	})
}

// newNoopCmd returns a minimal *cobra.Command suitable for passing to run
// functions that accept a *cobra.Command argument but do not use it.
func newNoopCmd() *cobra.Command {
	return &cobra.Command{}
}

// ---------------------------------------------------------------------------
// getFormat
// ---------------------------------------------------------------------------

func TestGetFormat_Table(t *testing.T) {
	resetFlags()
	flagJSON = false
	got := getFormat()
	if got != output.FormatTable {
		t.Errorf("getFormat() = %q, want %q", got, output.FormatTable)
	}
}

func TestGetFormat_JSON(t *testing.T) {
	resetFlags()
	flagJSON = true
	got := getFormat()
	if got != output.FormatJSON {
		t.Errorf("getFormat() = %q, want %q", got, output.FormatJSON)
	}
}

// ---------------------------------------------------------------------------
// getSources
// ---------------------------------------------------------------------------

func TestGetSources_AllSources(t *testing.T) {
	resetFlags()
	// flagTool is "" — should return all registered sources.
	got := getSources()
	if len(got) == 0 {
		t.Error("getSources() with empty flagTool returned no sources")
	}
}

func TestGetSources_FilteredByTool(t *testing.T) {
	resetFlags()
	flagTool = "claude"
	got := getSources()
	for _, s := range got {
		if s.Name() != model.ToolClaude {
			t.Errorf("getSources() with flagTool=%q: got source %q", flagTool, s.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// getListOptions
// ---------------------------------------------------------------------------

func TestGetListOptions_Defaults(t *testing.T) {
	resetFlags()
	opts := getListOptions()
	if opts.Limit != 0 {
		t.Errorf("Limit = %d, want 0", opts.Limit)
	}
	if opts.Project != "" {
		t.Errorf("Project = %q, want empty", opts.Project)
	}
	if opts.Since != 0 {
		t.Errorf("Since = %v, want 0", opts.Since)
	}
}

func TestGetListOptions_WithLimit(t *testing.T) {
	resetFlags()
	flagLimit = 5
	opts := getListOptions()
	if opts.Limit != 5 {
		t.Errorf("Limit = %d, want 5", opts.Limit)
	}
}

func TestGetListOptions_WithProject(t *testing.T) {
	resetFlags()
	flagProject = "myapp"
	opts := getListOptions()
	if opts.Project != "myapp" {
		t.Errorf("Project = %q, want myapp", opts.Project)
	}
}

func TestGetListOptions_WithSince(t *testing.T) {
	resetFlags()
	flagSince = "24h"
	opts := getListOptions()
	if opts.Since != 24*time.Hour {
		t.Errorf("Since = %v, want 24h", opts.Since)
	}
}

// TestGetListOptions_InvalidSince_Subprocess covers the os.Exit(1) path that
// is triggered when flagSince is invalid. It runs the test binary as a
// subprocess with TEST_SUBPROCESS_INVALID_SINCE=1 to capture the exit.
func TestGetListOptions_InvalidSince_Subprocess(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_INVALID_SINCE") == "1" {
		// Running inside the subprocess: trigger the os.Exit path.
		resetFlags()
		flagSince = "not-a-duration"
		_ = getListOptions()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGetListOptions_InvalidSince_Subprocess")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_INVALID_SINCE=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected subprocess to exit non-zero, but it succeeded")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
}

// ---------------------------------------------------------------------------
// parseQualifiedID
// ---------------------------------------------------------------------------

func TestParseQualifiedID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTool  model.Tool
		wantID    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "claude",
			input:    "claude:abc123",
			wantTool: model.ToolClaude,
			wantID:   "abc123",
		},
		{
			name:     "cursor",
			input:    "cursor:def456",
			wantTool: model.ToolCursor,
			wantID:   "def456",
		},
		{
			name:     "codex",
			input:    "codex:ghi789",
			wantTool: model.ToolCodex,
			wantID:   "ghi789",
		},
		{
			name:     "gemini",
			input:    "gemini:jkl012",
			wantTool: model.ToolGemini,
			wantID:   "jkl012",
		},
		{
			name:      "no colon — format error",
			input:     "claude-abc123",
			wantErr:   true,
			errSubstr: "expected format",
		},
		{
			name:      "unknown tool",
			input:     "unknown:abc123",
			wantErr:   true,
			errSubstr: "unknown tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, id, err := parseQualifiedID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseQualifiedID(%q) returned nil error, want error containing %q",
						tt.input, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseQualifiedID(%q) unexpected error: %v", tt.input, err)
			}
			if tool != tt.wantTool {
				t.Errorf("tool = %q, want %q", tool, tt.wantTool)
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// runList
// ---------------------------------------------------------------------------

func TestRunList_HappyPath(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	// Use mock activeSource — guaranteed to return sessions without real local data.
	flagTool = string(activeSourceName)
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList returned unexpected error: %v", err)
	}
}

func TestRunList_WithLimit(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagLimit = 1
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList returned unexpected error: %v", err)
	}
}

// TestRunList_LimitApplied ensures the limit truncation branch (all = all[:limit])
// is exercised. activeSource returns 2 sessions, so with limit=1 the truncation
// triggers reliably without any real local data.
func TestRunList_LimitApplied(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagLimit = 1
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList (limit applied) returned unexpected error: %v", err)
	}
}

func TestRunList_SourceError(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	// errSource always returns an error from List.
	flagTool = string(errSourceName)
	err := runList(newNoopCmd(), nil)
	// runList logs the error as a warning and continues — it does NOT return an error.
	if err != nil {
		t.Errorf("runList returned error when source errors (expected nil): %v", err)
	}
}

func TestRunList_JSONFormat(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagJSON = true
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList (JSON) returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runActive
// ---------------------------------------------------------------------------

func TestRunActive_HappyPath(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	// activeSource returns active sessions — guaranteed without real local data.
	flagTool = string(activeSourceName)
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive returned unexpected error: %v", err)
	}
}

func TestRunActive_SourceError(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(errSourceName)
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive returned error when source errors (expected nil): %v", err)
	}
}

func TestRunActive_WithLimit(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagLimit = 1
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive (limit) returned unexpected error: %v", err)
	}
}

// TestRunActive_LimitApplied exercises the limit truncation and sort-closure
// branches. activeSource returns 2 active sessions, so limit=1 triggers
// truncation without needing real local data.
func TestRunActive_LimitApplied(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagLimit = 1
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive (limit applied) returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runSearch
// ---------------------------------------------------------------------------

func TestRunSearch_HappyPath(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	// activeSource.Search returns results — no real local data needed.
	flagTool = string(activeSourceName)
	err := runSearch(newNoopCmd(), []string{"some-query"})
	if err != nil {
		t.Errorf("runSearch returned unexpected error: %v", err)
	}
}

func TestRunSearch_SourceError(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(errSourceName)
	err := runSearch(newNoopCmd(), []string{"query"})
	if err != nil {
		t.Errorf("runSearch returned error when source errors (expected nil): %v", err)
	}
}

func TestRunSearch_WithLimitAndJSON(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagLimit = 1
	flagJSON = true
	err := runSearch(newNoopCmd(), []string{"query"})
	if err != nil {
		t.Errorf("runSearch (limit+JSON) returned unexpected error: %v", err)
	}
}

// TestRunSearch_LimitApplied exercises the limit truncation and sort-closure
// branches in runSearch. activeSource returns 2 search results, so limit=1
// triggers truncation without needing real local data.
func TestRunSearch_LimitApplied(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(activeSourceName)
	flagLimit = 1
	err := runSearch(newNoopCmd(), []string{"the"})
	if err != nil {
		t.Errorf("runSearch (limit applied) returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runShow
// ---------------------------------------------------------------------------

// TestRunShow_NotFound_Stub covers the source-lookup and Get/nil branches of
// runShow using the gemini stub, which returns nil, nil from Get() instantly
// with no disk access.
func TestRunShow_NotFound_Stub(t *testing.T) {
	resetFlags()
	err := runShow(newNoopCmd(), []string{"gemini:any-session-id"})
	if err == nil {
		t.Error("expected 'session not found' error, got nil")
	}
}

func TestRunShow_InvalidFormat(t *testing.T) {
	resetFlags()
	// No colon in the argument → parseQualifiedID returns an error.
	err := runShow(newNoopCmd(), []string{"claude-abc123"})
	if err == nil {
		t.Error("runShow with invalid format: expected error, got nil")
	}
}

func TestRunShow_UnknownTool(t *testing.T) {
	resetFlags()
	err := runShow(newNoopCmd(), []string{"unknown:abc123"})
	if err == nil {
		t.Error("runShow with unknown tool: expected error, got nil")
	}
}

// TestRunShow_SessionNotFound covers the "session not found" path in runShow.
// Claude's Get() returns nil, nil for a non-existent session, so runShow
// returns a "session not found" error. Uses a UUIDv4 that cannot exist on disk.
func TestRunShow_SessionNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: scans real ~/.claude sessions")
	}
	silenceOutput(t)
	resetFlags()
	// Claude source returns nil, nil when session doesn't exist.
	err := runShow(newNoopCmd(), []string{"claude:00000000-0000-0000-0000-000000000000"})
	if err == nil {
		t.Error("runShow with non-existent session: expected 'not found' error, got nil")
	}
}

// TestRunShow_GetError covers the "Get returns an error" path in runShow.
// Cursor's Get() returns an error when the session file is missing.
func TestRunShow_GetError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: scans real ~/.cursor sessions")
	}
	silenceOutput(t)
	resetFlags()
	// Cursor source returns an error when the session is not found (not nil, nil).
	// Use a non-existent 8+ char ID so prefix-match is also attempted.
	err := runShow(newNoopCmd(), []string{"cursor:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"})
	if err == nil {
		t.Error("runShow (cursor, nonexistent): expected error from Get, got nil")
	}
}

// TestRunShow_FoundSession exercises the "session found and rendered" path in
// runShow. It dynamically retrieves the first real claude session from disk, so
// this test is a no-op if no claude sessions exist.
func TestRunShow_FoundSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: reads real ~/.claude sessions")
	}
	silenceOutput(t)
	resetFlags()

	sources := source.ByName(model.ToolClaude)
	if len(sources) == 0 {
		t.Skip("no claude source registered")
	}
	sessions, err := sources[0].List(source.ListOptions{Limit: 1})
	if err != nil || len(sessions) == 0 {
		t.Skip("no claude sessions available on this machine")
	}

	qualID := "claude:" + sessions[0].ID
	err = runShow(newNoopCmd(), []string{qualID})
	if err != nil {
		t.Errorf("runShow with valid session %q returned error: %v", qualID, err)
	}
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

// TestExecute_Help verifies that Execute succeeds when --help is requested.
// We call rootCmd.Execute() directly to avoid os.Exit in Execute().
func TestExecute_Help(t *testing.T) {
	silenceOutput(t)
	rootCmd.SetArgs([]string{"--help"})
	// cobra prints help and returns nil on --help.
	err := rootCmd.Execute()
	// restore default args
	rootCmd.SetArgs(nil)
	if err != nil {
		t.Errorf("rootCmd.Execute(--help) returned error: %v", err)
	}
}

// TestExecute_Subprocess covers the os.Exit(1) path in Execute() when cobra
// returns an error (e.g., invalid flag). The subprocess runs this test binary
// with TEST_SUBPROCESS_EXECUTE=1 and calls Execute() with a bad flag.
func TestExecute_Subprocess(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_EXECUTE") == "1" {
		// Inject a bad flag so rootCmd.Execute() returns an error.
		rootCmd.SetArgs([]string{"--bad-flag-that-does-not-exist"})
		Execute()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExecute_Subprocess")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_EXECUTE=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected subprocess to exit non-zero, but it succeeded")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
}
