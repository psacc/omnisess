package usage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

// errScanFile is a sentinel error used to test Scan's error propagation path.
var errScanFile = errors.New("sentinel scan error")

// errReader delivers its data on the first Read call, then returns a hard error.
// Used to exercise bufio.Scanner's sc.Err() path in scanReader.
type errReader struct {
	data []byte
	done bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("simulated read error after first read")
	}
	r.done = true
	n := copy(p, r.data)
	return n, nil
}

func TestScanFileExtractsBothInvocationKinds(t *testing.T) {
	got, err := scanFile("testdata/sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var model, user int
	for _, inv := range got {
		switch inv.Kind {
		case skills.InvocationModel:
			model++
		case skills.InvocationUser:
			user++
		}
	}
	if model != 2 {
		t.Errorf("model invocations: got %d want 2", model)
	}
	if user != 3 {
		t.Errorf("user invocations: got %d want 3", user)
	}
}

func TestScanFileSkillNames(t *testing.T) {
	got, _ := scanFile("testdata/sample.jsonl")
	names := map[string]int{}
	for _, inv := range got {
		names[inv.SkillName]++
	}
	expected := map[string]int{
		"grill-me":        1,
		"agent-slack":     1,
		"figma:figma-use": 1,
		"calendar":        1,
		"array-skill":     1,
	}
	for name, want := range expected {
		if names[name] != want {
			t.Errorf("skill %q count: got %d want %d (all: %v)", name, names[name], want, names)
		}
	}
	if _, leaked := names["Read"]; leaked {
		t.Error("Read tool_use should not be counted as a skill invocation")
	}
}

func TestScanFileTimestampsParse(t *testing.T) {
	got, _ := scanFile("testdata/sample.jsonl")
	if len(got) == 0 {
		t.Fatal("no invocations")
	}
	for _, inv := range got {
		if inv.Timestamp.IsZero() {
			t.Errorf("zero timestamp on %+v", inv)
		}
	}
}

func TestFindSessionFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	// Top-level
	mustWriteFile(t, filepath.Join(dir, "proj-a", "session1.jsonl"), "")
	// Subagents subdir (the bug location)
	mustWriteFile(t, filepath.Join(dir, "proj-a", "subagents", "agent-1.jsonl"), "")
	// Deeper nesting (should still be picked up)
	mustWriteFile(t, filepath.Join(dir, "proj-b", "deep", "deeper", "session.jsonl"), "")
	// Non-jsonl ignored
	mustWriteFile(t, filepath.Join(dir, "proj-a", "ignore.txt"), "")

	got, err := FindSessionFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %d files, want 3: %v", len(got), got)
	}
}

// mustWriteFile creates parent dirs and writes the file.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanWindowFiltersBefore(t *testing.T) {
	cutoff := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	got, err := Scan(ScanOptions{
		Files: []string{"testdata/sample.jsonl"},
		Since: cutoff,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range got {
		if inv.Timestamp.Before(cutoff) {
			t.Errorf("invocation %+v before cutoff %v", inv, cutoff)
		}
	}
	// Both 2026-05-02 (calendar) and 2026-05-03 (array-skill) survive the cutoff.
	if len(got) != 2 {
		t.Errorf("got %d invocations, want 2", len(got))
	}
}

func TestScanFileHandlesArrayContent(t *testing.T) {
	got, _ := scanFile("testdata/sample.jsonl")
	var found bool
	for _, inv := range got {
		if inv.SkillName == "array-skill" && inv.Kind == skills.InvocationUser {
			found = true
		}
	}
	if !found {
		t.Errorf("array-content user invocation should be extracted")
	}
}

func TestParseTimestampLayouts(t *testing.T) {
	cases := []struct {
		input string
		zero  bool
	}{
		{"", true},
		{"garbage-not-a-ts", true},
		{"2026-05-07T10:00:00Z", false},           // RFC3339 (no millis)
		{"2026-05-07T10:00:00.000Z", false},       // millis variant
		{"2026-05-07T10:00:00.123456789Z", false}, // RFC3339Nano
	}
	for _, tc := range cases {
		got := parseTimestamp(tc.input)
		if tc.zero && !got.IsZero() {
			t.Errorf("parseTimestamp(%q): expected zero time, got %v", tc.input, got)
		}
		if !tc.zero && got.IsZero() {
			t.Errorf("parseTimestamp(%q): expected non-zero time, got zero", tc.input)
		}
	}
}

func TestScanMalformedJSONLSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	// Mix of bad and valid lines; only the valid skill line should produce output.
	content := "not-json-at-all\n" +
		`{"type":"assistant","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"my-skill"}}]}}` + "\n" +
		"{also bad}\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SkillName != "my-skill" {
		t.Errorf("expected 1 invocation of my-skill, got %+v", got)
	}
}

func TestScanZeroSince_NoFilter(t *testing.T) {
	// Since: zero value means no filter — all invocations should be returned.
	got, err := Scan(ScanOptions{
		Files: []string{"testdata/sample.jsonl"},
		Since: time.Time{}, // zero = no filter
	})
	if err != nil {
		t.Fatal(err)
	}
	// sample.jsonl has 5 invocations total (2 model + 3 user).
	if len(got) != 5 {
		t.Errorf("Scan with zero Since: got %d invocations, want 5", len(got))
	}
}

func TestFindSessionFiles_NonExistentRoot(t *testing.T) {
	// FindSessionFiles should not error on a nonexistent root (IsNotExist swallowed).
	got, err := FindSessionFiles("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Errorf("FindSessionFiles nonexistent root: expected nil error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestScanFile_OpenError(t *testing.T) {
	_, err := scanFile("/nonexistent/path/to/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestScan_ScanFileError(t *testing.T) {
	// Scan propagates a scanFile error upward.
	_, err := Scan(ScanOptions{Files: []string{"/nonexistent/missing.jsonl"}})
	if err == nil {
		t.Error("expected error from Scan with missing file, got nil")
	}
}

func TestScanFile_AssistantMsgUnmarshalError(t *testing.T) {
	// message field is not valid JSON for assistant type — unmarshal fails, line skipped.
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_assistant.jsonl")
	// type=assistant but message is a bare string, not a JSON object.
	content := `{"type":"assistant","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":"not-an-object"}` + "\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 invocations for bad assistant message, got %d", len(got))
	}
}

func TestScanFile_ToolInputUnmarshalError(t *testing.T) {
	// Skill tool_use with non-object input — json.Unmarshal into skillInput fails.
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_input.jsonl")
	content := `{"type":"assistant","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":{"content":[{"type":"tool_use","name":"Skill","input":"not-an-object"}]}}` + "\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 invocations when tool input is bad JSON, got %d", len(got))
	}
}

func TestScanFile_EmptySkillNameSkipped(t *testing.T) {
	// tool_use Skill with skill:"" should be skipped.
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_skill.jsonl")
	content := `{"type":"assistant","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":""}}]}}` + "\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 invocations for empty skill name, got %d", len(got))
	}
}

func TestScanFile_UserMsgUnmarshalError(t *testing.T) {
	// type=user but message is not valid JSON structure — Unmarshal fails, skipped.
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_user.jsonl")
	// message field is a JSON number — rawUserMsg.Content can't unmarshal from number.
	// Actually rawUserMsg wraps Content as interface{} so numbers unmarshal fine.
	// Instead: message is missing the closing brace to make JSON invalid.
	content := "{\"type\":\"user\",\"timestamp\":\"2026-05-07T10:00:00Z\",\"sessionId\":\"s1\",\"message\":INVALID}\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err) // the outer json.Unmarshal of rawLine should fail, causing skip
	}
	if len(got) != 0 {
		t.Errorf("expected 0 invocations for bad user line, got %d", len(got))
	}
}

func TestScanFile_UserMsgContentUnmarshalError(t *testing.T) {
	// type=user with message that is valid JSON at outer level but message field
	// contains content that cannot be unmarshalled to rawUserMsg.
	dir := t.TempDir()
	path := filepath.Join(dir, "user_bad_msg.jsonl")
	// message is an invalid JSON fragment (the message value is malformed JSON).
	// We need the outer rawLine to parse OK, but message.Unmarshal to fail.
	// message must be a RawMessage. If we set it to an invalid JSON token:
	content := "{\"type\":\"user\",\"timestamp\":\"2026-05-07T10:00:00Z\",\"sessionId\":\"s1\",\"message\":{}}\n"
	// {} is valid — Content=nil, switch default triggered → continue.
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// No command-name tags in empty content → 0 invocations.
	if len(got) != 0 {
		t.Errorf("expected 0 invocations, got %d", len(got))
	}
}

func TestScanFile_UserContentDefaultBranch(t *testing.T) {
	// User message where Content unmarshals to something other than string or []interface{}.
	// json.Unmarshal into interface{} gives: number → float64, bool → bool, null → nil.
	// Content as a JSON number triggers the "default: continue" branch.
	dir := t.TempDir()
	path := filepath.Join(dir, "user_number.jsonl")
	content := `{"type":"user","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":{"content":42}}` + "\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 invocations for numeric content, got %d", len(got))
	}
}

func TestScanFile_UserArrayNonMapItemSkipped(t *testing.T) {
	// User message with array content where one item is not a map[string]interface{}.
	// Mixed array: one valid text-block and one plain string.
	dir := t.TempDir()
	path := filepath.Join(dir, "user_array_nonmap.jsonl")
	// Array with: valid text block + a number (not a map).
	content := `{"type":"user","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":{"content":[{"text":"<command-name>/my-skill</command-name>"},42]}}` + "\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The valid text item should produce one invocation; the number is skipped.
	if len(got) != 1 || got[0].SkillName != "my-skill" {
		t.Errorf("expected 1 invocation of my-skill, got %+v", got)
	}
}

func TestScanFile_UserMsgUnmarshalErrorInner(t *testing.T) {
	// type=user where message is a valid JSON string (not an object), making
	// json.Unmarshal into rawUserMsg fail because you can't unmarshal a JSON string
	// into a struct.
	dir := t.TempDir()
	path := filepath.Join(dir, "user_string_msg.jsonl")
	// message field is a JSON string literal — rawLine.Message will be `"not-an-object"`,
	// and json.Unmarshal("not-an-object", &rawUserMsg{}) returns an error.
	content := `{"type":"user","timestamp":"2026-05-07T10:00:00Z","sessionId":"s1","message":"not-an-object"}` + "\n"
	mustWriteFile(t, path, content)
	got, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 invocations for non-object user message, got %d", len(got))
	}
}

func TestFindSessionFiles_NonIsNotExistError(t *testing.T) {
	// FindSessionFiles should propagate errors that are NOT os.IsNotExist.
	// Make an unreadable directory so WalkDir returns a permission error.
	dir := t.TempDir()
	unreadable := dir + "/unreadable"
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o755) })
	_, err := FindSessionFiles(unreadable)
	if err == nil {
		t.Error("expected error for unreadable root, got nil")
	}
}

func TestScanFile_ScannerError(t *testing.T) {
	// Exercise sc.Err() in scanReader by overriding scanReaderFn to use a
	// broken reader that triggers the bufio.Scanner error path.
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.jsonl")
	mustWriteFile(t, path, "{}\n")

	orig := scanReaderFn
	scanReaderFn = func(r io.Reader, p string) ([]skills.Invocation, error) {
		return nil, errScanFile
	}
	t.Cleanup(func() { scanReaderFn = orig })

	_, err := scanFile(path)
	if err == nil || err != errScanFile {
		t.Errorf("expected errScanFile, got %v", err)
	}
}

func TestScanReader_ScannerError(t *testing.T) {
	// Exercise sc.Err() in scanReader directly using a broken reader.
	// The reader delivers only "---broken\n" (a complete JSONL line) then errors.
	// Since the line is malformed JSON, it's skipped. Then sc.Scan() returns false
	// and sc.Err() is non-nil.
	r := &errReader{data: []byte("{\"type\":\"x\"}\n")}
	_, err := scanReader(r, "test-path")
	if err == nil {
		t.Error("expected scanReader to return error from broken reader, got nil")
	}
}
