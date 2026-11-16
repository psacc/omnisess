package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSession writes lines (joined with newlines) to a temp session file.
func writeSession(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const testHeader = `{"type":"session","version":3,"id":"01900000-0000-7000-8000-00000000000a","timestamp":"2026-01-02T03:04:05.000Z","cwd":"/Users/foo/prj/edge"}`

// ---------------------------------------------------------------------------
// Header / peek errors
// ---------------------------------------------------------------------------

func TestReadSessionHeader_Missing(t *testing.T) {
	if _, err := readSessionHeader(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("expected open error, got nil")
	}
}

func TestReadSessionHeader_Empty(t *testing.T) {
	if _, err := readSessionHeader(writeSession(t, "")); err == nil {
		t.Fatal("expected empty-file error, got nil")
	}
}

func TestReadSessionHeader_Malformed(t *testing.T) {
	if _, err := readSessionHeader(writeSession(t, "not-json")); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestReadSessionHeader_NotSession(t *testing.T) {
	if _, err := readSessionHeader(writeSession(t, `{"type":"other","id":"x"}`)); err == nil {
		t.Fatal("expected type error, got nil")
	}
	if _, err := readSessionHeader(writeSession(t, `{"type":"session"}`)); err == nil {
		t.Fatal("expected missing-id error, got nil")
	}
}

func TestReadSessionHeader_Valid(t *testing.T) {
	header, err := readSessionHeader(writeSession(t, testHeader))
	if err != nil {
		t.Fatalf("readSessionHeader: %v", err)
	}
	if header.ID != "01900000-0000-7000-8000-00000000000a" || header.CWD != "/Users/foo/prj/edge" {
		t.Errorf("header = %+v", header)
	}
}

func TestPeekSessionFile_Missing(t *testing.T) {
	if _, _, err := peekSessionFile(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("expected open error, got nil")
	}
}

func TestPeekSessionFile_Errors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"malformed header", "not-json"},
		{"wrong type", `{"type":"other","id":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := peekSessionFile(writeSession(t, tt.body)); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestPeekSessionFile_ScanError(t *testing.T) {
	path := writeSession(t, testHeader, strings.Repeat("x", 300))

	orig := maxLineBytes
	maxLineBytes = 200
	t.Cleanup(func() { maxLineBytes = orig })

	if _, _, err := peekSessionFile(path); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestPeekSessionFile_SkipsMalformedLines(t *testing.T) {
	path := writeSession(t,
		testHeader,
		"not-json",
		`{"type":"model_change","provider":"prov","modelId":"mdl"}`,
		`{"type":"message","message":{"role":"assistant","content":"not a user"}}`,
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
	)
	_, peek, err := peekSessionFile(path)
	if err != nil {
		t.Fatalf("peekSessionFile: %v", err)
	}
	if peek.Preview != "hello" {
		t.Errorf("Preview = %q, want hello", peek.Preview)
	}
	if peek.Model != "prov/mdl" {
		t.Errorf("Model = %q, want prov/mdl", peek.Model)
	}
}

// ---------------------------------------------------------------------------
// parseSessionFile edge cases
// ---------------------------------------------------------------------------

func TestParseSessionFile_OpenError(t *testing.T) {
	if _, _, err := parseSessionFile(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("expected open error, got nil")
	}
}

func TestParseSessionFile_HeaderErrors(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseSessionFile(empty); err == nil {
		t.Fatal("expected empty-file error, got nil")
	}
	if _, _, err := parseSessionFile(writeSession(t, "not-json")); err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if _, _, err := parseSessionFile(writeSession(t, `{"type":"other","id":"x"}`)); err == nil {
		t.Fatal("expected type error, got nil")
	}
}

func TestParseSessionFile_ScanError(t *testing.T) {
	path := writeSession(t, testHeader, strings.Repeat("x", 300))

	orig := maxLineBytes
	maxLineBytes = 200
	t.Cleanup(func() { maxLineBytes = orig })

	if _, _, err := parseSessionFile(path); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestParseSessionFile_EdgeEntries(t *testing.T) {
	path := writeSession(t,
		testHeader,
		"", // blank line
		"not-json",
		`{"type":"message","message":123}`, // message is not an object
		`{"type":"message","message":{"role":"user","content":"","timestamp":1767323048000}}`,
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"hello"}],"timestamp":1767323048000}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":""}],"provider":"prov","model":"mdl","timestamp":1767323049000}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"answer"}],"provider":"prov","model":"mdl","timestamp":1767323049000}}`,
		`{"type":"message","message":{"role":"bashExecution","command":"ls","output":"","timestamp":1767323050000}}`,
		`{"type":"message","message":{"role":"custom","content":"ignored","timestamp":1767323051000}}`,
		`{"type":"usage","id":"u1"}`,
	)

	messages, meta, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}
	if meta.Name != "" {
		t.Errorf("Name = %q, want empty", meta.Name)
	}
	// The assistant without a model_change supplies the session model.
	if meta.Model != "prov/mdl" {
		t.Errorf("Model = %q, want prov/mdl", meta.Model)
	}
	if len(messages) != 4 {
		t.Fatalf("got %d messages, want 4: %+v", len(messages), messages)
	}
	if messages[0].Content != "hello" {
		t.Errorf("msg0 = %+v", messages[0])
	}
	if messages[1].Content != "" {
		t.Errorf("assistant with empty text = %+v", messages[1])
	}
	if messages[2].Content != "answer" {
		t.Errorf("msg2 = %+v", messages[2])
	}
	if messages[3].Content != "$ ls" {
		t.Errorf("msg3 = %+v", messages[3])
	}
}

func TestParseSessionFile_BashWithoutOutput(t *testing.T) {
	path := writeSession(t,
		testHeader,
		`{"type":"message","message":{"role":"bashExecution","command":"ls","output":"","timestamp":1767323050000}}`,
	)
	messages, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "$ ls" {
		t.Errorf("messages = %+v", messages)
	}
}

func TestParseSessionFile_ToolResultBeforeCall(t *testing.T) {
	path := writeSession(t,
		testHeader,
		`{"type":"message","message":{"role":"toolResult","toolCallId":"unknown","content":[{"type":"text","text":"x"}],"isError":false,"timestamp":1767323048000}}`,
	)
	messages, _, err := parseSessionFile(path)
	if err != nil {
		t.Fatalf("parseSessionFile: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("got %d messages, want 0", len(messages))
	}
}

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

func TestDecodeBlocks(t *testing.T) {
	if got := decodeBlocks(nil); got != nil {
		t.Errorf("decodeBlocks(nil) = %v, want nil", got)
	}
	if got := decodeBlocks(json.RawMessage(`"a string"`)); got != nil {
		t.Errorf("decodeBlocks(string) = %v, want nil", got)
	}
	got := decodeBlocks(json.RawMessage(`[{"type":"text","text":"a"},{"type":"toolCall","id":"c"}]`))
	if len(got) != 2 || got[0].Text != "a" || got[1].ID != "c" {
		t.Errorf("decodeBlocks(array) = %+v", got)
	}
}

func TestContentText(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"string", `"hello"`, "hello"},
		{"blocks", `[{"type":"text","text":"a"},{"type":"image","data":"x"},{"type":"text","text":"b"}]`, "a\nb"},
		{"invalid", `123`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentText(json.RawMessage(tt.raw)); got != tt.want {
				t.Errorf("contentText(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMessageTime(t *testing.T) {
	if got, want := messageTime(1767323048000, "ignored"), time.UnixMilli(1767323048000).UTC(); !got.Equal(want) {
		t.Errorf("messageTime(millis) = %v, want %v", got, want)
	}
	if got, want := messageTime(0, "2026-01-02T03:04:05.000Z"), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC); !got.Equal(want) {
		t.Errorf("messageTime(iso) = %v, want %v", got, want)
	}
}

func TestParseISOTime(t *testing.T) {
	if got := parseISOTime(""); !got.IsZero() {
		t.Errorf("parseISOTime(empty) = %v, want zero", got)
	}
	if got := parseISOTime("not-a-time"); !got.IsZero() {
		t.Errorf("parseISOTime(invalid) = %v, want zero", got)
	}
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := parseISOTime("2026-01-02T03:04:05Z"); !got.Equal(want) {
		t.Errorf("parseISOTime(valid) = %v, want %v", got, want)
	}
}

func TestJoinModel(t *testing.T) {
	tests := []struct {
		provider, id, want string
	}{
		{"p", "m", "p/m"},
		{"", "m", "m"},
		{"p", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := joinModel(tt.provider, tt.id); got != tt.want {
			t.Errorf("joinModel(%q, %q) = %q, want %q", tt.provider, tt.id, got, tt.want)
		}
	}
}

func TestExtractSnippet(t *testing.T) {
	tests := []struct {
		name    string
		content string
		idx     int
		match   int
		target  int
		want    string
	}{
		{"short content unchanged", "hello", 0, 2, 200, "hello"},
		{"centred match", "0123456789012345678901234567890123456789", 20, 5, 10, "...890123456..."},
		{"match near start", "0123456789012345678901234567890123456789", 1, 1, 10, "012345678..."},
		{"match near end", "0123456789012345678901234567890123456789", 37, 1, 10, "...123456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSnippet(tt.content, tt.idx, tt.match, tt.target); got != tt.want {
				t.Errorf("extractSnippet() = %q, want %q", got, tt.want)
			}
		})
	}
}
