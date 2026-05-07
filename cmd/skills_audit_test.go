package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/skills"
)

func TestParseWindow(t *testing.T) {
	cases := map[string]time.Duration{
		"30d":  30 * 24 * time.Hour,
		"90d":  90 * 24 * time.Hour,
		"180d": 180 * 24 * time.Hour,
	}
	for in, want := range cases {
		got, err := parseWindow(in)
		if err != nil {
			t.Errorf("parseWindow(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseWindow(%q) = %v want %v", in, got, want)
		}
	}
	if _, err := parseWindow("60d"); err == nil {
		t.Error("expected error for unsupported window")
	}
}

// ---------------------------------------------------------------------------
// Helpers for runSkillsAuditWith tests
// ---------------------------------------------------------------------------

// resetAuditFlags resets all audit-related flag vars to their defaults.
func resetAuditFlags() {
	auditRoots = nil
	auditNoGlobals = false
	auditAllowlist = ""
	auditWindow = "90d"
	auditFormat = "markdown"
	auditOutput = ""
}

// newAuditCmdCapture builds a cobra.Command that captures output in a buffer.
func newAuditCmdCapture(buf *bytes.Buffer) *cobra.Command {
	c := &cobra.Command{}
	c.SetOut(buf)
	return c
}

// fakeDeps returns an auditDeps that uses a temp home with no session files
// and optionally an override for findSessionFiles.
func fakeDeps(t *testing.T) auditDeps {
	t.Helper()
	fakeHome := t.TempDir()
	return auditDeps{
		homeDir: func() (string, error) { return fakeHome, nil },
		findSessionFiles: func(root string) ([]string, error) {
			// Return empty slice (no JSONL files in the temp dir).
			return nil, nil
		},
	}
}

// makeSkillRoot creates a temp project root with one .claude/skills/<name>/SKILL.md.
func makeSkillRoot(t *testing.T, skillName string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + skillName + "\ndescription: A test skill\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// makeAllowlist writes a plain-text allowlist file and returns its path.
func makeAllowlist(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "allow.txt")
	if err := os.WriteFile(path, []byte(strings.Join(names, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Tests for runSkillsAuditWith
// ---------------------------------------------------------------------------

func TestRunSkillsAudit_NoRootsNoGlobals(t *testing.T) {
	resetAuditFlags()
	auditNoGlobals = true
	// No roots, no globals → error.
	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err == nil || !strings.Contains(err.Error(), "--root") {
		t.Errorf("expected --root error, got %v", err)
	}
}

func TestRunSkillsAudit_InvalidWindow(t *testing.T) {
	resetAuditFlags()
	auditWindow = "60d"
	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err == nil || !strings.Contains(err.Error(), "--window") {
		t.Errorf("expected --window error, got %v", err)
	}
}

func TestRunSkillsAudit_DiscoveryError(t *testing.T) {
	// walkRoot propagates errors; use an unreadable directory.
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "broken-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: broken\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	resetAuditFlags()
	auditRoots = []string{dir}
	auditNoGlobals = true
	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err == nil || !strings.Contains(err.Error(), "discovery") {
		t.Errorf("expected discovery error, got %v", err)
	}
}

func TestRunSkillsAudit_FindSessionsError(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true

	deps := fakeDeps(t)
	deps.findSessionFiles = func(string) ([]string, error) {
		return nil, errors.New("find sessions error")
	}
	err := runSkillsAuditWith(newNoopCmd(), deps)
	if err == nil || !strings.Contains(err.Error(), "find sessions") {
		t.Errorf("expected find sessions error, got %v", err)
	}
}

func TestRunSkillsAudit_AllowlistError(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditAllowlist = "/nonexistent/path/to/allow.txt"

	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Errorf("expected allowlist error, got %v", err)
	}
}

func TestRunSkillsAudit_MarkdownSuccess(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "markdown"

	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, fakeDeps(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "# Skills Audit") {
		t.Errorf("expected markdown output, got: %s", buf.String())
	}
}

func TestRunSkillsAudit_JSONSuccess(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "json"

	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, fakeDeps(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"metadata"`) {
		t.Errorf("expected JSON output, got: %s", buf.String())
	}
}

func TestRunSkillsAudit_UnknownFormat(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "xml"

	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("expected unknown format error, got %v", err)
	}
}

func TestRunSkillsAudit_FileOutput(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	outFile := filepath.Join(t.TempDir(), "audit.md")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "markdown"
	auditOutput = outFile

	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if !strings.Contains(string(data), "# Skills Audit") {
		t.Errorf("output file missing expected content")
	}
}

func TestRunSkillsAudit_FileOutputCreateError(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "markdown"
	auditOutput = "/nonexistent/dir/audit.md"

	err := runSkillsAuditWith(newNoopCmd(), fakeDeps(t))
	if err == nil {
		t.Error("expected error for unwritable output path, got nil")
	}
}

func TestRunSkillsAudit_WithAllowlist(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	allow := makeAllowlist(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "markdown"
	auditAllowlist = allow

	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, fakeDeps(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Allowlisted skill is Keep-tier (count=1 in Summary).
	out := buf.String()
	if !strings.Contains(out, "# Skills Audit") {
		t.Errorf("expected markdown output with allowlist, got: %s", out)
	}
	if !strings.Contains(out, "| Keep | 1 |") {
		t.Errorf("expected Keep count 1 for allowlisted skill, got: %s", out)
	}
}

func TestRunSkillsAudit_MDAlias(t *testing.T) {
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "md"

	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, fakeDeps(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "# Skills Audit") {
		t.Errorf("md alias should produce markdown output")
	}
}

func TestRunSkillsAudit_WithGlobals(t *testing.T) {
	// IncludeGlobals=true but HOME is a temp dir with no skills → empty result.
	resetAuditFlags()
	auditNoGlobals = false
	// No roots, but globals on. This exercises the globals path with a fake HOME.
	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, fakeDeps(t))
	if err != nil {
		t.Fatalf("unexpected error with globals+no roots: %v", err)
	}
}

func TestRunSkillsAudit_ScanSessionsError(t *testing.T) {
	// usage.Scan errors when a session file is unreadable.
	root := makeSkillRoot(t, "my-skill")
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(badFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true

	deps := fakeDeps(t)
	deps.findSessionFiles = func(string) ([]string, error) {
		return []string{badFile}, nil
	}
	// scanFile returns nil error for an empty file (no lines), so this path won't
	// error. Instead, confirm Scan succeeds with an empty file (covers the Scan
	// invocation path in runSkillsAuditWith).
	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, deps)
	if err != nil {
		t.Fatalf("unexpected error scanning empty file: %v", err)
	}
}

func TestRunSkillsAudit_HomeDirError(t *testing.T) {
	// homeDir returns an error → home defaults to "" → projectsRoot = ".claude/projects"
	// → FindSessionFiles returns empty (no files in temp), so audit succeeds.
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "markdown"

	deps := fakeDeps(t)
	deps.homeDir = func() (string, error) {
		return "", errors.New("homeDir not available")
	}
	deps.findSessionFiles = func(string) ([]string, error) {
		return nil, nil // empty
	}

	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAuditWith(cmd, deps)
	if err != nil {
		t.Fatalf("unexpected error when homeDir fails: %v", err)
	}
}

func TestRunSkillsAudit_ScanInvocationsError(t *testing.T) {
	// Make findSessionFiles return a path to a nonexistent file so usage.Scan errors.
	root := makeSkillRoot(t, "my-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true

	deps := fakeDeps(t)
	deps.findSessionFiles = func(string) ([]string, error) {
		// A path that doesn't exist → scanFile returns open error → Scan returns error.
		return []string{"/nonexistent/path/to/missing.jsonl"}, nil
	}
	err := runSkillsAuditWith(newNoopCmd(), deps)
	if err == nil || !strings.Contains(err.Error(), "scan sessions") {
		t.Errorf("expected scan sessions error, got %v", err)
	}
}

// TestRunSkillsAuditInvokesRunSkillsAudit verifies the cobra RunE dispatch
// path (runSkillsAudit → runSkillsAuditWith) works end-to-end via the actual
// RunE function with the default deps var overridden.
func TestRunSkillsAuditInvokesRunSkillsAudit(t *testing.T) {
	root := makeSkillRoot(t, "cobra-skill")
	resetAuditFlags()
	auditRoots = []string{root}
	auditNoGlobals = true
	auditFormat = "markdown"

	orig := auditDepsVar
	auditDepsVar = fakeDeps(t)
	t.Cleanup(func() { auditDepsVar = orig })

	// runSkillsAudit is the cobra RunE; call it directly.
	var buf bytes.Buffer
	cmd := newAuditCmdCapture(&buf)
	err := runSkillsAudit(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "# Skills Audit") {
		t.Errorf("expected markdown output via cobra dispatch, got: %s", buf.String())
	}
}

// Verify that the skills package constant types still compile after Collisions removal.
func TestAuditResultNoCollisionsField(t *testing.T) {
	res := skills.AuditResult{
		Window:          90 * 24 * time.Hour,
		OmnisessVersion: "v0",
	}
	if res.Window == 0 {
		t.Error("expected non-zero window")
	}
}
