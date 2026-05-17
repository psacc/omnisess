package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/output"
	"github.com/psacc/omnisess/internal/resume"
	"github.com/psacc/omnisess/internal/source"
)

// TestMain jails $HOME to a temp dir for the whole package so tests can
// never reach the developer's real ~/.claude, ~/.cursor, etc. This is a
// structural guard — without it any test that forgets to stub will silently
// scan the 1.5GB real home directory and pad every test run by ~20s. Tests
// that need synthetic session data should build fixtures under this jail.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "omnisess-cmd-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: failed to create jailed HOME: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("HOME", tmp); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: failed to set HOME: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

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

// mockResumer is a no-op resume.Resumer used to exercise the resumer.Exec()
// dispatch path in handleTUIResult without replacing the test process.
// Uses a synthetic tool name to avoid colliding with real registered resumers.
// claude/cursor resumers are registered first (via blank imports in tui_test.go),
// so init() order is safe.
const mockResumerTool = model.Tool("test-mock-resumer")

type mockResumer struct{}

func (r *mockResumer) Tool() model.Tool                           { return mockResumerTool }
func (r *mockResumer) Modes() []resume.Mode                       { return []resume.Mode{resume.ModeResume} }
func (r *mockResumer) Exec(_ *model.Session, _ resume.Mode) error { return nil }

// getErrSource always returns an error from Get (used to cover showSession error path).
const getErrSourceName = model.Tool("test-get-err-src")

type getErrSource struct{}

func (g *getErrSource) Name() model.Tool                                   { return getErrSourceName }
func (g *getErrSource) List(_ source.ListOptions) ([]model.Session, error) { return nil, nil }
func (g *getErrSource) Get(_ string) (*model.Session, error) {
	return nil, errors.New("mock get error")
}
func (g *getErrSource) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

// getSessionSource always returns a valid session from Get (used to cover showSession success path).
const getSessionSourceName = model.Tool("test-get-session-src")

type getSessionSource struct{}

func (g *getSessionSource) Name() model.Tool                                   { return getSessionSourceName }
func (g *getSessionSource) List(_ source.ListOptions) ([]model.Session, error) { return nil, nil }
func (g *getSessionSource) Get(_ string) (*model.Session, error) {
	return &model.Session{ID: "test-session-id", Tool: getSessionSourceName}, nil
}
func (g *getSessionSource) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// digest mock sources
// ---------------------------------------------------------------------------

const (
	digestSrcName       = model.Tool("test-digest-src")
	digestGetErrSrcName = model.Tool("test-digest-get-err-src")
	digestGetNilSrcName = model.Tool("test-digest-get-nil-src")
)

type digestSrc struct{}

func (d *digestSrc) Name() model.Tool { return digestSrcName }

// List returns two sessions in INTENTIONALLY non-StartedAt order so that any
// regression in runDigestTo's sort step is detected by TestRunDigest_WithFlagDate.
func (d *digestSrc) List(_ source.ListOptions) ([]model.Session, error) {
	return []model.Session{
		{
			ID:        "digest-sess-2",
			Tool:      digestSrcName,
			Project:   "/tmp/test-project",
			Preview:   "Second question?",
			StartedAt: time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 10, 10, 1, 0, 0, time.UTC),
		},
		{
			ID:        "digest-sess-1",
			Tool:      digestSrcName,
			Project:   "/tmp/test-project",
			Preview:   "First question?",
			StartedAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 10, 9, 1, 0, 0, time.UTC),
		},
	}, nil
}
func (d *digestSrc) Get(id string) (*model.Session, error) {
	switch id {
	case "digest-sess-1":
		return &model.Session{
			ID:      "digest-sess-1",
			Tool:    digestSrcName,
			Project: "/tmp/test-project",
			Preview: "First question?",
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "First question?", Timestamp: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)},
				{Role: model.RoleAssistant, Content: "Answer 1.", Timestamp: time.Date(2026, 5, 10, 9, 1, 0, 0, time.UTC)},
			},
		}, nil
	case "digest-sess-2":
		return &model.Session{
			ID:      "digest-sess-2",
			Tool:    digestSrcName,
			Project: "/tmp/test-project",
			Preview: "Second question?",
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "Second question?"},
				{Role: model.RoleAssistant, Content: "Answer 2."},
			},
		}, nil
	}
	return nil, nil
}
func (d *digestSrc) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

type digestGetErrSrc struct{}

func (d *digestGetErrSrc) Name() model.Tool { return digestGetErrSrcName }
func (d *digestGetErrSrc) List(_ source.ListOptions) ([]model.Session, error) {
	return []model.Session{{ID: "digest-get-err-sess", Tool: digestGetErrSrcName}}, nil
}
func (d *digestGetErrSrc) Get(_ string) (*model.Session, error) {
	return nil, errors.New("mock digest get error")
}
func (d *digestGetErrSrc) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

type digestGetNilSrc struct{}

func (d *digestGetNilSrc) Name() model.Tool { return digestGetNilSrcName }
func (d *digestGetNilSrc) List(_ source.ListOptions) ([]model.Session, error) {
	return []model.Session{{ID: "digest-get-nil-sess", Tool: digestGetNilSrcName}}, nil
}
func (d *digestGetNilSrc) Get(_ string) (*model.Session, error) { return nil, nil }
func (d *digestGetNilSrc) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

func init() {
	source.Register(&errSource{})
	source.Register(&activeSource{})
	source.Register(&getErrSource{})
	source.Register(&getSessionSource{})
	source.Register(&digestSrc{})
	source.Register(&digestGetErrSrc{})
	source.Register(&digestGetNilSrc{})
	resume.Register(&mockResumer{})
}

// ---------------------------------------------------------------------------
// showSession
// ---------------------------------------------------------------------------

// TestShowSession_GetError covers the "failed to get session" error path.
func TestShowSession_GetError(t *testing.T) {
	err := showSession(&getErrSource{}, "test-get-err-src:abc", "abc", output.FormatTable)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get session") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestShowSession_Found covers the "session found and rendered" success path.
func TestShowSession_Found(t *testing.T) {
	silenceOutput(t)
	err := showSession(&getSessionSource{}, "test-get-session-src:test-session-id", "test-session-id", output.FormatTable)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resetFlags resets all package-level flags to their zero values between tests.
func resetFlags() {
	flagJSON = false
	flagTool = ""
	flagSince = ""
	flagDate = ""
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

func TestGetListOptions_WithDate(t *testing.T) {
	resetFlags()
	flagDate = "2026-04-22"
	opts := getListOptions()
	if opts.OnDate.IsZero() {
		t.Fatal("OnDate should be set")
	}
	y, m, d := opts.OnDate.Date()
	if y != 2026 || m != time.April || d != 22 {
		t.Errorf("OnDate = %v, want 2026-04-22", opts.OnDate)
	}
	// Parsed in time.Local — verify the location matches.
	if opts.OnDate.Location() != time.Local {
		t.Errorf("OnDate.Location() = %v, want Local", opts.OnDate.Location())
	}
}

// TestGetListOptions_InvalidDate_Subprocess covers the os.Exit(1) path
// triggered when flagDate is malformed.
func TestGetListOptions_InvalidDate_Subprocess(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_INVALID_DATE") == "1" {
		resetFlags()
		flagDate = "not-a-date"
		_ = getListOptions()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGetListOptions_InvalidDate_Subprocess")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_INVALID_DATE=1")
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
			name:     "copilot",
			input:    "copilot:mno345",
			wantTool: model.ToolCopilot,
			wantID:   "mno345",
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
		{
			// Regression for PR #44: copilot was registered but missing from the error
			// message's allow-list. Asserting it explicitly so the next source-list
			// drift is caught at this test.
			name:      "unknown tool error message lists copilot",
			input:     "unknown:abc123",
			wantErr:   true,
			errSubstr: "copilot",
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
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("parseQualifiedID(%q) error = %q, want substring %q",
						tt.input, err.Error(), tt.errSubstr)
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

// ---------------------------------------------------------------------------
// runDigest
// ---------------------------------------------------------------------------

// TestRunDigest_CobraAdapter ensures the thin runDigest(cobra) wrapper that
// forwards to runDigestTo(os.Stdout) is covered. All behavioral assertions live
// in the runDigestTo tests below.
func TestRunDigest_CobraAdapter(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	flagTool = string(errSourceName)
	if err := runDigest(newNoopCmd(), nil); err != nil {
		t.Errorf("runDigest (cobra adapter) returned unexpected error: %v", err)
	}
}

// TestRunDigest_DefaultsToToday covers the "no time filter → default to today" path.
// Asserts today's date appears in the header and the session count is 0
// (errSource only warns; it returns no sessions).
func TestRunDigest_DefaultsToToday(t *testing.T) {
	silenceOutput(t) // suppress errSource's stderr warning
	resetFlags()
	flagTool = string(errSourceName)

	var buf strings.Builder
	if err := runDigestTo(&buf); err != nil {
		t.Fatalf("runDigestTo returned unexpected error: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	out := buf.String()
	if !strings.Contains(out, "## AI sessions — "+today) {
		t.Errorf("expected today's date %q in header; got: %q", today, out)
	}
	if !strings.Contains(out, "(0 sessions)") {
		t.Errorf("expected (0 sessions) in header; got: %q", out)
	}
	if strings.Contains(out, "###") {
		t.Errorf("expected no session headings when 0 sessions; got: %q", out)
	}
}

// TestRunDigest_WithFlagDate verifies the flag-date branch AND the sort-by-StartedAt
// branch. digestSrc.List returns sessions in non-StartedAt order; the test asserts
// they appear in StartedAt order in the rendered output.
func TestRunDigest_WithFlagDate(t *testing.T) {
	resetFlags()
	flagDate = "2026-05-10"
	flagTool = string(digestSrcName)

	var buf strings.Builder
	if err := runDigestTo(&buf); err != nil {
		t.Fatalf("runDigestTo returned unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "## AI sessions — 2026-05-10") {
		t.Errorf("expected flagDate %q in header; got: %q", "2026-05-10", out)
	}
	if !strings.Contains(out, "(2 sessions)") {
		t.Errorf("expected (2 sessions) in header; got: %q", out)
	}
	// Sort regression: digestSrc.List returns [sess-2, sess-1]; runDigestTo must
	// reorder them by StartedAt so sess-1 (09:00) appears before sess-2 (10:00).
	idx1 := strings.Index(out, "First question?")
	idx2 := strings.Index(out, "Second question?")
	if idx1 == -1 || idx2 == -1 {
		t.Fatalf("expected both session titles in output; got: %q", out)
	}
	if idx1 >= idx2 {
		t.Errorf("sessions out of order: First question? at %d, Second question? at %d; output: %q",
			idx1, idx2, out)
	}
}

// TestRunDigest_SourceError covers the "source.List error → warn and continue" path.
// Asserts the header still renders with 0 sessions.
func TestRunDigest_SourceError(t *testing.T) {
	silenceOutput(t) // suppress errSource's stderr warning
	resetFlags()
	flagDate = "2026-05-10"
	flagTool = string(errSourceName)

	var buf strings.Builder
	if err := runDigestTo(&buf); err != nil {
		t.Fatalf("runDigestTo returned unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "(0 sessions)") {
		t.Errorf("source error should yield 0 sessions in header; got: %q", out)
	}
}

// TestRunDigest_LimitApplied covers the limit truncation branch. digestSrc.List
// returns 2 sessions; with flagLimit=1 the output must contain only one and the
// header must report (1 sessions).
func TestRunDigest_LimitApplied(t *testing.T) {
	resetFlags()
	flagDate = "2026-05-10"
	flagTool = string(digestSrcName)
	flagLimit = 1

	var buf strings.Builder
	if err := runDigestTo(&buf); err != nil {
		t.Fatalf("runDigestTo returned unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "(1 sessions)") {
		t.Errorf("expected (1 sessions) in header with limit=1; got: %q", out)
	}
	if got := strings.Count(out, "### "); got != 1 {
		t.Errorf("expected exactly 1 session heading with limit=1; got %d in: %q", got, out)
	}
	// With only 1 session there should be no separator.
	if strings.Contains(out, "\n---\n") {
		t.Errorf("expected no separator with limit=1; got: %q", out)
	}
	// After sort, the earlier session (sess-1 at 09:00) wins.
	if !strings.Contains(out, "First question?") {
		t.Errorf("limit=1 should keep the earliest session (First question?); got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// writeDigest
// ---------------------------------------------------------------------------

func TestWriteDigest_NoSessions(t *testing.T) {
	var buf strings.Builder
	writeDigest(&buf, nil, nil, "2026-05-10")
	got := buf.String()
	if !strings.Contains(got, "0 sessions") {
		t.Errorf("no-sessions digest missing count; got: %q", got)
	}
	if strings.Contains(got, "###") {
		t.Errorf("no-sessions digest should not have session headings; got: %q", got)
	}
}

func TestWriteDigest_UnknownTool(t *testing.T) {
	var buf strings.Builder
	sessions := []model.Session{{ID: "s1", Tool: model.Tool("no-such-tool")}}
	writeDigest(&buf, sessions, map[model.Tool]source.Source{}, "2026-05-10")
	got := buf.String()
	if strings.Contains(got, "###") {
		t.Errorf("unknown-tool session should be skipped; got: %q", got)
	}
}

func TestWriteDigest_GetError(t *testing.T) {
	var buf strings.Builder
	sessions := []model.Session{{ID: "err-sess", Tool: digestGetErrSrcName}}
	srcMap := map[model.Tool]source.Source{digestGetErrSrcName: &digestGetErrSrc{}}
	writeDigest(&buf, sessions, srcMap, "2026-05-10")
	got := buf.String()
	if strings.Contains(got, "###") {
		t.Errorf("get-error session should be skipped; got: %q", got)
	}
}

func TestWriteDigest_GetNil(t *testing.T) {
	var buf strings.Builder
	sessions := []model.Session{{ID: "nil-sess", Tool: digestGetNilSrcName}}
	srcMap := map[model.Tool]source.Source{digestGetNilSrcName: &digestGetNilSrc{}}
	writeDigest(&buf, sessions, srcMap, "2026-05-10")
	got := buf.String()
	if strings.Contains(got, "###") {
		t.Errorf("nil-session should be skipped; got: %q", got)
	}
}

func TestWriteDigest_MultiSession(t *testing.T) {
	var buf strings.Builder
	sessions := []model.Session{
		{ID: "digest-sess-1", Tool: digestSrcName},
		{ID: "digest-sess-2", Tool: digestSrcName},
	}
	srcMap := map[model.Tool]source.Source{digestSrcName: &digestSrc{}}
	writeDigest(&buf, sessions, srcMap, "2026-05-10")
	got := buf.String()
	if strings.Count(got, "---") != 1 {
		t.Errorf("two sessions should produce exactly 1 separator; got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// writeDigestSession
// ---------------------------------------------------------------------------

func TestWriteDigestSession(t *testing.T) {
	home := os.Getenv("HOME") // set to tmpdir by TestMain

	tests := []struct {
		name     string
		session  *model.Session
		wantSubs []string
		notWant  []string
	}{
		{
			name: "basic user and assistant turns",
			session: &model.Session{
				ID:      "sess-1",
				Tool:    model.ToolClaude,
				Project: "/tmp/project",
				Preview: "Hello?",
				Messages: []model.Message{
					{Role: model.RoleUser, Content: "Hello?", Timestamp: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)},
					{Role: model.RoleAssistant, Content: "World!", Timestamp: time.Date(2026, 5, 10, 9, 1, 0, 0, time.UTC)},
				},
			},
			wantSubs: []string{"### Hello?", "**Q [", "**A [", "Hello?", "World!"},
		},
		{
			name: "no preview uses ID as title",
			session: &model.Session{
				ID:      "my-session-id",
				Tool:    model.ToolClaude,
				Project: "/tmp/project",
			},
			wantSubs: []string{"### my-session-id"},
		},
		{
			name: "long title truncated to 80 chars",
			session: &model.Session{
				Preview: strings.Repeat("A", 90),
				Tool:    model.ToolClaude,
				Project: "/tmp/project",
			},
			wantSubs: []string{"### " + strings.Repeat("A", 80)},
			notWant:  []string{strings.Repeat("A", 81)},
		},
		{
			// Regression: title truncation at byte 80 used to slice mid-rune for
			// multi-byte UTF-8 (accents, emoji, CJK). The utf8.ValidString check
			// in the test loop catches this; we also explicitly verify no Unicode
			// replacement character escaped.
			name: "title with multi-byte rune at byte-80 boundary",
			session: &model.Session{
				// 79 ASCII bytes + 'é' (2 bytes) — naive [:80] cuts between the
				// two bytes of 'é' and produces invalid UTF-8.
				Preview: strings.Repeat("a", 79) + "é",
				Tool:    model.ToolClaude,
				Project: "/tmp/project",
			},
			notWant: []string{"�"},
		},
		{
			// Regression: emoji are 4 bytes; the same byte-slice bug applies at
			// every boundary that intersects a multi-byte rune.
			name: "title with 4-byte emoji at byte boundary",
			session: &model.Session{
				// 78 ASCII + emoji (4 bytes, occupies bytes 78-81). Any byte
				// truncate inside it produces invalid UTF-8.
				Preview: strings.Repeat("a", 78) + "🎉extra",
				Tool:    model.ToolClaude,
				Project: "/tmp/project",
			},
			notWant: []string{"�"},
		},
		{
			// Regression: user-turn truncation at byte 2000 used to cut mid-rune.
			// 1999 ASCII + 2 'é' = 2001 runes, 2003 bytes. Byte slice [:2000]
			// cuts inside the first 'é' → invalid UTF-8. Rune slice [:2000] =
			// 1999 'a' + 1 'é' → valid + truncated marker.
			name: "user turn rune-truncated with multi-byte content at boundary",
			session: &model.Session{
				ID:   "sess-user-runes",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleUser, Content: strings.Repeat("a", 1999) + "éé"},
				},
			},
			wantSubs: []string{"_(truncated)_"},
			notWant:  []string{"�"},
		},
		{
			// Regression: assistant-turn truncation at byte 2000 had the same bug.
			name: "assistant turn rune-truncated with multi-byte content at boundary",
			session: &model.Session{
				ID:   "sess-asst-runes",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleAssistant, Content: strings.Repeat("a", 1999) + "éé"},
				},
			},
			wantSubs: []string{"_(truncated)_"},
			notWant:  []string{"�"},
		},
		{
			name: "project with home prefix replaced by tilde",
			session: &model.Session{
				ID:      "sess-home",
				Tool:    model.ToolClaude,
				Preview: "Q",
				Project: home + "/myproject",
			},
			wantSubs: []string{"~/myproject"},
		},
		{
			name: "empty user message skipped",
			session: &model.Session{
				ID:   "sess-empty-user",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleUser, Content: ""},
					{Role: model.RoleUser, Content: "Real question?"},
				},
			},
			wantSubs: []string{"Real question?"},
		},
		{
			name: "user turn truncated at 2000 chars",
			session: &model.Session{
				ID:   "sess-user-trunc",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleUser, Content: strings.Repeat("x", 2100)},
				},
			},
			wantSubs: []string{"_(truncated)_"},
		},
		{
			name: "assistant turn truncated at 2000 chars",
			session: &model.Session{
				ID:   "sess-asst-trunc",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleAssistant, Content: strings.Repeat("y", 2100)},
				},
			},
			wantSubs: []string{"_(truncated)_"},
		},
		{
			name: "assistant tool-calls-only emits tool names",
			session: &model.Session{
				ID:   "sess-tools",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{
						Role:      model.RoleAssistant,
						Content:   "",
						ToolCalls: []model.ToolCall{{Name: "Bash"}, {Name: "Read"}},
					},
				},
			},
			wantSubs: []string{"_[tools: Bash, Read]_"},
		},
		{
			name: "assistant empty with no tools silently skipped",
			session: &model.Session{
				ID:   "sess-empty-asst",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleAssistant, Content: ""},
					{Role: model.RoleUser, Content: "After empty assistant"},
				},
			},
			wantSubs: []string{"After empty assistant"},
			notWant:  []string{"**A"},
		},
		{
			name: "no timestamp omits bracket",
			session: &model.Session{
				ID:   "sess-no-ts",
				Tool: model.ToolClaude,
				Messages: []model.Message{
					{Role: model.RoleUser, Content: "No timestamp here"},
				},
			},
			wantSubs: []string{"**Q:** No timestamp here"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			writeDigestSession(&buf, tt.session)
			got := buf.String()
			// Defensive: every digest fragment must be valid UTF-8. This is a
			// tripwire for future byte-slicing regressions (a multi-byte rune
			// cut at a byte boundary produces invalid UTF-8).
			if !utf8.ValidString(got) {
				t.Errorf("writeDigestSession produced invalid UTF-8 output:\n%q", got)
			}
			for _, want := range tt.wantSubs {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("output should not contain %q:\n%s", nw, got)
				}
			}
		})
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
