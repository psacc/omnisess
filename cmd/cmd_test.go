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

// activeSource is a source.Source that always returns one active session.
// It is registered alongside errSource to provide a second source with active
// sessions when running with flagTool="", enabling the limit truncation branch
// in runActive to be reached (two sources each returning 1 active session →
// len(all)=2 > opts.Limit=1).
const activeSourceName = model.Tool("test-active-src")

type activeSource struct{}

func (a *activeSource) Name() model.Tool { return activeSourceName }

func (a *activeSource) List(opts source.ListOptions) ([]model.Session, error) {
	sess := model.Session{
		ID:        "test-active-session-id",
		Tool:      activeSourceName,
		Project:   "/tmp/test-project",
		Active:    true,
		UpdatedAt: time.Now(),
		StartedAt: time.Now(),
		Preview:   "test active session",
	}
	if opts.Active && !sess.Active {
		return nil, nil
	}
	return []model.Session{sess}, nil
}

func (a *activeSource) Get(_ string) (*model.Session, error) {
	return nil, nil
}

func (a *activeSource) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
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
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	// Use codex tool — likely returns empty list (no ~/.codex) with no error.
	flagTool = "codex"
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList returned unexpected error: %v", err)
	}
}

func TestRunList_WithLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
	flagLimit = 1
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList returned unexpected error: %v", err)
	}
}

// TestRunList_LimitApplied ensures the limit truncation branch (all = all[:limit])
// is exercised. Uses all sources so that sessions from multiple sources combine
// to exceed the limit of 1, triggering the truncation in runList itself
// (the sources each return at most 1 session, but 2+ sources with data
// results in len(all) > 1 > limit).
func TestRunList_LimitApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	// flagTool = "" → all sources; claude + cursor both have sessions here,
	// giving len(all) >= 2 > opts.Limit = 1.
	flagLimit = 1
	err := runList(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runList (all sources, limit=1) returned unexpected error: %v", err)
	}
}

func TestRunList_SourceError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
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
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
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
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive returned unexpected error: %v", err)
	}
}

func TestRunActive_SourceError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = string(errSourceName)
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive returned error when source errors (expected nil): %v", err)
	}
}

func TestRunActive_WithLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
	flagLimit = 1
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive (limit) returned unexpected error: %v", err)
	}
}

// TestRunActive_LimitApplied exercises the limit truncation and sort-closure
// branches. Uses all sources with limit=1 so that the combined active sessions
// from multiple tools exceed the limit.
func TestRunActive_LimitApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	// flagTool = "" → all sources with Active=true filter.
	// Multiple sources may return active sessions, giving len(all) > 1.
	flagLimit = 1
	err := runActive(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runActive (all sources, limit=1) returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runSearch
// ---------------------------------------------------------------------------

func TestRunSearch_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
	err := runSearch(newNoopCmd(), []string{"some-query"})
	if err != nil {
		t.Errorf("runSearch returned unexpected error: %v", err)
	}
}

func TestRunSearch_SourceError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = string(errSourceName)
	err := runSearch(newNoopCmd(), []string{"query"})
	if err != nil {
		t.Errorf("runSearch returned error when source errors (expected nil): %v", err)
	}
}

func TestRunSearch_WithLimitAndJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
	flagLimit = 1
	flagJSON = true
	err := runSearch(newNoopCmd(), []string{"query"})
	if err != nil {
		t.Errorf("runSearch (limit+JSON) returned unexpected error: %v", err)
	}
}

// TestRunSearch_LimitApplied exercises the limit truncation and sort-closure
// by running a common term against all sources with limit=1.
func TestRunSearch_LimitApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	// flagTool = "" → all sources; "the" is common enough to match in
	// multiple sessions across multiple tools, giving len(all) > 1 > limit.
	flagLimit = 1
	err := runSearch(newNoopCmd(), []string{"the"})
	if err != nil {
		t.Errorf("runSearch (all sources, limit=1) returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runShow
// ---------------------------------------------------------------------------

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

func TestRunShow_SessionNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	// Claude source returns nil, nil when session doesn't exist.
	err := runShow(newNoopCmd(), []string{"claude:00000000-0000-0000-0000-000000000000"})
	if err == nil {
		t.Error("runShow with non-existent session: expected 'not found' error, got nil")
	}
}

func TestRunShow_GetError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
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
		t.Skip("skipping integration test in short mode")
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
