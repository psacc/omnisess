package index

import "time"

// Index is the interface implemented by the SQLite-backed transcript cache.
//
// Lifecycle: callers Open() once per process, then Close() on shutdown.
// EnsureSession is the lazy-populate entrypoint; QuerySession / QueryWindow
// are read-only.
type Index interface {
	// Close releases the underlying database handle.
	Close() error

	// EnsureSession indexes the given session if the source file's
	// (mtime, size, has_full_payloads) cache key has changed since the last
	// index run. Lazy populate. Pass includeFullPayloads=true to capture
	// arguments_json / result_json (privacy-sensitive).
	//
	// The caller provides the conversation ID (filename UUID) and the
	// already-parsed Session — Index does not depend on any source package.
	EnsureSession(claudeFilePath, conversationID string, includeFullPayloads, rebuild bool, sess *Session) error

	// QuerySession returns aggregated statistics for one session, or nil
	// (no error) if the conversation_id is not present in the index.
	QuerySession(conversationID string) (*SessionStats, error)

	// QueryWindow returns aggregated statistics across sessions whose
	// updated_at falls in [start, end). The tool argument is the
	// provider_name filter ('anthropic' for Claude); empty string = no filter.
	QueryWindow(start, end time.Time, providerName string) (*AggregateStats, error)
}
