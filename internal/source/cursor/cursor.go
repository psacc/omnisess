package cursor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/detect"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

func init() {
	source.Register(&cursorSource{})
}

type cursorSource struct{}

func (s *cursorSource) Name() model.Tool { return model.ToolCursor }

// List returns Cursor sessions ordered by most recent first.
// It uses the SQLite tracking DB as the primary metadata source,
// enriched with project path info from transcript file locations.
func (s *cursorSource) List(opts source.ListOptions) ([]model.Session, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cursor: resolve home dir: %w", err)
	}

	// Build a lookup from conversationID to transcript entry for project resolution.
	transcripts := listAllTranscripts(homeDir)
	transcriptMap := make(map[string]transcriptEntry, len(transcripts))
	for _, t := range transcripts {
		transcriptMap[t.ConversationID] = t
	}

	dbPath := filepath.Join(homeDir, ".cursor", "ai-tracking", "ai-code-tracking.db")
	summaries, err := readConversationSummaries(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cursor: warning: could not read tracking db: %v\n", err)
		summaries = nil
	}

	// Read chat store metadata for session names and creation times.
	chatMetas := readAllChatMeta(homeDir)

	// Track which conversation IDs we've seen from the DB.
	seen := make(map[string]bool, len(summaries))
	var sessions []model.Session

	cutoff := time.Time{}
	if opts.Since > 0 {
		cutoff = time.Now().Add(-opts.Since)
	}

	for _, sum := range summaries {
		seen[sum.ConversationID] = true

		sess := model.Session{
			ID:        sum.ConversationID,
			Tool:      model.ToolCursor,
			Title:     sum.Title,
			Summary:   sum.TLDR,
			Model:     sum.Model,
			StartedAt: sum.UpdatedAt, // best effort
			UpdatedAt: sum.UpdatedAt,
		}

		// Build preview from title or TLDR.
		if sum.Title != "" {
			sess.Preview = detect.Truncate(sum.Title, 120)
		} else if sum.TLDR != "" {
			sess.Preview = detect.Truncate(sum.TLDR, 120)
		}

		// Enrich with transcript location info.
		if t, ok := transcriptMap[sum.ConversationID]; ok {
			sess.Project = t.ProjectPath
			sess.Active = detect.IsSessionActive("cursor", t.FilePath)

			// Use file mod time for timestamps if DB is missing them.
			if info, err := os.Stat(t.FilePath); err == nil {
				if sess.UpdatedAt.IsZero() {
					sess.UpdatedAt = info.ModTime()
				}
				if sess.StartedAt.IsZero() {
					sess.StartedAt = info.ModTime()
				}
			}
		}

		if !matchesFilter(sess, opts, cutoff) {
			continue
		}

		sessions = append(sessions, sess)
	}

	// Add any transcript files not present in the DB (orphan transcripts).
	for _, t := range transcripts {
		if seen[t.ConversationID] {
			continue
		}

		var updatedAt time.Time
		var startedAt time.Time
		if info, err := os.Stat(t.FilePath); err == nil {
			updatedAt = info.ModTime()
			startedAt = updatedAt // best effort: use mtime as both
		}

		// Try to derive a preview from the first user message in the transcript.
		preview := ""
		if messages, err := parseTranscript(t.FilePath); err == nil && len(messages) > 0 {
			for _, m := range messages {
				if m.Role == model.RoleUser && strings.TrimSpace(m.Content) != "" {
					preview = detect.Truncate(m.Content, 120)
					if !m.Timestamp.IsZero() {
						startedAt = m.Timestamp
					}
					break
				}
			}
		}

		sess := model.Session{
			ID:        t.ConversationID,
			Tool:      model.ToolCursor,
			Project:   t.ProjectPath,
			StartedAt: startedAt,
			UpdatedAt: updatedAt,
			Preview:   preview,
			Active:    detect.IsSessionActive("cursor", t.FilePath),
		}

		// Enrich with chat store metadata (name, creation time).
		if cm, ok := chatMetas[t.ConversationID]; ok {
			if cm.Name != "" {
				sess.Title = cm.Name
				if sess.Preview == "" {
					sess.Preview = detect.Truncate(cm.Name, 120)
				}
			}
			if cm.CreatedAt > 0 {
				sess.StartedAt = chatMetaCreatedAt(cm)
			}
			if cm.Model != "" && cm.Model != "default" {
				sess.Model = cm.Model
			}
		}

		if !matchesFilter(sess, opts, cutoff) {
			continue
		}

		sessions = append(sessions, sess)
	}

	// Apply limit.
	if opts.Limit > 0 && len(sessions) > opts.Limit {
		sessions = sessions[:opts.Limit]
	}

	return sessions, nil
}

// Get returns a single session with full message history.
// Supports prefix matching: if sessionID is shorter than a full ID, it matches
// on the first 8+ characters.
func (s *cursorSource) Get(sessionID string) (*model.Session, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cursor: resolve home dir: %w", err)
	}

	// Find the transcript file — try exact match first, then prefix match.
	projectPath, transcriptPath := findTranscriptFile(homeDir, sessionID)

	if transcriptPath == "" && len(sessionID) >= 8 {
		// Prefix match: scan all transcripts.
		transcripts := listAllTranscripts(homeDir)
		for _, t := range transcripts {
			if strings.HasPrefix(t.ConversationID, sessionID) {
				projectPath = t.ProjectPath
				transcriptPath = t.FilePath
				sessionID = t.ConversationID // resolve to full ID
				break
			}
		}
	}

	if transcriptPath == "" {
		return nil, fmt.Errorf("cursor: session %q not found", sessionID)
	}

	messages, err := parseTranscript(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("cursor: parse transcript %s: %w", transcriptPath, err)
	}

	// Load metadata from DB if available.
	dbPath := filepath.Join(homeDir, ".cursor", "ai-tracking", "ai-code-tracking.db")
	summaries, _ := readConversationSummaries(dbPath)

	sess := &model.Session{
		ID:       sessionID,
		Tool:     model.ToolCursor,
		Project:  projectPath,
		Messages: messages,
		Active:   detect.IsSessionActive("cursor", transcriptPath),
	}

	// Set timestamp from file.
	if info, err := os.Stat(transcriptPath); err == nil {
		sess.UpdatedAt = info.ModTime()
	}

	// Enrich with DB metadata.
	for _, sum := range summaries {
		if sum.ConversationID == sessionID {
			sess.Title = sum.Title
			sess.Summary = sum.TLDR
			sess.Model = sum.Model
			if !sum.UpdatedAt.IsZero() {
				sess.UpdatedAt = sum.UpdatedAt
			}
			break
		}
	}

	// Derive preview from first user message if no title from DB.
	if sess.Title == "" {
		for _, m := range messages {
			if m.Role == model.RoleUser && strings.TrimSpace(m.Content) != "" {
				sess.Title = detect.Truncate(m.Content, 120)
				break
			}
		}
	}
	if sess.Preview == "" {
		if sess.Title != "" {
			sess.Preview = detect.Truncate(sess.Title, 120)
		} else if sess.Summary != "" {
			sess.Preview = detect.Truncate(sess.Summary, 120)
		}
	}

	return sess, nil
}

// Search returns sessions containing the query string in their transcripts.
func (s *cursorSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	sessions, err := s.List(opts)
	if err != nil {
		return nil, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cursor: resolve home dir: %w", err)
	}

	queryLower := strings.ToLower(query)
	var results []model.SearchResult

	// Build transcript map for file path resolution.
	transcripts := listAllTranscripts(homeDir)
	transcriptMap := make(map[string]transcriptEntry, len(transcripts))
	for _, t := range transcripts {
		transcriptMap[t.ConversationID] = t
	}

	for _, sess := range sessions {
		t, ok := transcriptMap[sess.ID]
		if !ok {
			continue
		}

		messages, err := parseTranscript(t.FilePath)
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

			snippet := extractSnippet(msg.Content, idx, len(query), 200)
			matches = append(matches, model.SearchMatch{
				MessageIndex: i,
				Snippet:      snippet,
				Role:         msg.Role,
			})
		}

		if len(matches) > 0 {
			results = append(results, model.SearchResult{
				Session: sess,
				Matches: matches,
			})
		}
	}

	return results, nil
}

// matchesFilter checks whether a session passes the list options filters.
func matchesFilter(sess model.Session, opts source.ListOptions, cutoff time.Time) bool {
	if !cutoff.IsZero() && sess.UpdatedAt.Before(cutoff) {
		return false
	}
	if opts.Project != "" && !strings.Contains(strings.ToLower(sess.Project), strings.ToLower(opts.Project)) {
		return false
	}
	if opts.Active && !sess.Active {
		return false
	}
	return true
}

// extractSnippet returns ~maxLen characters of context around a match at the given position.
func extractSnippet(content string, matchStart, matchLen, maxLen int) string {
	contextBefore := (maxLen - matchLen) / 2
	if contextBefore < 0 {
		contextBefore = 0
	}

	start := matchStart - contextBefore
	if start < 0 {
		start = 0
	}

	end := start + maxLen
	if end > len(content) {
		end = len(content)
		// Recalculate start to try to fill maxLen.
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	snippet := content[start:end]
	// Clean up whitespace for display.
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	snippet = strings.ReplaceAll(snippet, "\r", "")

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
