package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/psacconier/sessions/internal/detect"
	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/source"
)

func init() {
	source.Register(&claudeSource{})
}

type claudeSource struct{}

func (s *claudeSource) Name() model.Tool { return model.ToolClaude }

// claudeDir returns the path to ~/.claude.
func claudeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// historyPath returns the path to ~/.claude/history.jsonl.
func historyPath() (string, error) {
	dir, err := claudeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// sessionEntry holds deduplicated history data for a session.
type sessionEntry struct {
	SessionID string
	Project   string
	Display   string // first user prompt (from earliest entry)
	StartedAt time.Time
	UpdatedAt time.Time
}

// loadHistory reads history.jsonl and returns deduplicated session entries
// ordered by UpdatedAt descending.
func loadHistory() ([]sessionEntry, error) {
	hp, err := historyPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(hp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history: %w", err)
	}
	defer f.Close()

	// Track first and latest occurrence per session ID.
	type accumulator struct {
		entry     sessionEntry
		earliest  time.Time
		latest    time.Time
		firstDisp string
	}
	seen := make(map[string]*accumulator)

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

		ts := time.UnixMilli(he.Timestamp)

		acc, exists := seen[he.SessionID]
		if !exists {
			seen[he.SessionID] = &accumulator{
				entry: sessionEntry{
					SessionID: he.SessionID,
					Project:   he.Project,
					Display:   he.Display,
				},
				earliest:  ts,
				latest:    ts,
				firstDisp: he.Display,
			}
		} else {
			if ts.Before(acc.earliest) {
				acc.earliest = ts
				acc.firstDisp = he.Display
				// Also update project if the earliest entry has one
				if he.Project != "" {
					acc.entry.Project = he.Project
				}
			}
			if ts.After(acc.latest) {
				acc.latest = ts
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("warning: scanning history.jsonl: %v", err)
	}

	// Build result slice
	entries := make([]sessionEntry, 0, len(seen))
	for _, acc := range seen {
		acc.entry.StartedAt = acc.earliest
		acc.entry.UpdatedAt = acc.latest
		acc.entry.Display = acc.firstDisp
		entries = append(entries, acc.entry)
	}

	// Sort by UpdatedAt descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})

	return entries, nil
}

// findSessionFile locates the JSONL file for a given session ID by globbing
// across project directories.
func findSessionFile(sessionID string) (string, error) {
	dir, err := claudeDir()
	if err != nil {
		return "", err
	}

	pattern := filepath.Join(dir, "projects", "*", sessionID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob session file: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}
	return matches[0], nil
}

// findSessionFileForProject locates the JSONL file for a given session ID
// within a specific project directory.
func findSessionFileForProject(projectPath, sessionID string) string {
	dir, err := claudeDir()
	if err != nil {
		return ""
	}
	dirName := projectDirName(projectPath)
	path := filepath.Join(dir, "projects", dirName, sessionID+".jsonl")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// sessionFileUpdatedAt returns the modification time of a session file,
// which serves as a more accurate UpdatedAt than history.jsonl.
func sessionFileUpdatedAt(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// List returns sessions ordered by most recent first.
// Messages are NOT populated.
//
// Two-pass strategy:
//  1. Load sessions from history.jsonl (the standard index).
//  2. Scan ~/.claude/projects/*/*.jsonl for orphan session files that are
//     NOT in history.jsonl (e.g., sessions started from Cursor's embedded
//     Claude Code or other contexts that skip the history index).
func (s *claudeSource) List(opts source.ListOptions) ([]model.Session, error) {
	entries, err := loadHistory()
	if err != nil {
		return nil, fmt.Errorf("list claude sessions: %w", err)
	}

	// Track seen session IDs to avoid duplicates in the orphan scan.
	seenIDs := make(map[string]bool, len(entries))

	var sessions []model.Session

	// --- Pass 1: history.jsonl entries ---
	for _, entry := range entries {
		seenIDs[entry.SessionID] = true

		// Find the session file
		var sessionFilePath string
		if entry.Project != "" {
			sessionFilePath = findSessionFileForProject(entry.Project, entry.SessionID)
		}
		if sessionFilePath == "" {
			var err error
			sessionFilePath, err = findSessionFile(entry.SessionID)
			if err != nil {
				log.Printf("warning: finding session file for %s: %v", entry.SessionID, err)
			}
		}

		// Refine UpdatedAt from file modification time
		updatedAt := entry.UpdatedAt
		if sessionFilePath != "" {
			if modTime, ok := sessionFileUpdatedAt(sessionFilePath); ok {
				if modTime.After(updatedAt) {
					updatedAt = modTime
				}
			}
		}

		// Check active status
		active := false
		if sessionFilePath != "" {
			active = detect.IsSessionActive("claude", sessionFilePath)
		}

		// Apply filters
		if opts.Active && !active {
			continue
		}
		if opts.Since > 0 && time.Since(updatedAt) > opts.Since {
			continue
		}
		if opts.Project != "" && !strings.Contains(entry.Project, opts.Project) {
			continue
		}

		preview := detect.Truncate(entry.Display, 120)

		sess := model.Session{
			ID:        entry.SessionID,
			Tool:      model.ToolClaude,
			Project:   entry.Project,
			Title:     preview,
			StartedAt: entry.StartedAt,
			UpdatedAt: updatedAt,
			Active:    active,
			Preview:   preview,
		}

		// Try to extract branch and model from the session file header
		// without parsing the entire file: read just enough for metadata.
		if sessionFilePath != "" {
			if branch, mdl := peekSessionMetadata(sessionFilePath); branch != "" || mdl != "" {
				sess.Branch = branch
				sess.Model = mdl
			}
		}

		sessions = append(sessions, sess)
	}

	// --- Pass 2: orphan session files on disk ---
	orphans, err := findOrphanSessions(seenIDs)
	if err != nil {
		log.Printf("warning: scanning orphan sessions: %v", err)
	}

	for _, orphan := range orphans {
		updatedAt := orphan.UpdatedAt

		active := detect.IsSessionActive("claude", orphan.FilePath)

		// Apply filters
		if opts.Active && !active {
			continue
		}
		if opts.Since > 0 && time.Since(updatedAt) > opts.Since {
			continue
		}
		if opts.Project != "" && !strings.Contains(orphan.Project, opts.Project) {
			continue
		}

		sess := model.Session{
			ID:        orphan.SessionID,
			Tool:      model.ToolClaude,
			Project:   orphan.Project,
			Title:     orphan.Preview,
			StartedAt: orphan.UpdatedAt, // best we have
			UpdatedAt: updatedAt,
			Active:    active,
			Preview:   orphan.Preview,
			Branch:    orphan.Branch,
			Model:     orphan.Model,
		}

		sessions = append(sessions, sess)
	}

	// Sort all sessions (history + orphans) by UpdatedAt descending.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	// Apply limit after sorting the merged list.
	if opts.Limit > 0 && len(sessions) > opts.Limit {
		sessions = sessions[:opts.Limit]
	}

	return sessions, nil
}

// orphanSession holds data for a session file found on disk but not in history.jsonl.
type orphanSession struct {
	SessionID string
	Project   string
	FilePath  string
	UpdatedAt time.Time
	Preview   string
	Branch    string
	Model     string
}

// findOrphanSessions scans ~/.claude/projects/*/*.jsonl for session files
// whose IDs are not in the seenIDs set. For each orphan, it extracts
// metadata from the file without parsing it fully.
func findOrphanSessions(seenIDs map[string]bool) ([]orphanSession, error) {
	dir, err := claudeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(dir, "projects")
	pattern := filepath.Join(projectsDir, "*", "*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob orphan sessions: %w", err)
	}

	var orphans []orphanSession
	for _, match := range matches {
		sessionID := extractSessionIDFromPath(match)
		if sessionID == "" || seenIDs[sessionID] {
			continue
		}

		// Mark as seen to avoid duplicates across project dirs.
		seenIDs[sessionID] = true

		// Derive project path from the parent directory name.
		parentDirName := filepath.Base(filepath.Dir(match))
		project := projectPathFromDir(parentDirName)

		// Get file modification time for UpdatedAt.
		updatedAt := time.Time{}
		if modTime, ok := sessionFileUpdatedAt(match); ok {
			updatedAt = modTime
		}

		// Extract metadata (branch, model) from first few lines.
		branch, mdl := peekSessionMetadata(match)

		// Extract preview from first user message.
		preview := peekFirstUserMessage(match)

		orphans = append(orphans, orphanSession{
			SessionID: sessionID,
			Project:   project,
			FilePath:  match,
			UpdatedAt: updatedAt,
			Preview:   preview,
			Branch:    branch,
			Model:     mdl,
		})
	}

	return orphans, nil
}

// peekFirstUserMessage reads up to ~20 lines of a session file and returns
// the content of the first user message, truncated for use as a preview.
func peekFirstUserMessage(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)
	linesRead := 0
	for scanner.Scan() && linesRead < 20 {
		linesRead++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var sl sessionLine
		if err := jsonUnmarshalFast(line, &sl); err != nil {
			continue
		}

		if sl.Type != "user" {
			continue
		}

		// Parse the message payload to extract content.
		var payload messagePayload
		if err := jsonUnmarshalFast(sl.Message, &payload); err != nil {
			continue
		}

		content := extractContent(payload.Content)
		if content != "" {
			return detect.Truncate(content, 120)
		}
	}
	return ""
}

// peekSessionMetadata reads the first few lines of a session file to extract
// git branch and model without parsing the entire file.
func peekSessionMetadata(path string) (branch, mdl string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)
	linesRead := 0
	for scanner.Scan() && linesRead < 10 {
		linesRead++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var sl sessionLine
		if err := jsonUnmarshalFast(line, &sl); err != nil {
			continue
		}

		if branch == "" && sl.GitBranch != "" {
			branch = sl.GitBranch
		}
		if mdl == "" && sl.Type == "assistant" && sl.Model != "" {
			mdl = sl.Model
		}
		if branch != "" && mdl != "" {
			break
		}
	}
	return branch, mdl
}

// jsonUnmarshalFast is a thin wrapper for json.Unmarshal used by peekSessionMetadata.
func jsonUnmarshalFast(data []byte, v interface{}) error {
	return jsonUnmarshal(data, v)
}

var jsonUnmarshal = jsonUnmarshalImpl

func jsonUnmarshalImpl(data []byte, v interface{}) error {
	return json.Unmarshal(data, v) //nolint:musttag
}

// Get returns a single session with full message history.
// Supports prefix matching (first 8+ chars of the UUID).
func (s *claudeSource) Get(sessionID string) (*model.Session, error) {
	// Find the session file, supporting prefix match
	sessionFilePath, fullID, err := resolveSessionFile(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get claude session: %w", err)
	}
	if sessionFilePath == "" {
		return nil, nil
	}

	messages, mdl, branch, err := parseSessionFile(sessionFilePath)
	if err != nil {
		return nil, fmt.Errorf("parse claude session %s: %w", fullID, err)
	}

	// Determine project from the file path
	project := projectFromSessionPath(sessionFilePath)

	// Determine timestamps
	var startedAt, updatedAt time.Time
	if len(messages) > 0 {
		startedAt = messages[0].Timestamp
		updatedAt = messages[len(messages)-1].Timestamp
	}
	// Refine from file modification time
	if modTime, ok := sessionFileUpdatedAt(sessionFilePath); ok {
		if modTime.After(updatedAt) {
			updatedAt = modTime
		}
	}

	// Title from first user message
	title := ""
	preview := ""
	for _, m := range messages {
		if m.Role == model.RoleUser && m.Content != "" {
			title = detect.Truncate(m.Content, 120)
			preview = title
			break
		}
	}

	active := detect.IsSessionActive("claude", sessionFilePath)

	sess := &model.Session{
		ID:        fullID,
		Tool:      model.ToolClaude,
		Project:   project,
		Branch:    branch,
		Title:     title,
		Model:     mdl,
		StartedAt: startedAt,
		UpdatedAt: updatedAt,
		Active:    active,
		Messages:  messages,
		Preview:   preview,
	}

	return sess, nil
}

// resolveSessionFile finds the session file, supporting prefix matching.
// Returns (path, fullSessionID, error).
func resolveSessionFile(sessionID string) (string, string, error) {
	// First, try exact match
	path, err := findSessionFile(sessionID)
	if err != nil {
		return "", "", err
	}
	if path != "" {
		return path, sessionID, nil
	}

	// Try prefix match: glob for session files starting with the prefix
	dir, err := claudeDir()
	if err != nil {
		return "", "", err
	}

	pattern := filepath.Join(dir, "projects", "*", sessionID+"*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", "", fmt.Errorf("glob prefix match: %w", err)
	}

	if len(matches) == 0 {
		return "", "", nil
	}

	if len(matches) == 1 {
		fullID := extractSessionIDFromPath(matches[0])
		return matches[0], fullID, nil
	}

	// Multiple matches: ambiguous prefix
	var ids []string
	for _, m := range matches {
		ids = append(ids, extractSessionIDFromPath(m))
	}
	return "", "", fmt.Errorf("ambiguous session prefix %q, matches: %s", sessionID, strings.Join(ids, ", "))
}

// extractSessionIDFromPath extracts the session ID (filename without .jsonl) from a path.
func extractSessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

// projectFromSessionPath extracts the project path from a session file path.
// ~/.claude/projects/-Users-foo-bar/SESSION_ID.jsonl -> /Users/foo/bar
func projectFromSessionPath(sessionFilePath string) string {
	dir := filepath.Dir(sessionFilePath)
	dirName := filepath.Base(dir)
	if dirName == "." || dirName == "" {
		return ""
	}
	return projectPathFromDir(dirName)
}

// Search returns sessions containing the query string (case-insensitive substring match).
func (s *claudeSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	sessions, err := s.List(opts)
	if err != nil {
		return nil, fmt.Errorf("search claude sessions: %w", err)
	}

	queryLower := strings.ToLower(query)
	var results []model.SearchResult

	for _, sess := range sessions {
		// Find the session file
		var sessionFilePath string
		if sess.Project != "" {
			sessionFilePath = findSessionFileForProject(sess.Project, sess.ID)
		}
		if sessionFilePath == "" {
			sessionFilePath, _ = findSessionFile(sess.ID)
		}
		if sessionFilePath == "" {
			continue
		}

		messages, mdl, branch, err := parseSessionFile(sessionFilePath)
		if err != nil {
			log.Printf("warning: parsing session %s for search: %v", sess.ID, err)
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
			sess.Messages = nil // don't populate full messages in search results
			if mdl != "" {
				sess.Model = mdl
			}
			if branch != "" {
				sess.Branch = branch
			}
			results = append(results, model.SearchResult{
				Session: sess,
				Matches: matches,
			})
		}
	}

	return results, nil
}

// extractSnippet returns a ~targetLen character snippet around a match position.
func extractSnippet(content string, matchIdx, matchLen, targetLen int) string {
	if len(content) <= targetLen {
		return content
	}

	// Center the snippet around the match
	halfWindow := (targetLen - matchLen) / 2
	start := matchIdx - halfWindow
	end := matchIdx + matchLen + halfWindow

	if start < 0 {
		end -= start // shift right
		start = 0
	}
	if end > len(content) {
		start -= end - len(content) // shift left
		end = len(content)
	}
	if start < 0 {
		start = 0
	}

	snippet := content[start:end]

	// Add ellipsis markers
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
