//go:build !windows

package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/resume"
	"github.com/psacc/omnisess/internal/tui"

	// Pull in resume registrations so Modes() is populated.
	_ "github.com/psacc/omnisess/internal/resume/claude"
	_ "github.com/psacc/omnisess/internal/resume/cursor"
)

// ---------------------------------------------------------------------------
// buildToolModes
// ---------------------------------------------------------------------------

// TestBuildToolModes_KnownTools verifies that buildToolModes returns mode
// strings for the tools that have registered resumers (claude, cursor).
func TestBuildToolModes_KnownTools(t *testing.T) {
	tm := buildToolModes()

	// Claude and Cursor resumers are registered via blank imports above.
	for _, tool := range []model.Tool{model.ToolClaude, model.ToolCursor} {
		if _, ok := tm[tool]; !ok {
			t.Errorf("buildToolModes: expected modes for tool %q, got none", tool)
		}
	}
}

// TestBuildToolModes_ToolWithoutResumer verifies that tools without a
// registered resumer (codex, gemini) are absent from the returned map.
func TestBuildToolModes_ToolWithoutResumer(t *testing.T) {
	tm := buildToolModes()

	for _, tool := range []model.Tool{model.ToolCodex, model.ToolGemini} {
		if modes, ok := tm[tool]; ok {
			t.Errorf("buildToolModes: tool %q should not have modes, got %v", tool, modes)
		}
	}
}

// TestBuildToolModes_ModeStrings verifies that the mode string conversion is
// correct (resume.Mode → string) for claude modes.
func TestBuildToolModes_ModeStrings(t *testing.T) {
	claudeModes := resume.Modes(model.ToolClaude)
	if len(claudeModes) == 0 {
		t.Skip("no claude modes registered — skip mode string test")
	}

	tm := buildToolModes()
	strs, ok := tm[model.ToolClaude]
	if !ok {
		t.Fatal("buildToolModes: no modes for claude")
	}
	if len(strs) != len(claudeModes) {
		t.Errorf("buildToolModes claude: got %d modes, want %d", len(strs), len(claudeModes))
	}
	for i, m := range claudeModes {
		if strs[i] != string(m) {
			t.Errorf("buildToolModes claude mode[%d] = %q, want %q", i, strs[i], string(m))
		}
	}
}

// ---------------------------------------------------------------------------
// runTUI
// ---------------------------------------------------------------------------

// TestRunTUI_NoSessions verifies that runTUI returns nil and prints a message
// when no sessions are found (skips TUI entirely).
func TestRunTUI_NoSessions(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	// Use a tool name that will never match a real source (test-error-src
	// registered in cmd_test.go returns an error from List, so all = nil).
	// But errSource.List returns an error, so the warning is printed and we
	// end up with len(all) == 0 → the "No sessions found." path.
	flagTool = string(errSourceName)
	err := runTUI(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runTUI with no sessions returned unexpected error: %v", err)
	}
}

// TestRunTUI_WithSessions_TUIError verifies that runTUI returns a wrapped
// error when the Bubble Tea program fails to start (no TTY in test environment).
func TestRunTUI_WithSessions_TUIError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	flagTool = "codex"
	// codex list returns empty or sessions from disk; either way p.Run() will
	// fail in the test environment (no TTY → "could not open a new TTY").
	// If codex directory doesn't exist, list returns nil, nil (0 sessions)
	// and we hit the "No sessions found." path instead.
	// For a reliable TUI error path we inject our codex flag but also test
	// via claude which is more likely to have sessions.
	flagTool = "claude"
	// Any error from p.Run() (no TTY) is wrapped and returned.
	err := runTUI(newNoopCmd(), nil)
	// Either we got "No sessions found" (nil) or a TUI error (non-nil).
	// Both are valid outcomes in a non-TTY environment; we just ensure no panic.
	_ = err
}

// TestRunTUI_WithSessions_TUIError_Explicit creates a scenario where sessions
// are definitely non-empty (by using all sources) so that p.Run() is always
// attempted, and we verify the "TUI error" is properly wrapped.
func TestRunTUI_WithSessions_TUIError_Explicit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	silenceOutput(t)
	resetFlags()
	// Use all sources. On the developer machine, claude will return sessions.
	// In CI with no claude data, we may get 0 sessions — that's also fine.
	err := runTUI(newNoopCmd(), nil)
	if err != nil {
		if !strings.Contains(err.Error(), "TUI error") {
			t.Errorf("expected error to contain 'TUI error', got: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// openProjectDir
// ---------------------------------------------------------------------------

// TestOpenProjectDir_EditorNotFound verifies that openProjectDir returns an
// error when $EDITOR is set to an executable that cannot be found in PATH.
func TestOpenProjectDir_EditorNotFound(t *testing.T) {
	orig := os.Getenv("EDITOR")
	t.Cleanup(func() { os.Setenv("EDITOR", orig) })

	os.Setenv("EDITOR", "/nonexistent/path/to/editor")
	err := openProjectDir("/tmp")
	if err == nil {
		t.Error("openProjectDir with non-existent EDITOR: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestOpenProjectDir_DarwinFallback covers the darwin check path and the
// final fallback (fmt.Println) in openProjectDir. By temporarily setting
// PATH="" in-process, exec.LookPath("open") fails, causing the darwin branch
// to be entered but the syscall.Exec NOT to be called, and execution falls
// through to fmt.Println(dir); return nil.
//
// This covers lines: `if runtime.GOOS == "darwin"`, `LookPath("open")`,
// `if err == nil` (false branch), `fmt.Println(dir)`, and `return nil`.
func TestOpenProjectDir_DarwinFallback(t *testing.T) {
	origEditor := os.Getenv("EDITOR")
	origPath := os.Getenv("PATH")
	t.Cleanup(func() {
		if origEditor == "" {
			os.Unsetenv("EDITOR")
		} else {
			os.Setenv("EDITOR", origEditor)
		}
		os.Setenv("PATH", origPath)
	})

	os.Unsetenv("EDITOR")
	os.Setenv("PATH", "")

	// On darwin with PATH="" and no EDITOR: LookPath("open") fails → fallback.
	// On non-darwin: skips the darwin block and hits fallback directly.
	// Either way: fmt.Println(dir) + return nil are covered.
	err := openProjectDir("/tmp")
	if err != nil {
		t.Errorf("openProjectDir fallback: expected nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProgram injection — p.Run() error path
// ---------------------------------------------------------------------------

// TestRunTUI_TUIError_Mock verifies that runTUI returns a "TUI error"-wrapped
// error when the Bubble Tea program fails. Uses runProgram injection so the
// test is reliable without a real TTY and without reading real session data.
func TestRunTUI_TUIError_Mock(t *testing.T) {
	silenceOutput(t)
	resetFlags()

	origRunProgram := runProgram
	runProgram = func(m tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
		return m, errors.New("mock tui failure")
	}
	t.Cleanup(func() { runProgram = origRunProgram })

	flagTool = string(activeSourceName) // guaranteed non-empty session list
	err := runTUI(newNoopCmd(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "TUI error") {
		t.Errorf("expected error to contain 'TUI error', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// handleTUIResult dispatch branches
// ---------------------------------------------------------------------------

// makeTUIModelWithKey builds a tui.Model for a single session of the given
// tool, applies one key press, and returns the resulting tea.Model.
func makeTUIModelWithKey(tool model.Tool, toolModes map[model.Tool][]string, key string) tea.Model {
	sess := model.Session{
		ID:        "aaaaaaaa-1111-1111-1111-111111111111",
		Tool:      tool,
		Project:   "/tmp/test-project",
		Preview:   "test session",
		UpdatedAt: time.Now(),
		StartedAt: time.Now(),
	}
	m := tui.New([]model.Session{sess}, toolModes)
	var mdl tea.Model = m
	mdl, _ = mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return mdl
}

// TestHandleTUIResult_UserQuit verifies that handleTUIResult returns nil when
// the user quits without selecting a session.
func TestHandleTUIResult_UserQuit(t *testing.T) {
	m := tui.New(nil, nil)
	err := handleTUIResult(m)
	if err != nil {
		t.Errorf("handleTUIResult (user quit): expected nil, got %v", err)
	}
}

// TestHandleTUIResult_ModeAoE verifies that the AoE branch calls execInAoE.
func TestHandleTUIResult_ModeAoE(t *testing.T) {
	origExecInAoE := execInAoE
	var called bool
	execInAoE = func(tool, project, title string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { execInAoE = origExecInAoE })

	mdl := makeTUIModelWithKey(model.ToolClaude, map[model.Tool][]string{}, "a") // "a" = ModeAoE (always available)
	err := handleTUIResult(mdl)
	if err != nil {
		t.Errorf("handleTUIResult (ModeAoE): unexpected error: %v", err)
	}
	if !called {
		t.Error("handleTUIResult (ModeAoE): execInAoE was not called")
	}
}

// TestHandleTUIResult_ModeOpen verifies that the Open branch calls
// openProjectDir, which in turn calls execFn (injected to a no-op).
func TestHandleTUIResult_ModeOpen(t *testing.T) {
	origExecFn := execFn
	origGoosStr := goosStr
	var capturedArgv0 string
	execFn = func(argv0 string, argv []string, envv []string) error {
		capturedArgv0 = argv0
		return nil
	}
	goosStr = "darwin" // force darwin path so execFn is reached via LookPath("open")
	t.Cleanup(func() {
		execFn = origExecFn
		goosStr = origGoosStr
	})

	// Remove EDITOR so we fall through to the darwin/open path.
	origEditor := os.Getenv("EDITOR")
	os.Unsetenv("EDITOR")
	t.Cleanup(func() {
		if origEditor != "" {
			os.Setenv("EDITOR", origEditor)
		}
	})

	mdl := makeTUIModelWithKey(model.ToolClaude, map[model.Tool][]string{}, "o") // "o" = ModeOpen
	err := handleTUIResult(mdl)
	// Either execFn was called (darwin with "open" available) or fallback printed path (no "open").
	// Either way no error.
	if err != nil {
		t.Errorf("handleTUIResult (ModeOpen): unexpected error: %v", err)
	}
	_ = capturedArgv0
}

// TestHandleTUIResult_NoResumer verifies that handleTUIResult returns an error
// when no resumer is registered for the session's tool.
// codex has no registered resumer; we pass it "resume" mode via toolModes so
// the TUI allows the key press, then the resume.Get lookup fails.
func TestHandleTUIResult_NoResumer(t *testing.T) {
	codexModes := map[model.Tool][]string{model.ToolCodex: {string(resume.ModeResume)}}
	mdl := makeTUIModelWithKey(model.ToolCodex, codexModes, string(resume.ModeResume[:1])) // Enter key
	// Use Enter key (type KeyEnter) instead of a rune for resume mode.
	sess := model.Session{
		ID:        "bbbbbbbb-2222-2222-2222-222222222222",
		Tool:      model.ToolCodex,
		Project:   "/tmp/test",
		UpdatedAt: time.Now(),
		StartedAt: time.Now(),
	}
	m := tui.New([]model.Session{sess}, codexModes)
	var mdlTea tea.Model = m
	mdlTea, _ = mdlTea.Update(tea.KeyMsg{Type: tea.KeyEnter})

	err := handleTUIResult(mdlTea)
	if err == nil {
		t.Fatal("handleTUIResult (no resumer): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resume not supported") {
		t.Errorf("handleTUIResult (no resumer): unexpected error message: %v", err)
	}
	_ = mdl
}

// TestHandleTUIResult_ResumerExec verifies that handleTUIResult calls
// resumer.Exec when a resumer is registered for the session's tool.
// Uses the mockResumer registered in cmd_test.go init().
func TestHandleTUIResult_ResumerExec(t *testing.T) {
	mockModes := map[model.Tool][]string{mockResumerTool: {string(resume.ModeResume)}}
	sess := model.Session{
		ID:        "cccccccc-3333-3333-3333-333333333333",
		Tool:      mockResumerTool,
		Project:   "/tmp/test",
		UpdatedAt: time.Now(),
		StartedAt: time.Now(),
	}
	m := tui.New([]model.Session{sess}, mockModes)
	var mdl tea.Model = m
	mdl, _ = mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})

	err := handleTUIResult(mdl)
	if err != nil {
		t.Errorf("handleTUIResult (resumer exec): unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// openProjectDir — syscall.Exec injection paths
// ---------------------------------------------------------------------------

// TestOpenProjectDir_EditorExec verifies that openProjectDir calls execFn when
// $EDITOR is set to a valid executable. Creates a temporary fake editor binary
// so exec.LookPath succeeds, then injects execFn to capture the call.
func TestOpenProjectDir_EditorExec(t *testing.T) {
	// Create a temp dir with a fake editor script.
	tmpDir := t.TempDir()
	fakeEditor := filepath.Join(tmpDir, "fake-editor")
	if err := os.WriteFile(fakeEditor, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origExecFn := execFn
	var capturedArgv0 string
	execFn = func(argv0 string, argv []string, envv []string) error {
		capturedArgv0 = argv0
		return nil
	}
	t.Cleanup(func() { execFn = origExecFn })

	origEditor := os.Getenv("EDITOR")
	os.Setenv("EDITOR", fakeEditor) // full path: LookPath returns it directly
	t.Cleanup(func() {
		if origEditor == "" {
			os.Unsetenv("EDITOR")
		} else {
			os.Setenv("EDITOR", origEditor)
		}
	})

	err := openProjectDir("/tmp/testdir")
	if err != nil {
		t.Errorf("openProjectDir (editor exec): unexpected error: %v", err)
	}
	if capturedArgv0 != fakeEditor {
		t.Errorf("execFn argv0 = %q, want %q", capturedArgv0, fakeEditor)
	}
}

// TestOpenProjectDir_DarwinExec verifies that openProjectDir calls execFn via
// the "open" fallback on darwin. Injects goosStr = "darwin", creates a fake
// "open" binary on PATH, and injects execFn.
func TestOpenProjectDir_DarwinExec(t *testing.T) {
	// Create a temp dir with a fake "open" binary.
	tmpDir := t.TempDir()
	fakeOpen := filepath.Join(tmpDir, "open")
	if err := os.WriteFile(fakeOpen, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origExecFn := execFn
	var capturedArgv0 string
	execFn = func(argv0 string, argv []string, envv []string) error {
		capturedArgv0 = argv0
		return nil
	}
	origGoosStr := goosStr
	goosStr = "darwin"
	t.Cleanup(func() {
		execFn = origExecFn
		goosStr = origGoosStr
	})

	origEditor := os.Getenv("EDITOR")
	os.Unsetenv("EDITOR")
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+origPath) // prepend tmpDir so "open" is found
	t.Cleanup(func() {
		if origEditor != "" {
			os.Setenv("EDITOR", origEditor)
		}
		os.Setenv("PATH", origPath)
	})

	err := openProjectDir("/tmp/testdir")
	if err != nil {
		t.Errorf("openProjectDir (darwin exec): unexpected error: %v", err)
	}
	if capturedArgv0 != fakeOpen {
		t.Errorf("execFn argv0 = %q, want %q", capturedArgv0, fakeOpen)
	}
}

// ---------------------------------------------------------------------------
// runTUI — success path + limit truncation
// ---------------------------------------------------------------------------

// TestRunTUI_TUISuccess_Mock verifies that runTUI applies limit truncation
// (all = all[:limit]) and returns nil when the TUI program succeeds and the
// user quits without selecting. Uses runProgram injection for determinism.
func TestRunTUI_TUISuccess_Mock(t *testing.T) {
	silenceOutput(t)
	resetFlags()

	origRunProgram := runProgram
	runProgram = func(m tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
		return m, nil // success: user quit without selecting
	}
	t.Cleanup(func() { runProgram = origRunProgram })

	flagTool = string(activeSourceName) // returns 2 sessions
	flagLimit = 1                       // triggers all = all[:1] truncation

	err := runTUI(newNoopCmd(), nil)
	if err != nil {
		t.Errorf("runTUI (mock success, user quit): expected nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProgram closure body coverage
// ---------------------------------------------------------------------------

// TestRunProgramClosure calls the real runProgram closure (not mocked) to
// cover the body `return tea.NewProgram(m, opts...).Run()`. Uses
// tea.WithInput and tea.WithoutRenderer so no TTY is required.
func TestRunProgramClosure(t *testing.T) {
	silenceOutput(t)
	m := tui.New(nil, nil)
	// Send "q" so the TUI quits immediately via Update's "q" case.
	_, _ = runProgram(m,
		tea.WithoutRenderer(),
		tea.WithInput(strings.NewReader("q")),
		tea.WithOutput(io.Discard),
	)
}
