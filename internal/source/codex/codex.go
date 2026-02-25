package codex

import (
	"bufio"
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
	source.Register(&codexSource{})
}

type codexSource struct{}

func (s *codexSource) Name() model.Tool { return model.ToolCodex }

// codexDir returns ~/.codex.
func codexDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// historyFilePath returns ~/.codex/history.jsonl.
func historyFilePath() (string, error) {
	dir, err := codexDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// sessionAccumulator holds intermediate state while deduplicating history entries.
type sessionAccumulator struct {
	sessionID string
	earliest  time.Time
	latest    time.Time
	text      string // from the earliest entry (used as preview/title)
}

// loadHistory reads history.jsonl and returns deduplicated session entries
// ordered by UpdatedAt descending.
func loadHistory() ([]*sessionAccumulator, error) {
	hp, err := historyFilePath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(hp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open codex history: %w", err)
	}
	defer f.Close()

	seen := make(map[string]*sessionAccumulator)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		he, err := parseHistoryLine(line)
		if err != nil {
			continue // skip malformed
		}
		if he.SessionID == "" {
			continue
		}

		ts := time.Unix(he.Ts, 0) // history.jsonl ts is Unix seconds

		acc, exists := seen[he.SessionID]
		if !exists {
			seen[he.SessionID] = &sessionAccumulator{
				sessionID: he.SessionID,
				earliest:  ts,
				latest:    ts,
				text:      he.Text,
			}
		} else {
			if ts.Before(acc.earliest) {
				acc.earliest = ts
				acc.text = he.Text // earliest entry has the original prompt
			}
			if ts.After(acc.latest) {
				acc.latest = ts
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("warning: scanning codex history.jsonl: %v", err)
	}

	accs := make([]*sessionAccumulator, 0, len(seen))
	for _, acc := range seen {
		accs = append(accs, acc)
	}

	// Sort by UpdatedAt descending
	sort.Slice(accs, func(i, j int) bool {
		return accs[i].latest.After(accs[j].latest)
	})

	return accs, nil
}

// List returns Codex sessions ordered by most recent first.
// Messages are NOT populated.
func (s *codexSource) List(opts source.ListOptions) ([]model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("list codex sessions: resolve home: %w", err)
	}

	accs, err := loadHistory()
	if err != nil {
		return nil, fmt.Errorf("list codex sessions: %w", err)
	}

	var sessions []model.Session

	for _, acc := range accs {
		sessionFilePath := findSessionFile(home, acc.sessionID)

		// Refine UpdatedAt from file modification time
		updatedAt := acc.latest
		if sessionFilePath != "" {
			if info, err := os.Stat(sessionFilePath); err == nil {
				if info.ModTime().After(updatedAt) {
					updatedAt = info.ModTime()
				}
			}
		}

		// Check active status
		active := false
		if sessionFilePath != "" {
			active = detect.IsSessionActive("codex", sessionFilePath)
		}

		// Apply filters
		if opts.Active && !active {
			continue
		}
		if opts.Since > 0 && time.Since(updatedAt) > opts.Since {
			continue
		}
		// Codex sessions have no project in history — skip Project filter unless
		// we can get cwd; apply post-parse in Search/Get.

		preview := detect.Truncate(acc.text, 120)

		sessions = append(sessions, model.Session{
			ID:        acc.sessionID,
			Tool:      model.ToolCodex,
			Title:     preview,
			StartedAt: acc.earliest,
			UpdatedAt: updatedAt,
			Active:    active,
			Preview:   preview,
		})
	}

	// Apply limit after sorting
	if opts.Limit > 0 && len(sessions) > opts.Limit {
		sessions = sessions[:opts.Limit]
	}

	return sessions, nil
}

// Get returns a single Codex session with full message history.
// Supports exact and prefix match on sessionID.
func (s *codexSource) Get(sessionID string) (*model.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get codex session: resolve home: %w", err)
	}

	sessionFilePath, fullID, err := resolveCodexSessionFile(home, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get codex session %s: %w", sessionID, err)
	}
	if sessionFilePath == "" {
		return nil, nil
	}

	messages, cwd, err := parseSessionFile(sessionFilePath)
	if err != nil {
		return nil, fmt.Errorf("parse codex session %s: %w", fullID, err)
	}

	// Determine timestamps from messages
	var startedAt, updatedAt time.Time
	if len(messages) > 0 {
		startedAt = messages[0].Timestamp
		updatedAt = messages[len(messages)-1].Timestamp
	}
	// Refine UpdatedAt from file modification time
	if info, err := os.Stat(sessionFilePath); err == nil {
		if info.ModTime().After(updatedAt) {
			updatedAt = info.ModTime()
		}
	}

	// Title and preview from first user message
	title := ""
	preview := ""
	for _, m := range messages {
		if m.Role == model.RoleUser && m.Content != "" {
			title = detect.Truncate(m.Content, 120)
			preview = title
			break
		}
	}

	active := detect.IsSessionActive("codex", sessionFilePath)

	// Apply project filter on cwd
	sess := &model.Session{
		ID:        fullID,
		Tool:      model.ToolCodex,
		Project:   cwd,
		Title:     title,
		StartedAt: startedAt,
		UpdatedAt: updatedAt,
		Active:    active,
		Messages:  messages,
		Preview:   preview,
	}

	return sess, nil
}

// resolveCodexSessionFile finds a Codex session file by exact or prefix match.
// Returns (path, fullSessionID, error).
func resolveCodexSessionFile(homeDir, sessionID string) (string, string, error) {
	// Try exact match first
	path := findSessionFile(homeDir, sessionID)
	if path != "" {
		return path, sessionID, nil
	}

	// Prefix match: glob for files ending with -<prefix>*.jsonl
	// Sessions live at sessions/YYYY/MM/DD/ (4 levels), so use 4 wildcards.
	pattern := filepath.Join(homeDir, ".codex", "sessions", "*", "*", "*", "*-"+sessionID+"*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", "", fmt.Errorf("glob codex prefix match: %w", err)
	}

	if len(matches) == 0 {
		return "", "", nil
	}

	if len(matches) == 1 {
		fullID := extractSessionIDFromPath(matches[0])
		return matches[0], fullID, nil
	}

	// Multiple matches — ambiguous prefix
	var ids []string
	for _, m := range matches {
		ids = append(ids, extractSessionIDFromPath(m))
	}
	return "", "", fmt.Errorf("ambiguous session prefix %q, matches: %s", sessionID, strings.Join(ids, ", "))
}

// Search returns Codex sessions whose message content contains the query
// (case-insensitive substring match).
func (s *codexSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("search codex sessions: resolve home: %w", err)
	}

	sessions, err := s.List(opts)
	if err != nil {
		return nil, fmt.Errorf("search codex sessions: %w", err)
	}

	queryLower := strings.ToLower(query)
	var results []model.SearchResult

	for _, sess := range sessions {
		sessionFilePath := findSessionFile(home, sess.ID)
		if sessionFilePath == "" {
			continue
		}

		messages, cwd, err := parseSessionFile(sessionFilePath)
		if err != nil {
			log.Printf("warning: parsing codex session %s for search: %v", sess.ID, err)
			continue
		}

		// Apply Project filter using cwd now that we have it
		if opts.Project != "" && !strings.Contains(cwd, opts.Project) {
			continue
		}

		var matches []model.SearchMatch
		for i, msg := range messages {
			contentLower := strings.ToLower(msg.Content)
			idx := strings.Index(contentLower, queryLower)
			if idx < 0 {
				continue
			}
			snippet := extractSnippet(msg.Content, idx, len(query), 200)
			matches = append(matches, model.SearchMatch{
				MessageIndex: i,
				Snippet:      snippet,
				Role:         msg.Role,
			})
		}

		if len(matches) > 0 {
			sess.Project = cwd
			sess.Messages = nil // don't include full messages in search results
			results = append(results, model.SearchResult{
				Session: sess,
				Matches: matches,
			})
		}
	}

	return results, nil
}

// extractSnippet returns a ~targetLen character snippet centred on a match.
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
	if start < 0 {
		start = 0
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
