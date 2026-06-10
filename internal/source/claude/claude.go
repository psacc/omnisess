package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/detect"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/procsnap"
	"github.com/psacc/omnisess/internal/source"
	"golang.org/x/sync/errgroup"
)

// peekConcurrencyCap bounds the per-session fan-out inside List and
// findOrphanSessions. min(NumCPU, 16) keeps us close to disk bandwidth on
// SSDs while avoiding pathological FD pressure on hosts with very high CPU
// counts. The bound is also low enough that pgrep — when invoked — does not
// become a fork-bomb (we cache the per-tool running result anyway, so pgrep
// fires at most once per List call).
const peekConcurrencyCap = 16

// isToolRunning is the function used to probe whether the `claude` CLI is
// currently running. It exists as an overridable var (rather than calling
// detect.IsToolRunning directly) so tests can force both the running=true
// and running=false branches deterministically. Without this seam, coverage
// of `peekHistoryEntry` / `peekOrphanFile` depends on whether the host
// running the test happens to have a `claude` process alive — which differs
// between developer macOS hosts and Linux CI runners, breaking the per-package
// 100% coverage gate.
var isToolRunning = detect.IsToolRunning

// snapshotFn is the function used to obtain the shared live-process
// snapshot (one memoized enumeration per CLI run). Overridable so tests can
// inject fake snapshots — and the registry-vs-fallback branches — on any
// platform.
var snapshotFn = procsnap.Cached

// liveSnapshot returns the shared process snapshot and whether it is usable.
// Any error (including procsnap.ErrUnsupported on non-macOS) means callers
// must use the mtime fallback heuristic instead.
func liveSnapshot() (procsnap.Snapshot, bool) {
	snap, err := snapshotFn()
	if err != nil {
		return procsnap.Snapshot{}, false
	}
	return snap, true
}

// peekConcurrency returns min(NumCPU, peekConcurrencyCap), clamped to at
// least 1. The implementation is split into a pure clampConcurrency helper
// so the boundary cases (n > cap, n < 1) are unit-testable without messing
// with runtime.GOMAXPROCS or runtime.NumCPU.
func peekConcurrency() int {
	return clampConcurrency(runtime.NumCPU())
}

func clampConcurrency(n int) int {
	if n > peekConcurrencyCap {
		return peekConcurrencyCap
	}
	if n < 1 {
		return 1
	}
	return n
}

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

// SessionFilePath returns the absolute on-disk JSONL path for the given
// Claude session ID, or an empty string if no matching file is found. The
// index package uses this to obtain the (mtime, size) cache key. Exported
// so other internal packages can locate the underlying file without
// duplicating the projects-glob heuristic.
func SessionFilePath(sessionID string) (string, error) {
	return findSessionFile(sessionID)
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

// historyPeek holds the per-session metadata gathered concurrently inside
// the pass-1 fan-out. Each slot is index-aligned with the input
// `entries []sessionEntry` slice so the post-fan-out sequential filter +
// builder loop produces deterministic output independent of completion
// order across goroutines.
type historyPeek struct {
	sessionFilePath string
	updatedAt       time.Time
	active          bool
	status          string
	branch          string
	model           string
}

// List returns sessions ordered by most recent first.
// Messages are NOT populated.
//
// Two-pass strategy:
//  1. Load sessions from history.jsonl (the standard index).
//  2. Scan ~/.claude/projects/*/*.jsonl for orphan session files that are
//     NOT in history.jsonl (e.g., sessions started from Cursor's embedded
//     Claude Code or other contexts that skip the history index).
//
// The per-session peek work (file lookup, mtime stat, header peek, active
// detection) inside each pass is fanned out under errgroup.SetLimit so we
// pay the disk-open syscalls in parallel up to peekConcurrency(). The
// `claude` process-running probe is cached once per call so we don't spawn
// N pgrep subprocesses.
func (s *claudeSource) List(opts source.ListOptions) ([]model.Session, error) {
	entries, err := loadHistory()
	if err != nil {
		return nil, fmt.Errorf("list claude sessions: %w", err)
	}

	// One shared process snapshot per CLI run: a session is active iff the
	// registry attributes a live PID to its exact session ID. The pgrep probe
	// is only needed for the mtime fallback when the snapshot is unavailable
	// (non-macOS or enumeration failure) — cached once per call either way.
	snap, snapOK := liveSnapshot()
	claudeRunning := false
	if !snapOK {
		claudeRunning = isToolRunning("claude")
	}

	// Track seen session IDs to avoid duplicates in the orphan scan.
	seenIDs := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seenIDs[entry.SessionID] = true
	}

	// --- Pass 1: history.jsonl entries — peek metadata in parallel ---
	peeks := make([]historyPeek, len(entries))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(peekConcurrency())
	for i := range entries {
		i := i
		entry := entries[i]
		g.Go(func() error {
			peeks[i] = peekHistoryEntry(entry, claudeRunning, snap, snapOK)
			return nil
		})
	}
	// peekHistoryEntry never returns an error — fan-out is error-free by
	// construction (per-session failures log + degrade gracefully). Discard
	// the Wait return so the unreachable error branch doesn't sit on the
	// coverage budget.
	_ = g.Wait()

	var sessions []model.Session
	for i, entry := range entries {
		p := peeks[i]

		// Apply filters
		if opts.Active && !p.active {
			continue
		}
		if opts.Since > 0 && time.Since(p.updatedAt) > opts.Since {
			continue
		}
		if !source.MatchesDate(p.updatedAt, opts.OnDate) {
			continue
		}
		if opts.Project != "" && !strings.Contains(entry.Project, opts.Project) {
			continue
		}
		if source.MatchesExclude(entry.Project, opts.ExcludeProjects) {
			continue
		}

		preview := detect.Truncate(entry.Display, 120)

		sess := model.Session{
			ID:        entry.SessionID,
			Tool:      model.ToolClaude,
			Project:   entry.Project,
			Title:     preview,
			StartedAt: entry.StartedAt,
			UpdatedAt: p.updatedAt,
			Active:    p.active,
			Status:    p.status,
			Preview:   preview,
			Branch:    p.branch,
			Model:     p.model,
		}

		sessions = append(sessions, sess)
	}

	// --- Pass 2: orphan session files on disk ---
	// findOrphanSessions calls claudeDir() which already succeeded in loadHistory above,
	// so its error is unreachable in practice — ignore it.
	orphans, _ := findOrphanSessions(seenIDs, claudeRunning, snap, snapOK)

	for _, orphan := range orphans {
		updatedAt := orphan.UpdatedAt

		// Apply filters
		if opts.Active && !orphan.Active {
			continue
		}
		if opts.Since > 0 && time.Since(updatedAt) > opts.Since {
			continue
		}
		if !source.MatchesDate(updatedAt, opts.OnDate) {
			continue
		}
		if opts.Project != "" && !strings.Contains(orphan.Project, opts.Project) {
			continue
		}
		if source.MatchesExclude(orphan.Project, opts.ExcludeProjects) {
			continue
		}

		sess := model.Session{
			ID:        orphan.SessionID,
			Tool:      model.ToolClaude,
			Project:   orphan.Project,
			Title:     orphan.Preview,
			StartedAt: orphan.UpdatedAt, // best we have
			UpdatedAt: updatedAt,
			Active:    orphan.Active,
			Status:    orphan.Status,
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

// peekHistoryEntry performs the per-session I/O work for a single history
// entry: locate the session file, refine UpdatedAt from file mtime, detect
// active status (using the pre-cached claudeRunning bool), and read the
// JSONL header for branch and model. Returns a fully-populated historyPeek;
// failures degrade gracefully (empty fields) and never error — matches
// the sequential predecessor's behavior of logging warnings via log.Printf.
//
// Safe to call from multiple goroutines: it touches only the local entry,
// the global file system (read-only), and log.Printf (already safe).
func peekHistoryEntry(entry sessionEntry, claudeRunning bool, snap procsnap.Snapshot, snapOK bool) historyPeek {
	out := historyPeek{updatedAt: entry.UpdatedAt}

	// Find the session file.
	if entry.Project != "" {
		out.sessionFilePath = findSessionFileForProject(entry.Project, entry.SessionID)
	}
	if out.sessionFilePath == "" {
		path, err := findSessionFile(entry.SessionID)
		if err != nil {
			log.Printf("warning: finding session file for %s: %v", entry.SessionID, err)
		}
		out.sessionFilePath = path
	}

	if out.sessionFilePath == "" {
		return out
	}

	// Refine UpdatedAt from file modification time.
	if modTime, ok := sessionFileUpdatedAt(out.sessionFilePath); ok {
		if modTime.After(out.updatedAt) {
			out.updatedAt = modTime
		}
	}

	// Active = the registry attributes a live PID to this session ID.
	// Fallback (snapshot unavailable): claude running + transcript-tree
	// mtime — claudeRunning is pre-cached so no per-session pgrep spawns.
	if snapOK {
		live, ok := snap.Lookup(entry.SessionID)
		out.active = ok
		out.status = live.Status
	} else if claudeRunning {
		out.active = detect.IsSessionTreeRecentlyModified(out.sessionFilePath, detect.ActiveThreshold)
	}

	// Try to extract branch and model from the session file header
	// without parsing the entire file: read just enough for metadata.
	out.branch, out.model = peekSessionMetadata(out.sessionFilePath)

	return out
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
	Active    bool
	Status    string
}

// findOrphanSessions scans ~/.claude/projects/*/*.jsonl for session files
// whose IDs are not in the seenIDs set. For each orphan, it extracts
// metadata from the file without parsing it fully.
//
// The per-orphan peek work (mtime stat, header peek, first-user-message
// peek, active-status check) is fanned out under errgroup with
// peekConcurrency() limit. claudeRunning is passed in pre-cached so we
// don't spawn N pgrep subprocesses for the orphan pass.
func findOrphanSessions(seenIDs map[string]bool, claudeRunning bool, snap procsnap.Snapshot, snapOK bool) ([]orphanSession, error) {
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

	// First pass: filter the glob output to only the orphans (cheap, no I/O).
	// We update seenIDs sequentially here so the dedup across project dirs is
	// deterministic regardless of completion order across goroutines.
	type todo struct {
		match     string
		sessionID string
		project   string
	}
	todos := make([]todo, 0, len(matches))
	for _, match := range matches {
		sessionID := extractSessionIDFromPath(match)
		if sessionID == "" || seenIDs[sessionID] {
			continue
		}
		seenIDs[sessionID] = true
		parentDirName := filepath.Base(filepath.Dir(match))
		todos = append(todos, todo{
			match:     match,
			sessionID: sessionID,
			project:   projectPathFromDir(parentDirName),
		})
	}

	// Second pass: fan out per-orphan I/O. Result slots are index-aligned with
	// todos so the output order is deterministic regardless of completion order.
	orphans := make([]orphanSession, len(todos))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(peekConcurrency())
	for i := range todos {
		i := i
		t := todos[i]
		g.Go(func() error {
			orphans[i] = peekOrphanFile(t.match, t.sessionID, t.project, claudeRunning, snap, snapOK)
			return nil
		})
	}
	// peekOrphanFile never returns an error; discard Wait() so the
	// unreachable error branch doesn't sit on the coverage budget.
	_ = g.Wait()
	return orphans, nil
}

// peekOrphanFile populates the orphanSession metadata for one file. Safe to
// call concurrently — touches only its own return value, the filesystem
// (read-only), and pgrep is never invoked since claudeRunning is pre-cached.
func peekOrphanFile(match, sessionID, project string, claudeRunning bool, snap procsnap.Snapshot, snapOK bool) orphanSession {
	out := orphanSession{
		SessionID: sessionID,
		Project:   project,
		FilePath:  match,
	}
	if modTime, ok := sessionFileUpdatedAt(match); ok {
		out.UpdatedAt = modTime
	}
	out.Branch, out.Model = peekSessionMetadata(match)
	out.Preview = peekFirstUserMessage(match)
	if snapOK {
		live, ok := snap.Lookup(sessionID)
		out.Active = ok
		out.Status = live.Status
	} else if claudeRunning {
		out.Active = detect.IsSessionTreeRecentlyModified(match, detect.ActiveThreshold)
	}
	return out
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
		if mdl == "" && sl.Type == "assistant" {
			switch {
			case sl.Model != "":
				mdl = sl.Model
			default:
				var p messagePayload
				if jsonUnmarshalFast(sl.Message, &p) == nil && p.Model != "" {
					mdl = p.Model
				}
			}
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

	active, status := activeStatus(fullID, sessionFilePath)

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
		Status:    status,
		Messages:  messages,
		Preview:   preview,
	}

	return sess, nil
}

// activeStatus resolves activeness for a single session outside the List
// fan-out: registry-correlated when the shared snapshot is available, the
// process+mtime heuristic otherwise.
func activeStatus(sessionID, sessionFilePath string) (bool, string) {
	if snap, ok := liveSnapshot(); ok {
		live, found := snap.Lookup(sessionID)
		return found, live.Status
	}
	return detect.IsSessionActive("claude", sessionFilePath), ""
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

	// Try prefix match: glob for session files starting with the prefix.
	// claudeDir() already succeeded in findSessionFile above, so ignore error.
	dir, _ := claudeDir()
	pattern := filepath.Join(dir, "projects", "*", sessionID+"*.jsonl")
	// filepath.Glob only errors on malformed patterns (e.g. "[").
	// sessionID passed here already survived findSessionFile's glob above,
	// so this error is unreachable.
	matches, _ := filepath.Glob(pattern)

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

// searchPeek holds per-session search output, index-aligned with the
// `sessions []model.Session` slice returned by List. A slot with hit=false
// is dropped during the post-fan-out sequential filter, preserving the
// session ranking produced by List.
type searchPeek struct {
	hit     bool
	session model.Session
	matches []model.SearchMatch
}

// Search returns sessions containing the query string (case-insensitive
// substring match).
//
// Strategy mirrors the parallel template established for List in #58: the
// per-session JSONL parse + match work is fanned out under errgroup with
// peekConcurrency() limit, into index-aligned slots. The sequential filter
// loop afterwards walks the slots in order, so the result ranking matches
// List's ordering deterministically regardless of goroutine completion
// order. Per-session failures (truncated/missing JSONL) are logged + skipped
// rather than aborting the whole search.
func (s *claudeSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	sessions, err := s.List(opts)
	if err != nil {
		return nil, fmt.Errorf("search claude sessions: %w", err)
	}

	queryLower := strings.ToLower(query)
	peeks := make([]searchPeek, len(sessions))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(peekConcurrency())
	for i := range sessions {
		i := i
		sess := sessions[i]
		g.Go(func() error {
			peeks[i] = searchSessionPeek(sess, query, queryLower)
			return nil
		})
	}
	// searchSessionPeek never returns an error — per-session failures log +
	// skip via the hit=false path. Discard Wait() so the unreachable error
	// branch doesn't sit on the coverage budget.
	_ = g.Wait()

	results := make([]model.SearchResult, 0, len(peeks))
	for _, p := range peeks {
		if !p.hit {
			continue
		}
		results = append(results, model.SearchResult{
			Session: p.session,
			Matches: p.matches,
		})
	}
	return results, nil
}

// searchSessionPeek is the per-session worker for Search. Safe to call from
// multiple goroutines: it touches only its own return value, the file system
// (read-only), and log.Printf (already safe). queryLower is precomputed by
// the caller so we don't lower-case the query N times.
func searchSessionPeek(sess model.Session, query, queryLower string) searchPeek {
	out := searchPeek{session: sess}

	var sessionFilePath string
	if sess.Project != "" {
		sessionFilePath = findSessionFileForProject(sess.Project, sess.ID)
	}
	if sessionFilePath == "" {
		sessionFilePath, _ = findSessionFile(sess.ID)
	}
	if sessionFilePath == "" {
		return out
	}

	messages, mdl, branch, err := parseSessionFile(sessionFilePath)
	if err != nil {
		log.Printf("warning: parsing session %s for search: %v", sess.ID, err)
		return out
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

	if len(matches) == 0 {
		return out
	}

	// Refine session metadata from the parsed file. We zero Messages so search
	// results don't carry the full transcript (callers use Get() for that).
	out.session.Messages = nil
	if mdl != "" {
		out.session.Model = mdl
	}
	if branch != "" {
		out.session.Branch = branch
	}
	out.hit = true
	out.matches = matches
	return out
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
	// start is always ≥ 0 here: start can only go negative in the block above if
	// end - len(content) > start, which requires len(content) < targetLen —
	// impossible because the early return guards len(content) <= targetLen.

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
