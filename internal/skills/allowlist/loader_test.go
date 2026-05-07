package allowlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesPlainText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.txt")
	content := `# top comment
security-review        # inline comment
debugging

# blank line above
incident-response
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"security-review", "debugging", "incident-response"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestLoadEmptyPathReturnsEmpty(t *testing.T) {
	got, err := Load("")
	if err != nil {
		t.Fatalf("Load empty path: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestLoadMissingFileReturnsError(t *testing.T) {
	_, err := Load("/nonexistent/path/allow.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadDuplicatesDeduped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.txt")
	os.WriteFile(path, []byte("foo\nbar\nfoo\n"), 0o644)
	got, _ := Load(path)
	if len(got) != 2 {
		t.Errorf("expected 2 deduped entries, got %v", got)
	}
}
