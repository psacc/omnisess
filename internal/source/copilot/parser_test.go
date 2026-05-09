package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// Fixture session IDs (the directory base names under testdata/).
const (
	fixtureSessionID  = "session-aabbccdd-1234-5678-9abc-def012345678"
	fixtureSessionID2 = "session-11223344-aaaa-bbbb-cccc-ddddeeeeffff"
	fixtureRoot       = "testdata"
)

// setupFakeHome builds a minimal ~/.copilot/session-state layout in a temp
// dir by copying both fixture session directories under it. Returns the
// home directory path and the events.jsonl path of the first fixture.
func setupFakeHome(t *testing.T) (homeDir, eventsPath string) {
	t.Helper()
	home := t.TempDir()
	stateDir := filepath.Join(home, ".copilot", "session-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir session-state: %v", err)
	}

	for _, id := range []string{fixtureSessionID, fixtureSessionID2} {
		src := filepath.Join(fixtureRoot, id)
		dst := filepath.Join(stateDir, id)
		if err := copyDir(src, dst); err != nil {
			t.Fatalf("copy %s: %v", id, err)
		}
	}

	eventsPath = filepath.Join(stateDir, fixtureSessionID, "events.jsonl")
	return home, eventsPath
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// listSessionDirs
// ---------------------------------------------------------------------------

func TestListSessionDirs_NoRoot(t *testing.T) {
	home := t.TempDir()
	dirs, err := listSessionDirs(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirs != nil {
		t.Errorf("expected nil for missing root, got %v", dirs)
	}
}

func TestListSessionDirs_ReadDirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".copilot", "session-state")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755) //nolint:errcheck

	_, err := listSessionDirs(home)
	if err == nil {
		t.Fatal("expected error for unreadable session-state dir, got nil")
	}
}

func TestListSessionDirs_FiltersNonDirsAndEmptyDirs(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".copilot", "session-state")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// A regular file under session-state — must be skipped.
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A dir without events.jsonl — must be skipped.
	if err := os.MkdirAll(filepath.Join(root, "no-events"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A dir with events.jsonl — must be returned.
	good := filepath.Join(root, "good-session")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := listSessionDirs(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d (%v)", len(dirs), dirs)
	}
	if filepath.Base(dirs[0]) != "good-session" {
		t.Errorf("got %q, want good-session", filepath.Base(dirs[0]))
	}
}

// ---------------------------------------------------------------------------
// readMetadata
// ---------------------------------------------------------------------------

func TestReadMetadata_Missing(t *testing.T) {
	got := readMetadata(t.TempDir())
	if got != "" {
		t.Errorf("expected empty for missing metadata, got %q", got)
	}
}

func TestReadMetadata_Malformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vscode.metadata.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readMetadata(dir)
	if got != "" {
		t.Errorf("expected empty for malformed metadata, got %q", got)
	}
}

func TestReadMetadata_Valid(t *testing.T) {
	dir := filepath.Join(fixtureRoot, fixtureSessionID)
	got := readMetadata(dir)
	if got != "/Users/testuser/prj/myproject" {
		t.Errorf("got %q, want /Users/testuser/prj/myproject", got)
	}
}

// ---------------------------------------------------------------------------
// peekFirstUserMessage
// ---------------------------------------------------------------------------

func TestPeekFirstUserMessage_NoFile(t *testing.T) {
	if got := peekFirstUserMessage("/nonexistent/file"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPeekFirstUserMessage_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := peekFirstUserMessage(path); got != "" {
		t.Errorf("expected empty for empty file, got %q", got)
	}
}

func TestPeekFirstUserMessage_SkipsAndReturnsFirstUser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := "\n" +
		`not json` + "\n" +
		`{"type":"tool.execution_start","timestamp":"2026-04-14T08:10:11.000Z","data":{"tool":"x"}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:12.453Z","data":"not an object"}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:12.500Z","data":{"content":"   "}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:13.000Z","data":{"content":"  hello there  "}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := peekFirstUserMessage(path)
	if got != "hello there" {
		t.Errorf("got %q, want %q", got, "hello there")
	}
}

func TestPeekFirstUserMessage_NoUserMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := `{"type":"assistant.message","timestamp":"2026-04-14T08:10:13.000Z","data":{"content":"hi"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := peekFirstUserMessage(path); got != "" {
		t.Errorf("expected empty when no user.message, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// peekFirstTimestamp
// ---------------------------------------------------------------------------

func TestPeekFirstTimestamp_NoFile(t *testing.T) {
	if got := peekFirstTimestamp("/nonexistent/file"); !got.IsZero() {
		t.Errorf("expected zero, got %v", got)
	}
}

func TestPeekFirstTimestamp_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := peekFirstTimestamp(path); !got.IsZero() {
		t.Errorf("expected zero for empty file, got %v", got)
	}
}

func TestPeekFirstTimestamp_SkipsAndReturns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := "\n" +
		`not json` + "\n" +
		`{"type":"user.message","timestamp":"","data":{"content":"x"}}` + "\n" +
		`{"type":"user.message","timestamp":"garbage","data":{"content":"x"}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:12.453Z","data":{"content":"x"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := peekFirstTimestamp(path)
	if got.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
	if got.Year() != 2026 {
		t.Errorf("year = %d, want 2026", got.Year())
	}
}

// ---------------------------------------------------------------------------
// parseEvents
// ---------------------------------------------------------------------------

func TestParseEvents_NoFile(t *testing.T) {
	_, err := parseEvents("/nonexistent/file")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseEvents_FixtureMergesAssistant(t *testing.T) {
	msgs, err := parseEvents(filepath.Join(fixtureRoot, fixtureSessionID, "events.jsonl"))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	// fixture: user, assistant, assistant (merged), user, assistant → 4 messages
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[0].Role != model.RoleUser || msgs[0].Content != "compare AGENTS.md with CLAUDE.md" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Errorf("msgs[1].Role = %q", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "Both files define agent behavior.") ||
		!strings.Contains(msgs[1].Content, "AGENTS.md is more concise.") {
		t.Errorf("msgs[1].Content not merged: %q", msgs[1].Content)
	}
	if msgs[2].Role != model.RoleUser {
		t.Errorf("msgs[2].Role = %q", msgs[2].Role)
	}
	if msgs[3].Role != model.RoleAssistant {
		t.Errorf("msgs[3].Role = %q", msgs[3].Role)
	}
}

func TestParseEvents_SkipsNoise(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := "\n" +
		`{not json` + "\n" +
		`{"type":"tool.execution_start","timestamp":"2026-04-14T08:10:11.000Z","data":{"tool":"x"}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:12.000Z","data":"not an object"}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:13.000Z","data":{"content":"   "}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-04-14T08:10:14.000Z","data":{"content":"hello"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, err := parseEvents(path)
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("got %q, want hello", msgs[0].Content)
	}
}

func TestParseEvents_AssistantMergeBlankTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// Second assistant chunk has no timestamp — merge should not zero out
	// the first chunk's timestamp. Both chunks are TrimSpace'd on read,
	// so concatenation preserves the streaming-like joining behaviour.
	content := `{"type":"assistant.message","timestamp":"2026-04-14T08:10:14.000Z","data":{"content":"first"}}` + "\n" +
		`{"type":"assistant.message","timestamp":"","data":{"content":"second"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, err := parseEvents(path)
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(msgs))
	}
	if msgs[0].Content != "firstsecond" {
		t.Errorf("merged content = %q, want %q", msgs[0].Content, "firstsecond")
	}
	if msgs[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp preserved from first chunk")
	}
}

func TestParseEvents_ScanError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"type":"user.message","timestamp":"2026-04-14T08:10:14.000Z","data":{"content":"ok"}}` + "\n")
	big := make([]byte, 11*1024*1024)
	for i := range big {
		big[i] = 'x'
	}
	_, _ = f.Write(big)
	_, _ = f.WriteString("\n")
	f.Close()

	_, err = parseEvents(path)
	if err == nil {
		t.Fatal("expected scan error for oversized line, got nil")
	}
}

// ---------------------------------------------------------------------------
// mapEventRole
// ---------------------------------------------------------------------------

func TestMapEventRole(t *testing.T) {
	tests := []struct {
		in   string
		want model.Role
	}{
		{"user.message", model.RoleUser},
		{"assistant.message", model.RoleAssistant},
		{"tool.execution_start", ""},
		{"", ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := mapEventRole(tt.in); got != tt.want {
				t.Errorf("mapEventRole(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseCopilotTimestamp
// ---------------------------------------------------------------------------

func TestParseCopilotTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		isZero bool
	}{
		{"empty", "", true},
		{"garbage", "not-a-timestamp", true},
		{"RFC3339Nano", "2026-04-14T08:10:12.453Z", false},
		{"RFC3339", "2026-04-14T08:10:12Z", false},
		{"millis Z", "2026-04-14T08:10:12.000Z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCopilotTimestamp(tt.input)
			if tt.isZero {
				if !got.IsZero() {
					t.Errorf("got %v, want zero", got)
				}
				return
			}
			if got.IsZero() {
				t.Fatal("expected non-zero")
			}
			if got.Year() != 2026 {
				t.Errorf("year = %d, want 2026", got.Year())
			}
			_ = time.Now() // keep import alive even if every layout shifts
		})
	}
}
