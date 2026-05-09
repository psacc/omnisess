package copilot

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"

	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

const (
	chatSessionID  = "chat-aaaa-1111-2222-3333-444455556666"
	vscdbSessionID = "vscdb-cccc-1111-2222-3333-444455556666"
)

// forceVSCodeOSSupported makes vscodeOSSupported return true for the test's
// duration. Tests that exercise darwin-only paths must call this so they pass
// on Linux CI; the production predicate is `runtime.GOOS == "darwin"`.
func forceVSCodeOSSupported(t *testing.T) {
	t.Helper()
	prev := vscodeOSSupported
	vscodeOSSupported = func() bool { return true }
	t.Cleanup(func() { vscodeOSSupported = prev })
}

// setupFakeVSCodeHome builds a temp HOME with one populated VS Code workspace.
// Returns the home path and the workspace storage dir for inspection.
func setupFakeVSCodeHome(t *testing.T) (homeDir, storageDir string) {
	t.Helper()
	forceVSCodeOSSupported(t)
	home := t.TempDir()
	storage := filepath.Join(home, darwinVSCodeWorkspaceStorageRel, "ws-1")
	if err := os.MkdirAll(filepath.Join(storage, "chatSessions"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(storage, "workspace.json"),
		[]byte(`{"folder":"file:///Users/foo/myapp"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	chatLine := `{"v":{"sessionId":"` + chatSessionID +
		`","creationDate":1700000000000,"lastMessageDate":1700000005000,` +
		`"requests":[{"message":{"text":"hello there"},"response":[{"kind":"markdownContent","content":{"value":"hi back"}}]}]}}`
	if err := os.WriteFile(
		filepath.Join(storage, "chatSessions", chatSessionID+".jsonl"),
		[]byte(chatLine+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// state.vscdb with one interactive session.
	vscdbValue := `[{"sessionId":"` + vscdbSessionID +
		`","creationDate":1700000000000,"lastMessageDate":1700000010000,` +
		`"requests":[{"message":{"text":"vscdb hello world"},"response":[{"kind":"markdownContent","content":{"value":"vscdb response"}}]}]}]`
	createVSCDB(t, filepath.Join(storage, "state.vscdb"), vscdbValue)

	return home, storage
}

// createVSCDB initializes a minimal state.vscdb at path with one row.
// Pass valueJSON = "" to create an empty ItemTable.
func createVSCDB(t *testing.T, path, valueJSON string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open vscdb: %v", err)
	}
	defer db.Close() //nolint:errcheck

	if _, err := db.Exec(`CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create ItemTable: %v", err)
	}
	if valueJSON != "" {
		if _, err := db.Exec(
			`INSERT INTO ItemTable (key, value) VALUES ('interactive.sessions', ?)`, valueJSON,
		); err != nil {
			t.Fatalf("insert row: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// vscodeWorkspaceStorageDir
// ---------------------------------------------------------------------------

// TestVSCodeOSSupportedDefault exercises the production predicate so
// coverage stays at 100% regardless of GOOS — every other test in this
// package replaces vscodeOSSupported via forceVSCodeOSSupported, which
// would otherwise leave vscodeOSSupportedDefault uncovered on Linux CI.
func TestVSCodeOSSupportedDefault(t *testing.T) {
	want := runtime.GOOS == "darwin"
	if got := vscodeOSSupportedDefault(); got != want {
		t.Errorf("vscodeOSSupportedDefault() = %v, want %v", got, want)
	}
}

func TestVSCodeWorkspaceStorageDir_Darwin(t *testing.T) {
	// Force darwin even if running off-darwin (CI is darwin-only here, but
	// the explicit override keeps the test deterministic).
	prev := vscodeOSSupported
	vscodeOSSupported = func() bool { return true }
	t.Cleanup(func() { vscodeOSSupported = prev })

	got := vscodeWorkspaceStorageDir("/h")
	want := filepath.Join("/h", darwinVSCodeWorkspaceStorageRel)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVSCodeWorkspaceStorageDir_Unsupported(t *testing.T) {
	prev := vscodeOSSupported
	vscodeOSSupported = func() bool { return false }
	t.Cleanup(func() { vscodeOSSupported = prev })

	if got := vscodeWorkspaceStorageDir("/h"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// discoverVSCodeWorkspaces
// ---------------------------------------------------------------------------

func TestDiscoverVSCodeWorkspaces_Unsupported(t *testing.T) {
	prev := vscodeOSSupported
	vscodeOSSupported = func() bool { return false }
	t.Cleanup(func() { vscodeOSSupported = prev })

	got, err := discoverVSCodeWorkspaces(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDiscoverVSCodeWorkspaces_Missing(t *testing.T) {
	got, err := discoverVSCodeWorkspaces(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDiscoverVSCodeWorkspaces_ReadDirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	forceVSCodeOSSupported(t)
	home := t.TempDir()
	parent := filepath.Join(home, "Library/Application Support/Code/User")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "workspaceStorage")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755) //nolint:errcheck

	_, err := discoverVSCodeWorkspaces(home)
	if err == nil {
		t.Fatal("expected error for unreadable workspaceStorage, got nil")
	}
}

func TestDiscoverVSCodeWorkspaces_HappyPath(t *testing.T) {
	home, _ := setupFakeVSCodeHome(t)
	got, err := discoverVSCodeWorkspaces(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d workspaces, want 1", len(got))
	}
	if got[0].ProjectPath != "/Users/foo/myapp" {
		t.Errorf("project = %q", got[0].ProjectPath)
	}
}

func TestDiscoverVSCodeWorkspaces_SkipsNonDirsAndMissingWorkspaceJSON(t *testing.T) {
	forceVSCodeOSSupported(t)
	home := t.TempDir()
	root := filepath.Join(home, darwinVSCodeWorkspaceStorageRel)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Stray file in workspaceStorage — should be skipped.
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Workspace dir without workspace.json — kept (with empty project).
	if err := os.MkdirAll(filepath.Join(root, "ws-no-json"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := discoverVSCodeWorkspaces(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].ProjectPath != "" {
		t.Errorf("project = %q, want empty", got[0].ProjectPath)
	}
}

// ---------------------------------------------------------------------------
// readWorkspaceJSON
// ---------------------------------------------------------------------------

func TestReadWorkspaceJSON(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"folder", `{"folder":"file:///foo/bar"}`, "/foo/bar"},
		{"workspace fallback", `{"workspace":"file:///foo/baz.code-workspace"}`, "/foo/baz.code-workspace"},
		{"both empty", `{}`, ""},
		{"non file uri", `{"folder":"vscode-vfs://github/foo/bar"}`, "vscode-vfs://github/foo/bar"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".json")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := readWorkspaceJSON(path); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if got := readWorkspaceJSON("/nonexistent"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		path := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := readWorkspaceJSON(path); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// ---------------------------------------------------------------------------
// decodeFileURI
// ---------------------------------------------------------------------------

func TestDecodeFileURI(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"file:///Users/foo/bar":   "/Users/foo/bar",
		"file:///foo%20bar":       "/foo bar",
		"vscode-vfs://github/x/y": "vscode-vfs://github/x/y",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := decodeFileURI(in); got != want {
				t.Errorf("decodeFileURI(%q) = %q, want %q", in, got, want)
			}
		})
	}

	t.Run("malformed URI falls back to TrimPrefix", func(t *testing.T) {
		// A URI that net/url rejects: percent-encoding an invalid byte.
		got := decodeFileURI("file://%zz")
		if got != "%zz" {
			t.Errorf("got %q, want %%zz", got)
		}
	})
}

// ---------------------------------------------------------------------------
// listChatSessionsInWorkspace + loadChatSessionFile
// ---------------------------------------------------------------------------

func TestListChatSessionsInWorkspace_Happy(t *testing.T) {
	home, storage := setupFakeVSCodeHome(t)
	_ = home

	ws := vsWorkspace{StorageDir: storage, WorkspaceID: "ws-1", ProjectPath: "/Users/foo/myapp"}
	got := listChatSessionsInWorkspace(ws)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].ID != chatSessionID {
		t.Errorf("ID = %q", got[0].ID)
	}
	if got[0].Project != "/Users/foo/myapp" {
		t.Errorf("Project = %q", got[0].Project)
	}
	if got[0].Preview != "hello there" {
		t.Errorf("Preview = %q", got[0].Preview)
	}
	if got[0].UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set from lastMessageDate")
	}
}

func TestListChatSessionsInWorkspace_NoChatDir(t *testing.T) {
	ws := vsWorkspace{StorageDir: t.TempDir()}
	if got := listChatSessionsInWorkspace(ws); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestLoadChatSessionFile_FileMissing(t *testing.T) {
	if _, ok := loadChatSessionFile("/nonexistent", vsWorkspace{}); ok {
		t.Error("expected !ok for missing file")
	}
}

func TestLoadChatSessionFile_LastLineWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.jsonl")
	body := strings.Join([]string{
		"",
		"not json",
		`{"v":{"requests":[]}}`, // empty requests — skipped
		`{"v":{"sessionId":"older","creationDate":1,"lastMessageDate":2,"requests":[{"message":{"text":"old"},"response":[]}]}}`,
		`{"v":{"sessionId":"newer","creationDate":1700000000000,"lastMessageDate":1700000005000,"requests":[{"message":{"text":"newer prompt"},"response":[]}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, ok := loadChatSessionFile(path, vsWorkspace{StorageDir: dir, WorkspaceID: "ws"})
	if !ok {
		t.Fatal("expected ok")
	}
	if sess.ID != "newer" {
		t.Errorf("ID = %q, want newer", sess.ID)
	}
	if sess.Preview != "newer prompt" {
		t.Errorf("Preview = %q", sess.Preview)
	}
}

func TestLoadChatSessionFile_NoCanonicalLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(`{"v":{"requests":[]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadChatSessionFile(path, vsWorkspace{}); ok {
		t.Error("expected !ok when no line has requests")
	}
}

func TestLoadChatSessionFile_StemFallbackAndMtimeFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stem-id.jsonl")
	// No sessionId, no creationDate, no lastMessageDate → fall back to stem
	// and file mtime for timestamps.
	body := `{"v":{"requests":[{"message":{"text":"prompt only"},"response":[]}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, ok := loadChatSessionFile(path, vsWorkspace{})
	if !ok {
		t.Fatal("expected ok")
	}
	if sess.ID != "stem-id" {
		t.Errorf("ID = %q", sess.ID)
	}
	if sess.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should fall back to mtime")
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt should fall back to UpdatedAt")
	}
}

// ---------------------------------------------------------------------------
// chatSessionMessages + joinResponseParts
// ---------------------------------------------------------------------------

func TestChatSessionMessages(t *testing.T) {
	cs := chatSession{
		Requests: []chatRequest{
			{Message: chatRequestMessage{Text: "  hello  "}, Response: []chatResponsePart{{Kind: "markdownContent", Content: chatResponseContent{Value: "world"}}}},
			{Message: chatRequestMessage{Text: ""}, Response: nil},                                                                                          // both empty: produces no msgs
			{Message: chatRequestMessage{Text: "second"}, Response: []chatResponsePart{{Value: "bare value"}}},                                              // both present
			{Message: chatRequestMessage{Text: "third"}, Response: []chatResponsePart{{Kind: "markdownVuln", Content: chatResponseContent{Value: "vuln"}}}}, // markdownVuln branch
			{Message: chatRequestMessage{Text: ""}, Response: []chatResponsePart{{Content: chatResponseContent{Value: "fallback content"}}}},                // default+content branch
		},
	}
	msgs := chatSessionMessages(cs)
	if len(msgs) != 7 {
		t.Fatalf("got %d messages, want 7 (3 user, 4 assistant): %+v", len(msgs), msgs)
	}
	if msgs[0].Role != model.RoleUser || msgs[0].Content != "hello" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != model.RoleAssistant || msgs[1].Content != "world" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
}

func TestJoinResponseParts_Empty(t *testing.T) {
	if got := joinResponseParts(nil); got != "" {
		t.Errorf("nil parts: got %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// getChatSession + loadChatSessionForGet
// ---------------------------------------------------------------------------

func TestGetChatSession_DirectStemHit(t *testing.T) {
	home, storage := setupFakeVSCodeHome(t)
	_ = home
	workspaces := []vsWorkspace{{StorageDir: storage, ProjectPath: "/Users/foo/myapp"}}
	sess, msgs, ok := getChatSession(workspaces, chatSessionID)
	if !ok {
		t.Fatal("expected hit")
	}
	if sess.ID != chatSessionID {
		t.Errorf("ID = %q", sess.ID)
	}
	if len(msgs) == 0 {
		t.Error("expected messages")
	}
}

func TestGetChatSession_FallbackScanBySessionId(t *testing.T) {
	// Place file with one stem name but the sessionId field is different.
	home := t.TempDir()
	storage := filepath.Join(home, darwinVSCodeWorkspaceStorageRel, "ws-1", "chatSessions")
	if err := os.MkdirAll(storage, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"v":{"sessionId":"hidden-id","creationDate":1,"lastMessageDate":2,"requests":[{"message":{"text":"hi"},"response":[]}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(storage, "different-stem.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	workspaces := []vsWorkspace{{StorageDir: filepath.Dir(storage)}}
	sess, _, ok := getChatSession(workspaces, "hidden-id")
	if !ok {
		t.Fatal("expected fallback hit by sessionId")
	}
	if sess.ID != "hidden-id" {
		t.Errorf("ID = %q", sess.ID)
	}
}

func TestGetChatSession_DirectStemBadFileFallsThrough(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home, storage := setupFakeVSCodeHome(t)
	_ = home

	// Make the canonical file unreadable so direct-stem path returns !ok,
	// then the fallback scan should also miss → final !ok.
	if err := os.Chmod(filepath.Join(storage, "chatSessions", chatSessionID+".jsonl"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(storage, "chatSessions", chatSessionID+".jsonl"), 0o644) //nolint:errcheck

	workspaces := []vsWorkspace{{StorageDir: storage}}
	_, _, ok := getChatSession(workspaces, chatSessionID)
	if ok {
		t.Error("expected miss when the only file is unreadable")
	}
}

func TestGetChatSession_Miss(t *testing.T) {
	workspaces := []vsWorkspace{{StorageDir: t.TempDir()}}
	if _, _, ok := getChatSession(workspaces, "nonexistent"); ok {
		t.Error("expected miss")
	}
}

func TestGetChatSession_FallbackGlobError(t *testing.T) {
	// StorageDir with a malformed bracket forces filepath.Glob to error
	// during the fallback scan; the function must continue (and return miss).
	workspaces := []vsWorkspace{{StorageDir: "/home/[malformed"}}
	if _, _, ok := getChatSession(workspaces, "anything"); ok {
		t.Error("expected miss when glob errors")
	}
}

func TestLoadChatSessionForGet_MalformedLineSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mix.jsonl")
	body := "not json\n" +
		`{"v":{"sessionId":"good","creationDate":1,"lastMessageDate":2,"requests":[{"message":{"text":"hi"},"response":[]}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, _, ok := loadChatSessionForGet(path, vsWorkspace{})
	if !ok {
		t.Fatal("expected ok")
	}
	if sess.ID != "good" {
		t.Errorf("ID = %q", sess.ID)
	}
}

// ---------------------------------------------------------------------------
// listVSCDBSessionsInWorkspace + readVSCDBInteractiveSessions
// ---------------------------------------------------------------------------

func TestListVSCDBSessionsInWorkspace_Happy(t *testing.T) {
	_, storage := setupFakeVSCodeHome(t)
	ws := vsWorkspace{StorageDir: storage, ProjectPath: "/Users/foo/myapp"}
	got := listVSCDBSessionsInWorkspace(ws)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].ID != vscdbSessionID {
		t.Errorf("ID = %q", got[0].ID)
	}
	if got[0].Project != "/Users/foo/myapp" {
		t.Errorf("Project = %q", got[0].Project)
	}
}

func TestListVSCDBSessionsInWorkspace_MissingDB(t *testing.T) {
	ws := vsWorkspace{StorageDir: t.TempDir()}
	if got := listVSCDBSessionsInWorkspace(ws); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestListVSCDBSessionsInWorkspace_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	createVSCDB(t, filepath.Join(dir, "state.vscdb"), "not json")
	ws := vsWorkspace{StorageDir: dir}
	if got := listVSCDBSessionsInWorkspace(ws); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestListVSCDBSessionsInWorkspace_EmptyRequestsSkipped(t *testing.T) {
	dir := t.TempDir()
	createVSCDB(t, filepath.Join(dir, "state.vscdb"), `[{"sessionId":"x","requests":[]}]`)
	ws := vsWorkspace{StorageDir: dir}
	if got := listVSCDBSessionsInWorkspace(ws); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestListVSCDBSessionsInWorkspace_SessionIdFallback(t *testing.T) {
	dir := t.TempDir()
	createVSCDB(t, filepath.Join(dir, "state.vscdb"),
		`[{"creationDate":1700000000000,"requests":[{"message":{"text":"q"},"response":[]}]}]`)
	ws := vsWorkspace{StorageDir: dir, WorkspaceID: "ws-x"}
	got := listVSCDBSessionsInWorkspace(ws)
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if !strings.HasPrefix(got[0].ID, "ws-x-") {
		t.Errorf("ID = %q, want ws-x-prefix", got[0].ID)
	}
}

func TestReadVSCDBInteractiveSessions_Missing(t *testing.T) {
	if _, ok := readVSCDBInteractiveSessions("/nonexistent.vscdb"); ok {
		t.Error("expected miss for nonexistent path")
	}
}

func TestReadVSCDBInteractiveSessions_NoRow(t *testing.T) {
	dir := t.TempDir()
	createVSCDB(t, filepath.Join(dir, "state.vscdb"), "")
	if _, ok := readVSCDBInteractiveSessions(filepath.Join(dir, "state.vscdb")); ok {
		t.Error("expected miss when key is absent")
	}
}

func TestReadVSCDBInteractiveSessions_NotADatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.vscdb")
	if err := os.WriteFile(path, []byte("not a sqlite file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := readVSCDBInteractiveSessions(path); ok {
		t.Error("expected miss for invalid sqlite file")
	}
}

// ---------------------------------------------------------------------------
// getVSCDBSession
// ---------------------------------------------------------------------------

func TestGetVSCDBSession_Hit(t *testing.T) {
	_, storage := setupFakeVSCodeHome(t)
	workspaces := []vsWorkspace{{StorageDir: storage, ProjectPath: "/Users/foo/myapp"}}
	sess, msgs, ok := getVSCDBSession(workspaces, vscdbSessionID)
	if !ok {
		t.Fatal("expected hit")
	}
	if sess.ID != vscdbSessionID {
		t.Errorf("ID = %q", sess.ID)
	}
	if len(msgs) == 0 {
		t.Error("expected messages")
	}
}

func TestGetVSCDBSession_Miss(t *testing.T) {
	_, storage := setupFakeVSCodeHome(t)
	workspaces := []vsWorkspace{{StorageDir: storage}}
	if _, _, ok := getVSCDBSession(workspaces, "nope"); ok {
		t.Error("expected miss")
	}
}

func TestGetVSCDBSession_SkipsWorkspaceWithoutDB(t *testing.T) {
	// First workspace has no state.vscdb, second has the session.
	_, storage := setupFakeVSCodeHome(t)
	empty := t.TempDir()
	workspaces := []vsWorkspace{
		{StorageDir: empty},
		{StorageDir: storage},
	}
	sess, _, ok := getVSCDBSession(workspaces, vscdbSessionID)
	if !ok {
		t.Fatal("expected hit on second workspace")
	}
	if sess.ID != vscdbSessionID {
		t.Errorf("ID = %q", sess.ID)
	}
}

func TestGetVSCDBSession_MalformedJSONSkipped(t *testing.T) {
	dir := t.TempDir()
	createVSCDB(t, filepath.Join(dir, "state.vscdb"), "not json")
	workspaces := []vsWorkspace{{StorageDir: dir}}
	if _, _, ok := getVSCDBSession(workspaces, "anything"); ok {
		t.Error("expected miss for malformed JSON")
	}
}

func TestGetVSCDBSession_EmptyRequestsSkipped(t *testing.T) {
	dir := t.TempDir()
	createVSCDB(t, filepath.Join(dir, "state.vscdb"), `[{"sessionId":"x","requests":[]}]`)
	workspaces := []vsWorkspace{{StorageDir: dir}}
	if _, _, ok := getVSCDBSession(workspaces, "x"); ok {
		t.Error("expected miss when requests are empty")
	}
}

// ---------------------------------------------------------------------------
// vscdbSessionToSession
// ---------------------------------------------------------------------------

func TestVSCDBSessionToSession_DefaultsAndFallbacks(t *testing.T) {
	cs := chatSession{
		// no SessionID, no LastMessageDate
		CreationDate: 1700000000000,
		Requests: []chatRequest{
			{Message: chatRequestMessage{Text: "hi"}},
		},
	}
	ws := vsWorkspace{WorkspaceID: "ws-1", ProjectPath: "/p"}
	sess := vscdbSessionToSession(cs, ws)
	if !strings.HasPrefix(sess.ID, "ws-1-") {
		t.Errorf("ID = %q", sess.ID)
	}
	// updatedAt should fall back to startedAt.
	if !sess.UpdatedAt.Equal(sess.StartedAt) {
		t.Errorf("UpdatedAt %v != StartedAt %v", sess.UpdatedAt, sess.StartedAt)
	}
}

// ---------------------------------------------------------------------------
// loadChatSessionForGet — error paths not exercised elsewhere
// ---------------------------------------------------------------------------

func TestLoadChatSessionForGet_FileMissing(t *testing.T) {
	if _, _, ok := loadChatSessionForGet("/nonexistent", vsWorkspace{}); ok {
		t.Error("expected !ok")
	}
}

func TestLoadChatSessionForGet_NoCanonicalLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(`{"v":{"requests":[]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := loadChatSessionForGet(path, vsWorkspace{}); ok {
		t.Error("expected !ok")
	}
}

func TestLoadChatSessionForGet_StemFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stem-only.jsonl")
	body := `{"v":{"requests":[{"message":{"text":"hi"},"response":[]}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, _, ok := loadChatSessionForGet(path, vsWorkspace{})
	if !ok {
		t.Fatal("expected ok")
	}
	if sess.ID != "stem-only" {
		t.Errorf("ID = %q", sess.ID)
	}
	if sess.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should fall back to mtime")
	}
}

// ---------------------------------------------------------------------------
// msToTime / truncatePreview
// ---------------------------------------------------------------------------

func TestMsToTime(t *testing.T) {
	if got := msToTime(0); !got.IsZero() {
		t.Errorf("zero ms: got %v, want zero", got)
	}
	if got := msToTime(-1); !got.IsZero() {
		t.Errorf("negative ms: got %v, want zero", got)
	}
	got := msToTime(1700000000000)
	want := time.UnixMilli(1700000000000).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTruncatePreview(t *testing.T) {
	if got := truncatePreview("\n  hello\nworld  \r\n"); got != "hello world" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("a", 200)
	got := truncatePreview(long)
	if len(got) != 120 {
		t.Errorf("len = %d, want 120", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("missing ellipsis: %q", got)
	}
}
