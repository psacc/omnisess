package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// envIndexPath is the env var that overrides the default index file path.
const envIndexPath = "OMNISESS_INDEX_PATH"

// DefaultPath returns the resolved index file path:
//   - $OMNISESS_INDEX_PATH if set
//   - os.UserCacheDir() / "omnisess" / "index.sqlite" otherwise
//
// The caller must call Open() to actually create the parent directory and
// open the database. Returns an error only if os.UserCacheDir() fails,
// which on supported platforms requires $HOME (Unix) / %LOCALAPPDATA%
// (Windows) to be unset.
func DefaultPath() (string, error) {
	if p := os.Getenv(envIndexPath); p != "" {
		return p, nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "omnisess", "index.sqlite"), nil
}

// sqliteIndex is the SQLite-backed Index implementation.
type sqliteIndex struct {
	db   *sql.DB
	path string
}

// Open returns a ready Index pointing at `path`. The parent directory of the
// database file is created (with permission 0o700) if it does not exist;
// if it already exists at looser permissions it is re-chmodded to 0o700 so
// the index file is never world-readable. Schema migrations are applied
// automatically.
//
// To honour the env override (OMNISESS_INDEX_PATH) and default cache path,
// call DefaultPath() to resolve `path` before invoking Open.
func Open(path string) (Index, error) {
	parent := filepath.Dir(path)
	if parent != "" && parent != "." && parent != "/" {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return nil, fmt.Errorf("create index parent dir %s: %w", parent, err)
		}
		// Best-effort re-chmod to tighten perms if the dir pre-existed at a
		// looser mode. Failure here is non-fatal — the data is still in a
		// user-private location; we just couldn't enforce 0o700.
		_ = os.Chmod(parent, 0o700)
	}
	// sql.Open with a registered driver only errors on unknown driver name,
	// which is unreachable here ("sqlite" is registered via the modernc.org
	// blank import). Any real connection failure surfaces at migrate() time.
	//
	// _busy_timeout=5000: wait up to 5s for a writer lock before SQLITE_BUSY.
	// Required because Go's database/sql pool may open multiple connections
	// concurrently (e.g. bulk `index --all` doing lookup-then-write on
	// adjacent connections), and the default 0ms timeout surfaces BUSY
	// errors as soon as two connections contend.
	db, _ := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Tighten file permissions to 0o600 (defense in depth). The parent
	// directory is already 0o700, but if the file is ever moved or copied
	// out (backup, debug snapshot, …) its default umask-derived mode of
	// 0o644 would leak the cache. Apply to the WAL/SHM sidecars if they
	// already exist; otherwise leave it — SQLite recreates them on demand
	// and the next Open() will catch them. Best-effort throughout.
	_ = os.Chmod(path, 0o600)
	for _, suffix := range []string{"-wal", "-shm"} {
		side := path + suffix
		if _, err := os.Stat(side); err == nil {
			_ = os.Chmod(side, 0o600)
		}
	}
	return &sqliteIndex{db: db, path: path}, nil
}

func (s *sqliteIndex) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
