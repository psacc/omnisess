package copilot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/model"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// VS Code's User/workspaceStorage path varies by OS. Currently darwin-only.
// Linux: ~/.config/Code/User/workspaceStorage
// Windows: %APPDATA%\Code\User\workspaceStorage
// We only ship the darwin path for now — others are a follow-up.
const darwinVSCodeWorkspaceStorageRel = "Library/Application Support/Code/User/workspaceStorage"

// vsWorkspace describes one workspaceStorage entry.
type vsWorkspace struct {
	StorageDir  string // <home>/Library/.../workspaceStorage/<hash>
	WorkspaceID string // basename of StorageDir
	ProjectPath string // resolved from workspace.json's "folder" / "workspace" field
}

// chatSessionFile is the on-disk shape of one line in chatSessions/*.jsonl.
type chatSessionFile struct {
	V chatSession `json:"v"`
}

// chatSession is the inner payload of a chatSessions/*.jsonl line OR an
// element of state.vscdb's `interactive.sessions` JSON array.
type chatSession struct {
	SessionID       string        `json:"sessionId,omitempty"`
	CreationDate    int64         `json:"creationDate,omitempty"`    // ms epoch
	LastMessageDate int64         `json:"lastMessageDate,omitempty"` // ms epoch
	Requests        []chatRequest `json:"requests,omitempty"`
}

type chatRequest struct {
	Message  chatRequestMessage `json:"message"`
	Response []chatResponsePart `json:"response"`
}

type chatRequestMessage struct {
	Text string `json:"text"`
}

// chatResponsePart is a heterogeneous fragment. Two shapes are observed:
//   - {"kind":"markdownContent","content":{"value":"..."}}
//   - {"kind":"markdownVuln","content":{"value":"..."}}
//   - {"value":"..."} (fallback)
type chatResponsePart struct {
	Kind    string              `json:"kind,omitempty"`
	Value   string              `json:"value,omitempty"`
	Content chatResponseContent `json:"content,omitempty"`
}

type chatResponseContent struct {
	Value string `json:"value"`
}

// vscodeOSSupported lets tests force the unsupported-OS branch on platforms
// where runtime.GOOS is "darwin". Override via vscodeOSSupported = func() bool { return false }.
var vscodeOSSupported = func() bool { return runtime.GOOS == "darwin" }

// vscodeWorkspaceStorageDir returns the platform-appropriate workspaceStorage path,
// or "" if unsupported on the current OS.
func vscodeWorkspaceStorageDir(homeDir string) string {
	if !vscodeOSSupported() {
		return ""
	}
	return filepath.Join(homeDir, darwinVSCodeWorkspaceStorageRel)
}

// discoverVSCodeWorkspaces enumerates workspaceStorage entries on disk and
// resolves the project path for each by reading workspace.json.
// Workspaces with no workspace.json or a malformed one are skipped.
// Missing workspaceStorage dir returns nil, nil.
func discoverVSCodeWorkspaces(homeDir string) ([]vsWorkspace, error) {
	root := vscodeWorkspaceStorageDir(homeDir)
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read vscode workspaceStorage: %w", err)
	}
	var out []vsWorkspace
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ws := filepath.Join(root, e.Name())
		project := readWorkspaceJSON(filepath.Join(ws, "workspace.json"))
		if project == "" {
			// We still keep the workspace; some sessions may live in
			// state.vscdb / chatSessions even without a workspace.json.
			project = ""
		}
		out = append(out, vsWorkspace{
			StorageDir:  ws,
			WorkspaceID: e.Name(),
			ProjectPath: project,
		})
	}
	return out, nil
}

// readWorkspaceJSON returns a filesystem-style project path read from a
// VS Code workspace.json file. Returns "" if the file is missing or malformed.
//
// workspace.json typically looks like:
//
//	{"folder":"file:///Users/foo/prj/myapp"}
//	{"workspace":"file:///Users/foo/Library/.../bar.code-workspace"}
func readWorkspaceJSON(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var raw struct {
		Folder    string `json:"folder"`
		Workspace string `json:"workspace"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}
	pick := raw.Folder
	if pick == "" {
		pick = raw.Workspace
	}
	return decodeFileURI(pick)
}

// decodeFileURI converts "file:///Users/foo/bar" → "/Users/foo/bar".
// Returns the input unchanged if it isn't a file URI.
func decodeFileURI(s string) string {
	if !strings.HasPrefix(s, "file://") {
		return s
	}
	u, err := url.Parse(s)
	if err != nil {
		return strings.TrimPrefix(s, "file://")
	}
	return u.Path
}

// ---------------------------------------------------------------------------
// chatSessions/*.jsonl — VS Code "classic" chat
// ---------------------------------------------------------------------------

// listChatSessionsInWorkspace scans <ws>/chatSessions/*.jsonl and returns one
// `model.Session` per file. The last `v` line wins when a file has multiple
// snapshots (each line is a successive state of the same conversation).
func listChatSessionsInWorkspace(ws vsWorkspace) []model.Session {
	dir := filepath.Join(ws.StorageDir, "chatSessions")
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	var out []model.Session
	for _, p := range matches {
		sess, ok := loadChatSessionFile(p, ws)
		if ok {
			out = append(out, sess)
		}
	}
	return out
}

// loadChatSessionFile reads a single chatSessions/*.jsonl. The file may
// contain multiple lines, each carrying a `{"v": {...}}` snapshot. We use
// the LAST line that has a non-empty `requests` array as the canonical
// state; older snapshots represent earlier moments of the same chat.
func loadChatSessionFile(path string, ws vsWorkspace) (model.Session, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Session{}, false
	}
	var canonical chatSession
	have := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry chatSessionFile
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if len(entry.V.Requests) == 0 {
			continue
		}
		canonical = entry.V
		have = true
	}
	if !have {
		return model.Session{}, false
	}

	stem := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	id := stem
	if canonical.SessionID != "" {
		id = canonical.SessionID
	}

	started := msToTime(canonical.CreationDate)
	updated := msToTime(canonical.LastMessageDate)
	if updated.IsZero() {
		updated = started
	}
	if updated.IsZero() {
		if info, err := os.Stat(path); err == nil {
			updated = info.ModTime()
		}
	}
	if started.IsZero() {
		started = updated
	}

	preview := ""
	if len(canonical.Requests) > 0 {
		preview = strings.TrimSpace(canonical.Requests[0].Message.Text)
	}

	return model.Session{
		ID:        id,
		Tool:      model.ToolCopilot,
		Project:   ws.ProjectPath,
		Title:     truncatePreview(preview),
		StartedAt: started,
		UpdatedAt: updated,
		Preview:   truncatePreview(preview),
	}, true
}

// chatSessionMessages converts a chatSession into a list of model.Messages.
func chatSessionMessages(cs chatSession) []model.Message {
	var msgs []model.Message
	for _, r := range cs.Requests {
		userText := strings.TrimSpace(r.Message.Text)
		if userText != "" {
			msgs = append(msgs, model.Message{
				Role:    model.RoleUser,
				Content: userText,
			})
		}
		respText := joinResponseParts(r.Response)
		if respText != "" {
			msgs = append(msgs, model.Message{
				Role:    model.RoleAssistant,
				Content: respText,
			})
		}
	}
	return msgs
}

func joinResponseParts(parts []chatResponsePart) string {
	if len(parts) == 0 {
		return ""
	}
	var chunks []string
	for _, p := range parts {
		switch p.Kind {
		case "markdownContent", "markdownVuln":
			if p.Content.Value != "" {
				chunks = append(chunks, p.Content.Value)
			}
		default:
			if p.Value != "" {
				chunks = append(chunks, p.Value)
			} else if p.Content.Value != "" {
				chunks = append(chunks, p.Content.Value)
			}
		}
	}
	return strings.TrimSpace(strings.Join(chunks, ""))
}

// getChatSession finds a chat-session by ID across all VS Code workspaces.
// Match is by file stem first, then by sessionId field inside the JSONL.
// Returns (session, true) on hit; (zero, false) on miss.
func getChatSession(workspaces []vsWorkspace, id string) (model.Session, []model.Message, bool) {
	for _, ws := range workspaces {
		// Direct file lookup by stem.
		direct := filepath.Join(ws.StorageDir, "chatSessions", id+".jsonl")
		if _, err := os.Stat(direct); err == nil {
			if sess, cs, ok := loadChatSessionForGet(direct, ws); ok {
				return sess, chatSessionMessages(cs), true
			}
		}
	}
	// Fall back to scanning every chatSessions file for sessionId match.
	for _, ws := range workspaces {
		matches, err := filepath.Glob(filepath.Join(ws.StorageDir, "chatSessions", "*.jsonl"))
		if err != nil {
			continue
		}
		for _, p := range matches {
			sess, cs, ok := loadChatSessionForGet(p, ws)
			if !ok {
				continue
			}
			if sess.ID == id || cs.SessionID == id {
				return sess, chatSessionMessages(cs), true
			}
		}
	}
	return model.Session{}, nil, false
}

// loadChatSessionForGet returns both the session metadata and the raw
// chatSession body so the caller can extract messages.
func loadChatSessionForGet(path string, ws vsWorkspace) (model.Session, chatSession, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Session{}, chatSession{}, false
	}
	var canonical chatSession
	have := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry chatSessionFile
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if len(entry.V.Requests) == 0 {
			continue
		}
		canonical = entry.V
		have = true
	}
	if !have {
		return model.Session{}, chatSession{}, false
	}

	stem := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	id := stem
	if canonical.SessionID != "" {
		id = canonical.SessionID
	}

	started := msToTime(canonical.CreationDate)
	updated := msToTime(canonical.LastMessageDate)
	if updated.IsZero() {
		updated = started
	}
	if updated.IsZero() {
		if info, err := os.Stat(path); err == nil {
			updated = info.ModTime()
		}
	}
	if started.IsZero() {
		started = updated
	}

	preview := ""
	if len(canonical.Requests) > 0 {
		preview = strings.TrimSpace(canonical.Requests[0].Message.Text)
	}
	title := truncatePreview(preview)

	return model.Session{
		ID:        id,
		Tool:      model.ToolCopilot,
		Project:   ws.ProjectPath,
		Title:     title,
		StartedAt: started,
		UpdatedAt: updated,
		Preview:   title,
	}, canonical, true
}

// ---------------------------------------------------------------------------
// state.vscdb — legacy SQLite-backed sessions
// ---------------------------------------------------------------------------

// listVSCDBSessionsInWorkspace reads the `interactive.sessions` row of
// state.vscdb (if present) and returns one model.Session per JSON entry.
// Errors and missing files are silently skipped — these stores are sometimes
// empty for workspaces that have only used agent-mode chat.
func listVSCDBSessionsInWorkspace(ws vsWorkspace) []model.Session {
	dbPath := filepath.Join(ws.StorageDir, "state.vscdb")
	value, ok := readVSCDBInteractiveSessions(dbPath)
	if !ok {
		return nil
	}
	var raw []chatSession
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil
	}
	var out []model.Session
	for _, cs := range raw {
		if len(cs.Requests) == 0 {
			continue
		}
		sess := vscdbSessionToSession(cs, ws)
		out = append(out, sess)
	}
	return out
}

func vscdbSessionToSession(cs chatSession, ws vsWorkspace) model.Session {
	id := cs.SessionID
	if id == "" {
		// Fallback: synthesize a stable-ish ID from workspace + creation date.
		id = fmt.Sprintf("%s-%d", ws.WorkspaceID, cs.CreationDate)
	}
	started := msToTime(cs.CreationDate)
	updated := msToTime(cs.LastMessageDate)
	if updated.IsZero() {
		updated = started
	}
	preview := ""
	if len(cs.Requests) > 0 {
		preview = strings.TrimSpace(cs.Requests[0].Message.Text)
	}
	title := truncatePreview(preview)
	return model.Session{
		ID:        id,
		Tool:      model.ToolCopilot,
		Project:   ws.ProjectPath,
		Title:     title,
		StartedAt: started,
		UpdatedAt: updated,
		Preview:   title,
	}
}

// getVSCDBSession finds a session by ID across all VS Code workspaces' state.vscdb.
func getVSCDBSession(workspaces []vsWorkspace, id string) (model.Session, []model.Message, bool) {
	for _, ws := range workspaces {
		dbPath := filepath.Join(ws.StorageDir, "state.vscdb")
		value, ok := readVSCDBInteractiveSessions(dbPath)
		if !ok {
			continue
		}
		var raw []chatSession
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			continue
		}
		for _, cs := range raw {
			if len(cs.Requests) == 0 {
				continue
			}
			sess := vscdbSessionToSession(cs, ws)
			if sess.ID == id {
				return sess, chatSessionMessages(cs), true
			}
		}
	}
	return model.Session{}, nil, false
}

// readVSCDBInteractiveSessions reads the `interactive.sessions` value from a
// state.vscdb file. Returns ("", false) on any error or absence — this row
// is genuinely missing in many workspaces, so the caller treats it as a
// non-fatal "no data" rather than an error.
//
// sql.Open with the registered modernc.org/sqlite driver never fails on Open
// (the actual connection check happens on the first query), so its error is
// not handled here — same convention as internal/source/cursor.
func readVSCDBInteractiveSessions(dbPath string) (string, bool) {
	if _, err := os.Stat(dbPath); err != nil {
		return "", false
	}
	db, _ := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	defer db.Close() //nolint:errcheck

	var value string
	err := db.QueryRow(
		"SELECT value FROM ItemTable WHERE key='interactive.sessions'",
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// msToTime converts a millisecond epoch (as VS Code records dates) to a UTC
// time.Time. Zero / negative input returns the zero value.
func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// truncatePreview is a tiny wrapper around the standard 120-char preview
// length so the call sites stay tidy.
func truncatePreview(s string) string {
	const limit = 120
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", ""))
	if len(s) <= limit {
		return s
	}
	return s[:limit-3] + "..."
}
