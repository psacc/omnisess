// Package opencode reads OpenCode sessions from its local SQLite database.
//
// OpenCode persists every session in a single database:
//
//	<data-dir>/opencode/opencode.db
//
// where <data-dir> is $XDG_DATA_HOME when set, else ~/.local/share.
// Two schema generations are supported:
//
//   - v2: "session_v2" + "session_message". Current releases; parts of an
//     assistant turn are inlined in the message row's `data` JSON.
//   - legacy: "session" + "message" + "part". Pre-v2 releases; parts live in
//     the "part" table. Used only when "session_v2" is absent.
//
// See docs/references/opencode-format.md.
package opencode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/psacc/omnisess/internal/detect"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

func init() {
	source.Register(&opencodeSource{})
}

// Package-level injection points; overridden in tests. Tests that override
// them must not call t.Parallel.
var (
	isToolRunning = detect.IsToolRunning
	nowFn         = time.Now
)

type opencodeSource struct{}

func (s *opencodeSource) Name() model.Tool { return model.ToolOpenCode }

// dataDir returns the OpenCode data directory. OpenCode follows the XDG base
// directory spec on every platform, macOS included.
func dataDir(homeDir string) string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	return filepath.Join(homeDir, ".local", "share", "opencode")
}

// dbPath returns the path of OpenCode's SQLite database.
func dbPath(homeDir string) string {
	return filepath.Join(dataDir(homeDir), "opencode.db")
}

// openReadOnly opens the database without taking write locks: omnisess never
// mutates source data, and OpenCode keeps the file open in WAL mode.
func openReadOnly(path string) *sql.DB {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	u.RawQuery = q.Encode()
	// sql.Open with the registered modernc.org/sqlite driver never fails.
	db, _ := sql.Open("sqlite", u.String())
	return db
}

// sessionMeta is the subset of a session row omnisess needs for List.
type sessionMeta struct {
	ID        string
	Title     string
	Directory string
	Model     string
	StartedAt time.Time
	UpdatedAt time.Time
}

const (
	selectV2Sessions = `SELECT id, title, directory, model, time_created, time_updated
		FROM session_v2 ORDER BY time_updated DESC`
	selectLegacySessions = `SELECT id, title, directory, model, time_created, time_updated
		FROM session ORDER BY time_updated DESC`
)

// hasTable reports whether a table exists in the database.
func hasTable(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("inspect schema: %w", err)
	}
	return n > 0, nil
}

// useV2Schema reports whether the database uses the current (v2) schema.
func useV2Schema(db *sql.DB) (bool, error) {
	return hasTable(db, "session_v2")
}

// readSessionMetas returns every session row, most recently updated first.
// A database with neither schema returns no rows rather than an error.
func readSessionMetas(db *sql.DB) ([]sessionMeta, error) {
	v2, err := useV2Schema(db)
	if err != nil {
		return nil, err
	}
	if !v2 {
		legacy, legacyErr := hasTable(db, "session")
		if legacyErr != nil || !legacy {
			// A database with no session tables is not an OpenCode database.
			return nil, legacyErr
		}
	}
	query := selectLegacySessions
	if v2 {
		query = selectV2Sessions
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var metas []sessionMeta
	for rows.Next() {
		var (
			id, directory    string
			title, model     sql.NullString
			created, updated int64
		)
		if err := rows.Scan(&id, &title, &directory, &model, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		metas = append(metas, sessionMeta{
			ID:        id,
			Title:     title.String,
			Directory: directory,
			Model:     modelLabel(model.String),
			StartedAt: unixMilli(created),
			UpdatedAt: unixMilli(updated),
		})
	}
	return metas, rows.Err()
}

// modelLabel renders the session `model` JSON column as "provider/id".
func modelLabel(raw string) string {
	if raw == "" {
		return ""
	}
	var m struct {
		ID         string `json:"id"`
		ModelID    string `json:"modelID"`
		ProviderID string `json:"providerID"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	id := m.ID
	if id == "" {
		id = m.ModelID
	}
	switch {
	case m.ProviderID != "" && id != "":
		return m.ProviderID + "/" + id
	case id != "":
		return id
	default:
		return ""
	}
}

// List returns OpenCode sessions ordered by most recent first.
// Messages are NOT populated.
func (s *opencodeSource) List(opts source.ListOptions) ([]model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("list opencode sessions: resolve home: %w", err)
	}

	dbFile := dbPath(home)
	if _, err := os.Stat(dbFile); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list opencode sessions: stat db: %w", err)
	}

	db := openReadOnly(dbFile)
	defer db.Close()

	metas, err := readSessionMetas(db)
	if err != nil {
		return nil, fmt.Errorf("list opencode sessions: %w", err)
	}

	// One process probe per List call, not one per session.
	running := isToolRunning(string(model.ToolOpenCode))
	now := nowFn()

	sessions := make([]model.Session, 0, len(metas))
	for _, m := range metas {
		sess := model.Session{
			ID:        m.ID,
			Tool:      model.ToolOpenCode,
			Project:   m.Directory,
			Title:     m.Title,
			Model:     m.Model,
			StartedAt: m.StartedAt,
			UpdatedAt: m.UpdatedAt,
			Preview:   detect.Truncate(m.Title, 120),
			// Same fallback as the other sources: tool process alive and the
			// session written within ActiveThreshold.
			Active: running && now.Sub(m.UpdatedAt) < detect.ActiveThreshold,
		}
		if !passesFilters(sess, opts) {
			continue
		}
		sessions = append(sessions, sess)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(sessions) > opts.Limit {
		sessions = sessions[:opts.Limit]
	}
	return sessions, nil
}

// passesFilters applies the ListOptions filters shared by all sources.
func passesFilters(sess model.Session, opts source.ListOptions) bool {
	if opts.Active && !sess.Active {
		return false
	}
	if opts.Since > 0 && nowFn().Sub(sess.UpdatedAt) > opts.Since {
		return false
	}
	if !source.MatchesDate(sess.UpdatedAt, opts.OnDate) {
		return false
	}
	if opts.Project != "" && !strings.Contains(sess.Project, opts.Project) {
		return false
	}
	if source.MatchesExclude(sess.Project, opts.ExcludeProjects) {
		return false
	}
	return true
}

// Get returns a single OpenCode session with full message history.
// Exact IDs win; otherwise a unique ID prefix is accepted.
func (s *opencodeSource) Get(sessionID string) (*model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get opencode session: resolve home: %w", err)
	}

	dbFile := dbPath(home)
	if _, err := os.Stat(dbFile); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get opencode session: stat db: %w", err)
	}

	db := openReadOnly(dbFile)
	defer db.Close()

	v2, err := useV2Schema(db)
	if err != nil {
		return nil, fmt.Errorf("get opencode session: %w", err)
	}

	meta, err := resolveSession(db, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get opencode session %s: %w", sessionID, err)
	}
	if meta == nil {
		return nil, nil
	}

	var messages []model.Message
	if v2 {
		messages, err = readV2Messages(db, meta.ID)
	} else {
		messages, err = readLegacyMessages(db, meta.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("parse opencode session %s: %w", meta.ID, err)
	}

	startedAt := meta.StartedAt
	updatedAt := meta.UpdatedAt
	if len(messages) > 0 {
		if !messages[0].Timestamp.IsZero() {
			startedAt = messages[0].Timestamp
		}
		if last := messages[len(messages)-1].Timestamp; last.After(updatedAt) {
			updatedAt = last
		}
	}

	title := meta.Title
	preview := detect.Truncate(title, 120)
	if title == "" {
		for _, m := range messages {
			if m.Role == model.RoleUser && m.Content != "" {
				title = detect.Truncate(m.Content, 120)
				preview = title
				break
			}
		}
	}

	return &model.Session{
		ID:        meta.ID,
		Tool:      model.ToolOpenCode,
		Project:   meta.Directory,
		Title:     title,
		Model:     meta.Model,
		StartedAt: startedAt,
		UpdatedAt: updatedAt,
		Active:    isToolRunning(string(model.ToolOpenCode)) && nowFn().Sub(updatedAt) < detect.ActiveThreshold,
		Messages:  messages,
		Preview:   preview,
	}, nil
}

// resolveSession finds a session by exact ID, then by unique ID prefix.
func resolveSession(db *sql.DB, sessionID string) (*sessionMeta, error) {
	metas, err := readSessionMetas(db)
	if err != nil {
		return nil, err
	}
	for i := range metas {
		if metas[i].ID == sessionID {
			return &metas[i], nil
		}
	}

	var matches []sessionMeta
	for _, m := range metas {
		if strings.HasPrefix(m.ID, sessionID) {
			matches = append(matches, m)
		}
	}
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, fmt.Errorf("ambiguous session prefix %q, matches: %s",
			sessionID, strings.Join(ids, ", "))
	}
}

// Search returns OpenCode sessions whose message content contains the query
// (case-insensitive substring match).
func (s *opencodeSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("search opencode sessions: resolve home: %w", err)
	}
	dbFile := dbPath(home)
	if _, err := os.Stat(dbFile); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("search opencode sessions: stat db: %w", err)
	}
	db := openReadOnly(dbFile)
	defer db.Close()
	v2, err := useV2Schema(db)
	if err != nil {
		return nil, fmt.Errorf("search opencode sessions: %w", err)
	}

	sessions, err := s.List(opts)
	if err != nil {
		return nil, fmt.Errorf("search opencode sessions: %w", err)
	}

	queryLower := strings.ToLower(query)
	var results []model.SearchResult
	for _, sess := range sessions {
		var messages []model.Message
		if v2 {
			messages, err = readV2Messages(db, sess.ID)
		} else {
			messages, err = readLegacyMessages(db, sess.ID)
		}
		if err != nil {
			continue
		}

		var matches []model.SearchMatch
		for i, msg := range messages {
			contentLower := strings.ToLower(msg.Content)
			idx := strings.Index(contentLower, queryLower)
			if idx < 0 {
				continue
			}
			matches = append(matches, model.SearchMatch{
				MessageIndex: i,
				Snippet:      extractSnippet(msg.Content, idx, len(query), 200),
				Role:         msg.Role,
			})
		}
		if len(matches) > 0 {
			sess.Messages = nil
			results = append(results, model.SearchResult{Session: sess, Matches: matches})
		}
	}
	return results, nil
}

// unixMilli converts a millisecond epoch to a UTC time, treating 0 as unset.
func unixMilli(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// extractSnippet returns a ~targetLen character snippet centred on a match.
// Mirrors the helper in the other sources.
func extractSnippet(content string, matchIdx, matchLen, targetLen int) string {
	if len(content) <= targetLen {
		return content
	}

	halfWindow := (targetLen - matchLen) / 2
	start := matchIdx - halfWindow
	end := matchIdx + matchLen + halfWindow

	if start < 0 {
		end -= start
		start = 0
	}
	if end > len(content) {
		start -= end - len(content)
		end = len(content)
	}

	snippet := content[start:end]

	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
	}
	if end < len(content) {
		suffix = "..."
	}
	return prefix + snippet + suffix
}
