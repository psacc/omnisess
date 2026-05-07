package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

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
	if user != 2 {
		t.Errorf("user invocations: got %d want 2", user)
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
	// Only the 2026-05-02 calendar invocation should remain.
	if len(got) != 1 {
		t.Errorf("got %d invocations, want 1", len(got))
	}
}
