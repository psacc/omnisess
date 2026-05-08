package allowlist

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// errReader delivers its data on the first Read, then returns a hard error.
// Used to exercise the bufio.Scanner sc.Err() path in scanAllowlist.
type errReader struct {
	data []byte
	done bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("simulated read error")
	}
	r.done = true
	n := copy(p, r.data)
	return n, nil
}

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

func TestScanAllowlist_ScannerError(t *testing.T) {
	// errReader delivers only "foo\n" then errors; the scanner will see the error
	// on the next read attempt (with no data), causing sc.Err() to return non-nil.
	r := &errReader{data: []byte("foo\n")}
	_, err := scanAllowlist(r)
	if err == nil {
		t.Error("expected scanAllowlist to return error from broken reader, got nil")
	}
}

func TestLoad_ScannerError(t *testing.T) {
	// Override scanAllowlistFn to inject an error, exercising the Load error path.
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.txt")
	os.WriteFile(path, []byte("foo\n"), 0o644)

	sentinel := errors.New("injected scanner error")
	orig := scanAllowlistFn
	scanAllowlistFn = func(_ io.Reader) ([]string, error) {
		return nil, sentinel
	}
	t.Cleanup(func() { scanAllowlistFn = orig })

	_, err := Load(path)
	if err == nil || err != sentinel {
		t.Errorf("expected sentinel error, got %v", err)
	}
}
