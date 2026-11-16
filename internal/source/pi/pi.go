// Package pi reads pi coding-agent sessions from its local JSONL files.
//
// pi stores one JSONL file per session under:
//
//	<agent-dir>/sessions/<encoded-cwd>/<timestamp>_<session-id>.jsonl
//
// where <agent-dir> is $PI_CODING_AGENT_DIR when set, else ~/.pi/agent.
// The first line is a session header; every following line is an append-only
// entry (message, model change, session info, …).
//
// See docs/references/pi-format.md.
package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/detect"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

func init() {
	source.Register(&piSource{})
}

// Package-level injection points; overridden in tests. Tests that override
// them must not call t.Parallel.
var (
	isToolRunning = detect.IsToolRunning
	nowFn         = time.Now
)

type piSource struct{}

func (s *piSource) Name() model.Tool { return model.ToolPi }

// agentDir returns pi's agent directory.
func agentDir(homeDir string) string {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(homeDir, ".pi", "agent")
}

// sessionsRoot returns the directory holding per-cwd session directories.
func sessionsRoot(homeDir string) string {
	return filepath.Join(agentDir(homeDir), "sessions")
}

// discoverSessionFiles returns every *.jsonl session file below root.
// Layout is one encoded-cwd directory per project; bare *.jsonl files directly
// under root are accepted too, for custom --session-dir layouts.
func discoverSessionFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pi sessions dir: %w", err)
	}

	var files []string
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if !entry.IsDir() {
			if strings.HasSuffix(entry.Name(), ".jsonl") {
				files = append(files, path)
			}
			continue
		}
		children, err := os.ReadDir(path)
		if err != nil {
			continue // unreadable directory: skip, don't fail the whole list
		}
		for _, child := range children {
			if child.IsDir() || !strings.HasSuffix(child.Name(), ".jsonl") {
				continue
			}
			files = append(files, filepath.Join(path, child.Name()))
		}
	}
	return files, nil
}

// List returns pi sessions ordered by most recent first.
// Messages are NOT populated.
func (s *piSource) List(opts source.ListOptions) ([]model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("list pi sessions: resolve home: %w", err)
	}

	files, err := discoverSessionFiles(sessionsRoot(home))
	if err != nil {
		return nil, fmt.Errorf("list pi sessions: %w", err)
	}
	return listFromFiles(files, opts), nil
}

// listFromFiles builds session metadata from discovered transcript files.
func listFromFiles(files []string, opts source.ListOptions) []model.Session {
	// One process probe per List call, not one per session.
	running := isToolRunning(string(model.ToolPi))
	now := nowFn()

	sessions := make([]model.Session, 0, len(files))
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		header, peek, err := peekSessionFile(path)
		if err != nil {
			continue // not a pi session file (or unreadable): skip
		}

		startedAt := parseISOTime(header.Timestamp)
		updatedAt := info.ModTime()
		if startedAt.IsZero() {
			startedAt = updatedAt
		}
		title := peek.Name
		if title == "" {
			title = peek.Preview
		}

		sess := model.Session{
			ID:        header.ID,
			Tool:      model.ToolPi,
			Project:   header.CWD,
			Title:     title,
			Model:     peek.Model,
			StartedAt: startedAt,
			UpdatedAt: updatedAt,
			Preview:   detect.Truncate(title, 120),
			// Same fallback as the other sources: tool process alive and the
			// transcript written within ActiveThreshold.
			Active: running && now.Sub(updatedAt) < detect.ActiveThreshold,
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
	return sessions
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

// Get returns a single pi session with full message history.
// Exact session IDs win; otherwise a unique ID prefix is accepted.
func (s *piSource) Get(sessionID string) (*model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get pi session: resolve home: %w", err)
	}

	files, err := discoverSessionFiles(sessionsRoot(home))
	if err != nil {
		return nil, fmt.Errorf("get pi session: %w", err)
	}

	path, header, err := resolveSessionFile(files, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get pi session %s: %w", sessionID, err)
	}
	if path == "" {
		return nil, nil
	}

	messages, meta, err := parseSessionFile(path)
	if err != nil {
		return nil, fmt.Errorf("parse pi session %s: %w", header.ID, err)
	}

	startedAt := parseISOTime(header.Timestamp)
	updatedAt := meta.UpdatedAt
	if info, statErr := os.Stat(path); statErr == nil && info.ModTime().After(updatedAt) {
		updatedAt = info.ModTime()
	}

	title := meta.Name
	preview := detect.Truncate(title, 120)
	if preview == "" {
		for _, m := range messages {
			if m.Role == model.RoleUser && m.Content != "" {
				title = detect.Truncate(m.Content, 120)
				preview = title
				break
			}
		}
	}

	return &model.Session{
		ID:        header.ID,
		Tool:      model.ToolPi,
		Project:   header.CWD,
		Title:     title,
		Model:     meta.Model,
		StartedAt: startedAt,
		UpdatedAt: updatedAt,
		Active:    isToolRunning(string(model.ToolPi)) && nowFn().Sub(updatedAt) < detect.ActiveThreshold,
		Messages:  messages,
		Preview:   preview,
	}, nil
}

// resolveSessionFile finds a session file by exact ID, then by unique ID
// prefix. It returns an empty path when nothing matches.
func resolveSessionFile(files []string, sessionID string) (string, *piHeader, error) {
	type candidate struct {
		path   string
		header *piHeader
	}
	var (
		exact    *candidate
		prefixes []candidate
	)
	for _, path := range files {
		header, err := readSessionHeader(path)
		if err != nil {
			continue
		}
		switch {
		case header.ID == sessionID:
			exact = &candidate{path: path, header: header}
		case strings.HasPrefix(header.ID, sessionID):
			prefixes = append(prefixes, candidate{path: path, header: header})
		}
	}

	if exact != nil {
		return exact.path, exact.header, nil
	}
	switch len(prefixes) {
	case 0:
		return "", nil, nil
	case 1:
		return prefixes[0].path, prefixes[0].header, nil
	default:
		ids := make([]string, len(prefixes))
		for i, c := range prefixes {
			ids[i] = c.header.ID
		}
		return "", nil, fmt.Errorf("ambiguous session prefix %q, matches: %s",
			sessionID, strings.Join(ids, ", "))
	}
}

// Search returns pi sessions whose message content contains the query
// (case-insensitive substring match).
func (s *piSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("search pi sessions: resolve home: %w", err)
	}
	files, err := discoverSessionFiles(sessionsRoot(home))
	if err != nil {
		return nil, fmt.Errorf("search pi sessions: %w", err)
	}

	sessions := listFromFiles(files, opts)
	pathByID := make(map[string]string, len(files))
	for _, path := range files {
		if header, err := readSessionHeader(path); err == nil {
			pathByID[header.ID] = path
		}
	}

	queryLower := strings.ToLower(query)
	var results []model.SearchResult
	for _, sess := range sessions {
		messages, _, err := parseSessionFile(pathByID[sess.ID])
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
