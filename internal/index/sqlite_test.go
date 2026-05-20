package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultPath_Env(t *testing.T) {
	t.Setenv(envIndexPath, "/tmp/override.sqlite")
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if p != "/tmp/override.sqlite" {
		t.Errorf("DefaultPath() = %q, want override", p)
	}
}

func TestDefaultPath_Default(t *testing.T) {
	t.Setenv(envIndexPath, "")
	t.Setenv("HOME", t.TempDir())
	// On Linux honour XDG_CACHE_HOME; on macOS UserCacheDir is OS-derived.
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	}
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if filepath.Base(p) != "index.sqlite" {
		t.Errorf("DefaultPath() base = %q, want index.sqlite", filepath.Base(p))
	}
	if !filepath.IsAbs(p) {
		t.Errorf("DefaultPath() = %q, want absolute", p)
	}
}

// TestDefaultPath_NoHome exercises the os.UserCacheDir error branch. On Unix
// UserCacheDir requires HOME (or XDG_CACHE_HOME on linux); unsetting both
// makes it fail.
func TestDefaultPath_NoHome(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("HOME-required test only meaningful on Unix")
	}
	t.Setenv(envIndexPath, "")
	t.Setenv("HOME", "")
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", "")
	}
	if _, err := DefaultPath(); err == nil {
		t.Errorf("expected error when HOME is unset")
	}
}

func TestOpen_CreatesParentWith0o700(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "nested", "omnisess")
	dbPath := filepath.Join(parent, "index.sqlite")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close()
	st, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("stat parent: %v", err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Errorf("parent perm = %v, want 0o700", st.Mode().Perm())
	}
}

func TestOpen_ReChmodsExistingParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "preexisting")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(parent, "index.sqlite")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close()
	st, _ := os.Stat(parent)
	if st.Mode().Perm() != 0o700 {
		t.Errorf("re-chmod failed: %v", st.Mode().Perm())
	}
}

func TestOpen_BadParentReturnsError(t *testing.T) {
	// Try to create the index inside a non-existing nested path under a file
	// we control. The mkdir step should fail.
	regularFile := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(regularFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Parent path lives inside the regular file → mkdir fails.
	dbPath := filepath.Join(regularFile, "nested", "index.sqlite")
	if _, err := Open(dbPath); err == nil {
		t.Errorf("expected Open() to fail when parent path is invalid")
	}
}

func TestOpen_BadDBPathReturnsError(t *testing.T) {
	// Provide an empty path; sql.Open succeeds but migrate fails.
	// modernc.org/sqlite treats "" as in-memory? Force a real failure with a
	// path that resolves to a directory.
	dir := t.TempDir()
	if _, err := Open(dir); err == nil {
		t.Errorf("expected Open() to fail when path is a directory")
	}
}

func TestCloseNilDB(t *testing.T) {
	s := &sqliteIndex{}
	if err := s.Close(); err != nil {
		t.Errorf("Close on zero-value sqliteIndex: %v", err)
	}
}

// TestMigrate_ClosedDB covers the apply-schema and insert-version error
// branches of migrate() by passing a closed *sql.DB.
func TestMigrate_ClosedDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	// Open a vanilla connection bypassing our Open() (which migrates).
	db, err := openRawSQLite(dbPath)
	if err != nil {
		t.Fatalf("openRawSQLite: %v", err)
	}
	_ = db.Close()
	if err := migrate(db); err == nil {
		t.Errorf("migrate() on closed db should fail")
	}
}

// openRawSQLite returns a *sql.DB without running migrations. Used by tests
// that need to drive migrate() directly.
func openRawSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path)
}

// TestOpen_DBFileMode0o600 verifies the DB file (and its WAL/SHM sidecars
// once SQLite materializes them) ends up at 0o600. Defense-in-depth against
// the cache file being copied out of its 0o700 parent directory and
// surfacing world-readable on another filesystem.
func TestOpen_DBFileMode0o600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits don't map cleanly on Windows")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.sqlite")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close()
	// Trigger at least one write so the WAL/SHM files exist.
	if _, err := idx.(*sqliteIndex).db.Exec(
		`INSERT INTO schema_version (version) VALUES (99)`,
	); err != nil {
		// schema_version may have UNIQUE; fall back to a harmless write.
		if _, err2 := idx.(*sqliteIndex).db.Exec(`SELECT 1`); err2 != nil {
			t.Fatalf("touch db: %v / %v", err, err2)
		}
	}
	st, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat %s: %v", dbPath, err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("db file perm = %v, want 0o600", st.Mode().Perm())
	}
	// Re-open the existing file to exercise the sidecar-chmod branch.
	// First close so the WAL/SHM are flushed and we can chmod them looser.
	_ = idx.Close()
	for _, suffix := range []string{"-wal", "-shm"} {
		side := dbPath + suffix
		if _, err := os.Stat(side); err == nil {
			_ = os.Chmod(side, 0o644)
		}
	}
	idx2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}
	defer idx2.Close()
	for _, suffix := range []string{"-wal", "-shm"} {
		side := dbPath + suffix
		st, err := os.Stat(side)
		if err != nil {
			continue // sidecar absent — acceptable
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s perm = %v, want 0o600", side, st.Mode().Perm())
		}
	}
}
