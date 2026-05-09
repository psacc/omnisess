// Package copilot reads GitHub Copilot CLI sessions from
// ~/.copilot/session-state/<session-uuid>/.
//
// The CLI persists every session as a directory containing:
//   - events.jsonl       — append-only conversation event log
//   - vscode.metadata.json — workspace metadata (workspaceFolder = cwd)
//
// VS Code workspaceStorage formats (chatSessions/*.jsonl, state.vscdb) are
// intentionally NOT supported here: agent-mode sessions are not written
// to those files reliably, and supporting state.vscdb would require a
// SQLite dependency. The CLI session-state directory is the authoritative
// local source for Copilot session content.
package copilot

import (
	"fmt"
	"log"
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
	source.Register(&copilotSource{})
}

type copilotSource struct{}

func (s *copilotSource) Name() model.Tool { return model.ToolCopilot }

// eventsPath returns the events.jsonl path inside a session-state directory.
func eventsPath(sessionDir string) string {
	return filepath.Join(sessionDir, "events.jsonl")
}

// sessionIDFromDir extracts the session ID (the directory base name) from a
// session-state directory path.
func sessionIDFromDir(sessionDir string) string {
	return filepath.Base(sessionDir)
}

// List returns Copilot sessions ordered by most recent first.
// Messages are NOT populated.
func (s *copilotSource) List(opts source.ListOptions) ([]model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("list copilot sessions: resolve home: %w", err)
	}

	dirs, err := listSessionDirs(home)
	if err != nil {
		return nil, fmt.Errorf("list copilot sessions: %w", err)
	}

	var sessions []model.Session
	for _, dir := range dirs {
		ev := eventsPath(dir)

		// UpdatedAt: events.jsonl mtime (the file is append-only).
		var updatedAt time.Time
		if info, err := os.Stat(ev); err == nil {
			updatedAt = info.ModTime()
		}

		startedAt := peekFirstTimestamp(ev)
		if startedAt.IsZero() {
			startedAt = updatedAt
		}

		cwd := readMetadata(dir)
		active := detect.IsSessionActive("copilot", ev)

		// Apply filters.
		if opts.Active && !active {
			continue
		}
		if opts.Since > 0 && time.Since(updatedAt) > opts.Since {
			continue
		}
		if opts.Project != "" && !strings.Contains(cwd, opts.Project) {
			continue
		}
		if source.MatchesExclude(cwd, opts.ExcludeProjects) {
			continue
		}

		preview := detect.Truncate(peekFirstUserMessage(ev), 120)

		sessions = append(sessions, model.Session{
			ID:        sessionIDFromDir(dir),
			Tool:      model.ToolCopilot,
			Project:   cwd,
			Title:     preview,
			StartedAt: startedAt,
			UpdatedAt: updatedAt,
			Active:    active,
			Preview:   preview,
		})
	}

	// Sort by UpdatedAt descending.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(sessions) > opts.Limit {
		sessions = sessions[:opts.Limit]
	}

	return sessions, nil
}

// Get returns a single Copilot session with full message history.
// Supports exact and prefix match on sessionID.
func (s *copilotSource) Get(sessionID string) (*model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get copilot session: resolve home: %w", err)
	}

	sessionDir, fullID, err := resolveSessionDir(home, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get copilot session %s: %w", sessionID, err)
	}
	if sessionDir == "" {
		return nil, nil
	}

	ev := eventsPath(sessionDir)
	messages, err := parseEvents(ev)
	if err != nil {
		return nil, fmt.Errorf("parse copilot session %s: %w", fullID, err)
	}

	var startedAt, updatedAt time.Time
	if len(messages) > 0 {
		startedAt = messages[0].Timestamp
		updatedAt = messages[len(messages)-1].Timestamp
	}
	if info, err := os.Stat(ev); err == nil {
		if info.ModTime().After(updatedAt) {
			updatedAt = info.ModTime()
		}
	}

	cwd := readMetadata(sessionDir)
	active := detect.IsSessionActive("copilot", ev)

	title := ""
	preview := ""
	for _, m := range messages {
		if m.Role == model.RoleUser && m.Content != "" {
			title = detect.Truncate(m.Content, 120)
			preview = title
			break
		}
	}

	return &model.Session{
		ID:        fullID,
		Tool:      model.ToolCopilot,
		Project:   cwd,
		Title:     title,
		StartedAt: startedAt,
		UpdatedAt: updatedAt,
		Active:    active,
		Messages:  messages,
		Preview:   preview,
	}, nil
}

// resolveSessionDir finds a session-state directory by exact or prefix match.
// Returns (path, fullSessionID, error). An empty path means no match.
func resolveSessionDir(homeDir, sessionID string) (string, string, error) {
	root := sessionStateDir(homeDir)

	// Exact match: sessionID is the directory name.
	exact := filepath.Join(root, sessionID)
	if _, err := os.Stat(filepath.Join(exact, "events.jsonl")); err == nil {
		return exact, sessionID, nil
	}

	// Prefix match: glob across direct children.
	pattern := filepath.Join(root, sessionID+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", "", fmt.Errorf("glob copilot session dirs: %w", err)
	}

	// Keep only directories that actually contain events.jsonl.
	var valid []string
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(m, "events.jsonl")); err != nil {
			continue
		}
		valid = append(valid, m)
	}

	switch len(valid) {
	case 0:
		return "", "", nil
	case 1:
		return valid[0], filepath.Base(valid[0]), nil
	default:
		var ids []string
		for _, p := range valid {
			ids = append(ids, filepath.Base(p))
		}
		return "", "", fmt.Errorf("ambiguous session prefix %q, matches: %s", sessionID, strings.Join(ids, ", "))
	}
}

// Search returns Copilot sessions whose message content contains the query
// (case-insensitive substring match).
func (s *copilotSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("search copilot sessions: resolve home: %w", err)
	}

	sessions, err := s.List(opts)
	if err != nil {
		return nil, fmt.Errorf("search copilot sessions: %w", err)
	}

	queryLower := strings.ToLower(query)
	root := sessionStateDir(home)

	var results []model.SearchResult
	for _, sess := range sessions {
		ev := eventsPath(filepath.Join(root, sess.ID))
		messages, err := parseEvents(ev)
		if err != nil {
			log.Printf("warning: parsing copilot session %s for search: %v", sess.ID, err)
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
			results = append(results, model.SearchResult{
				Session: sess,
				Matches: matches,
			})
		}
	}

	return results, nil
}

// extractSnippet returns a ~targetLen character snippet centred on a match.
// Mirrors the helper in claude/codex sources.
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
