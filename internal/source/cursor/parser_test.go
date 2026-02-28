package cursor

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupFakeHome builds a minimal ~/.cursor layout in a temp dir and returns
// the home directory path. It mirrors the real Cursor on-disk layout:
//
//	<home>/.cursor/projects/<encodedProj>/agent-transcripts/<convID>.txt
//	<home>/.cursor/ai-tracking/ai-code-tracking.db   (optional, via addTrackingDB)
//	<home>/.cursor/chats/<ws>/<agent>/store.db        (optional, via addChatStore)
func setupFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	return home
}

const (
	fixtureConvID  = "conv-aabbccdd-1234-5678"
	fixtureConvID2 = "conv-11223344-aaaa-bbbb"

	// Cursor encodes /Users/testuser/prj/myproject as Users-testuser-prj-myproject
	// (no leading slash, dots/slashes → dashes).
	// We deliberately keep the encoded name simple so projectPathFromDir produces
	// a predictable result via the naive-decode fallback.
	fixtureProjDirName = "Users-testuser-prj-myproject"
	fixtureProjPath    = "/Users/testuser/prj/myproject"
)

// addTranscriptFile writes a transcript fixture into the fake home.
// Returns the full path to the written file.
func addTranscriptFile(t *testing.T, homeDir, projDirName, convID, content string) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".cursor", "projects", projDirName, "agent-transcripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create transcript dir: %v", err)
	}
	path := filepath.Join(dir, convID+".txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}

// addTrackingDB creates the ai-code-tracking.db in <home>/.cursor/ai-tracking/
// with one row per summary in the conversation_summaries table.
func addTrackingDB(t *testing.T, homeDir string, rows []conversationSummary) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".cursor", "ai-tracking")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create ai-tracking dir: %v", err)
	}
	dbPath := filepath.Join(dir, "ai-code-tracking.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE conversation_summaries (
		conversationId TEXT,
		title         TEXT,
		tldr          TEXT,
		overview      TEXT,
		model         TEXT,
		mode          TEXT,
		updatedAt     INTEGER
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	for _, r := range rows {
		var updatedAtMs sql.NullInt64
		if !r.UpdatedAt.IsZero() {
			updatedAtMs = sql.NullInt64{Int64: r.UpdatedAt.UnixMilli(), Valid: true}
		}
		_, err = db.Exec(
			`INSERT INTO conversation_summaries VALUES (?,?,?,?,?,?,?)`,
			r.ConversationID, r.Title, r.TLDR, r.Overview, r.Model, r.Mode, updatedAtMs,
		)
		if err != nil {
			t.Fatalf("insert summary: %v", err)
		}
	}
	return dbPath
}

// addChatStoreDB creates a store.db with a meta table containing a hex-encoded
// chatMeta JSON under key "0".
func addChatStoreDB(t *testing.T, homeDir, wsDirName, agentDirName string, meta chatMeta) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".cursor", "chats", wsDirName, agentDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create chat store dir: %v", err)
	}
	dbPath := filepath.Join(dir, "store.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE meta (key TEXT, value TEXT)`)
	if err != nil {
		t.Fatalf("create meta table: %v", err)
	}

	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	hexVal := hex.EncodeToString(jsonBytes)

	_, err = db.Exec(`INSERT INTO meta VALUES ('0', ?)`, hexVal)
	if err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return dbPath
}

// ---------------------------------------------------------------------------
// transcript.go — parseTranscript
// ---------------------------------------------------------------------------

func TestParseTranscript_Simple(t *testing.T) {
	path := filepath.Join("testdata", "transcript_simple.txt")
	msgs, err := parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}

	// Expect: user, assistant (with tool call + result), assistant, user
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}

	if msgs[0].Role != model.RoleUser {
		t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "fix this bug") {
		t.Errorf("msgs[0].Content = %q, want to contain 'fix this bug'", msgs[0].Content)
	}

	// Second message: assistant with tool call
	if msgs[1].Role != model.RoleAssistant {
		t.Errorf("msgs[1].Role = %q, want assistant", msgs[1].Role)
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("msgs[1].ToolCalls = %d, want 1", len(msgs[1].ToolCalls))
	}
	if msgs[1].ToolCalls[0].Name != "Read" {
		t.Errorf("tool call name = %q, want Read", msgs[1].ToolCalls[0].Name)
	}
	if msgs[1].ToolCalls[0].Output == "" {
		t.Error("expected non-empty tool result output")
	}
}

func TestParseTranscript_InlineContent(t *testing.T) {
	path := filepath.Join("testdata", "transcript_inline_content.txt")
	msgs, err := parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != model.RoleUser {
		t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "2+2") {
		t.Errorf("msgs[0].Content = %q, want to contain '2+2'", msgs[0].Content)
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Errorf("msgs[1].Role = %q, want assistant", msgs[1].Role)
	}
}

func TestParseTranscript_Markup(t *testing.T) {
	path := filepath.Join("testdata", "transcript_markup.txt")
	msgs, err := parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}
	if len(msgs) < 1 {
		t.Fatal("expected at least 1 message")
	}
	// user_query tags and Thinking should be stripped
	if strings.Contains(msgs[0].Content, "<user_query>") {
		t.Errorf("user_query tag not stripped: %q", msgs[0].Content)
	}
	if strings.Contains(msgs[0].Content, "</user_query>") {
		t.Errorf("user_query close tag not stripped: %q", msgs[0].Content)
	}
}

func TestParseTranscript_OrphanToolCall(t *testing.T) {
	path := filepath.Join("testdata", "transcript_orphan_tool.txt")
	msgs, err := parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}
	// Orphan tool call should be wrapped in an assistant message
	found := false
	for _, m := range msgs {
		if m.Role == model.RoleAssistant && len(m.ToolCalls) > 0 {
			found = true
			if m.ToolCalls[0].Name != "Bash" {
				t.Errorf("tool name = %q, want Bash", m.ToolCalls[0].Name)
			}
		}
	}
	if !found {
		t.Error("expected orphan tool call wrapped in assistant message")
	}
}

func TestParseTranscript_NonExistent(t *testing.T) {
	_, err := parseTranscript("testdata/nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestParseTranscript_ToolResultNoAssistant(t *testing.T) {
	// Tool result with a preceding assistant that has no tool calls — covers the
	// "len(messages) > 0 but last message has no tool calls" branch in stateToolResult.
	dir := t.TempDir()
	path := filepath.Join(dir, "t.txt")
	content := "A:\nsome text\n[Tool result]\nthe output\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, err := parseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should produce an assistant message with no tool calls — result is attached to last tool call of last assistant
	// but there is no tool call, so the result is silently dropped.
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestParseTranscript_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, err := parseTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty file, got %d", len(msgs))
	}
}

// ---------------------------------------------------------------------------
// transcript.go — marker helpers
// ---------------------------------------------------------------------------

func TestIsUserMarker(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"user:", true},
		{"User:", true},
		{"user: hello", true},
		{"User: hello", true},
		{"assistant:", false},
		{"", false},
	}
	for _, c := range cases {
		got := isUserMarker(c.s)
		if got != c.want {
			t.Errorf("isUserMarker(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestIsAssistantMarker(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"assistant:", true},
		{"A:", true},
		{"assistant: text", true},
		{"A: text", true},
		{"user:", false},
		{"", false},
	}
	for _, c := range cases {
		got := isAssistantMarker(c.s)
		if got != c.want {
			t.Errorf("isAssistantMarker(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestIsToolCallMarker(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"[Tool call: Bash]", true},
		{"[Tool call:Bash]", true},
		{"[Tool result]", false},
		{"user:", false},
	}
	for _, c := range cases {
		got := isToolCallMarker(c.s)
		if got != c.want {
			t.Errorf("isToolCallMarker(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestIsToolResultMarker(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"[Tool result]", true},
		{"[Tool result: foo]", true},
		{"[Tool call: Bash]", false},
		{"", false},
	}
	for _, c := range cases {
		got := isToolResultMarker(c.s)
		if got != c.want {
			t.Errorf("isToolResultMarker(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestExtractAfterMarker(t *testing.T) {
	cases := []struct {
		line   string
		marker string
		want   string
	}{
		{"user: hello world", "user:", "hello world"},
		{"user:", "user:", ""},
		{"no marker here", "user:", ""},
		{"A: answer text", "A:", "answer text"},
	}
	for _, c := range cases {
		got := extractAfterMarker(c.line, c.marker)
		if got != c.want {
			t.Errorf("extractAfterMarker(%q, %q) = %q, want %q", c.line, c.marker, got, c.want)
		}
	}
}

func TestExtractToolCallName(t *testing.T) {
	cases := []struct {
		s    string
		want string
	}{
		{"[Tool call: Bash]", "Bash"},
		{"[Tool call:Read]", "Read"},
		{"[Tool call: Write File]", "Write File"},
		{"[Tool call]", ""},
	}
	for _, c := range cases {
		got := extractToolCallName(c.s)
		if got != c.want {
			t.Errorf("extractToolCallName(%q) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestCleanCursorMarkup(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"<user_query>hello</user_query>", "hello"},
		{"[Thinking]\nsome text", "some text"},
		{"plain text", "plain text"},
		{"  spaced  ", "spaced"},
	}
	for _, c := range cases {
		got := cleanCursorMarkup(c.input)
		if got != c.want {
			t.Errorf("cleanCursorMarkup(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// workspace.go — encodePath / projectPathFromDir / resolvePathGreedy
// ---------------------------------------------------------------------------

func TestEncodePath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/Users/foo/bar", "Users-foo-bar"},
		{"/Users/foo.bar/baz", "Users-foo-bar-baz"},
		{"", ""},
	}
	for _, c := range cases {
		got := encodePath(c.input)
		if got != c.want {
			t.Errorf("encodePath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestProjectPathFromDir_NaiveFallback(t *testing.T) {
	// When the dirName does not start with the encoded homeDir, naive decode applies.
	got := projectPathFromDir("some-random-dir")
	if !strings.HasPrefix(got, "/") {
		t.Errorf("expected absolute path, got %q", got)
	}
}

func TestProjectPathFromDir_HomeMatch(t *testing.T) {
	// Build a real temp dir so the greedy walk can succeed.
	home := t.TempDir()
	// Encode the home the same way
	homeEncoded := encodePath(home)

	// Create a sub-project directory under home
	projDir := filepath.Join(home, "myproject")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Override HOME so projectPathFromDir uses our temp dir
	t.Setenv("HOME", home)

	// Encode the project path: homeEncoded + "-" + "myproject"
	encoded := homeEncoded + "-myproject"
	got := projectPathFromDir(encoded)
	if got != projDir {
		t.Errorf("projectPathFromDir(%q) = %q, want %q", encoded, got, projDir)
	}
}

func TestProjectPathFromDir_HomeSuffixEmpty(t *testing.T) {
	// dirName == encodedHome exactly — should return homeDir
	home := t.TempDir()
	t.Setenv("HOME", home)

	homeEncoded := encodePath(home)
	got := projectPathFromDir(homeEncoded)
	if got != home {
		t.Errorf("projectPathFromDir(encodedHome) = %q, want %q", got, home)
	}
}

func TestProjectPathFromDir_HomeSuffixUnresolvable(t *testing.T) {
	// dirName starts with encodedHome but suffix doesn't match any real directory
	home := t.TempDir()
	t.Setenv("HOME", home)

	homeEncoded := encodePath(home)
	encoded := homeEncoded + "-nonexistent-deep-path"
	got := projectPathFromDir(encoded)
	// Falls through to naive decode: should start with "/"
	if !strings.HasPrefix(got, "/") {
		t.Errorf("expected absolute path, got %q", got)
	}
}

func TestResolvePathGreedy_EmptyEncoded(t *testing.T) {
	home := t.TempDir()
	got := resolvePathGreedy(home, "")
	if got != home {
		t.Errorf("resolvePathGreedy with empty encoded = %q, want %q", got, home)
	}
}

func TestResolvePathGreedy_UnreadableDir(t *testing.T) {
	got := resolvePathGreedy("/nonexistent/path", "foo")
	if got != "" {
		t.Errorf("expected empty string for unreadable dir, got %q", got)
	}
}

func TestResolvePathGreedy_LongestMatchFirst(t *testing.T) {
	// Create two dirs: "foo" and "foo-bar" so that "foo-bar-baz" matches "foo-bar" (longest).
	// Also add a non-dir file to exercise the !e.IsDir() continue branch.
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "foo", "baz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "foo-bar", "baz"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a regular file that sorts before the dirs so the !IsDir branch is hit
	if err := os.WriteFile(filepath.Join(base, "AAA-not-a-dir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// "foo-bar-baz": should prefer foo-bar / baz over foo / bar-baz
	got := resolvePathGreedy(base, "foo-bar-baz")
	want := filepath.Join(base, "foo-bar", "baz")
	if got != want {
		t.Errorf("resolvePathGreedy = %q, want %q", got, want)
	}
}

func TestResolvePathGreedy_ExactMatch(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "myproject"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolvePathGreedy(base, "myproject")
	want := filepath.Join(base, "myproject")
	if got != want {
		t.Errorf("resolvePathGreedy exact = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// workspace.go — findTranscriptFile / listAllTranscripts
// ---------------------------------------------------------------------------

func TestFindTranscriptFile_ExactMatch(t *testing.T) {
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")

	projPath, filePath := findTranscriptFile(home, fixtureConvID)
	if filePath == "" {
		t.Fatal("expected file path, got empty")
	}
	if !strings.HasSuffix(filePath, fixtureConvID+".txt") {
		t.Errorf("filePath = %q, want suffix %q", filePath, fixtureConvID+".txt")
	}
	_ = projPath
}

func TestFindTranscriptFile_NoMatch(t *testing.T) {
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")

	_, filePath := findTranscriptFile(home, "no-such-id")
	if filePath != "" {
		t.Errorf("expected empty path, got %q", filePath)
	}
}

func TestFindTranscriptFile_NoProjectsDir(t *testing.T) {
	home := t.TempDir() // no .cursor/projects created
	_, filePath := findTranscriptFile(home, fixtureConvID)
	if filePath != "" {
		t.Errorf("expected empty path, got %q", filePath)
	}
}

func TestListAllTranscripts_Basic(t *testing.T) {
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID2, "user:\nworld\n")

	entries := listAllTranscripts(home)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.ConversationID] = true
	}
	if !ids[fixtureConvID] {
		t.Errorf("missing convID %q", fixtureConvID)
	}
	if !ids[fixtureConvID2] {
		t.Errorf("missing convID %q", fixtureConvID2)
	}
}

func TestListAllTranscripts_NoProjectsDir(t *testing.T) {
	home := t.TempDir()
	entries := listAllTranscripts(home)
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestListAllTranscripts_SkipsNonDirEntries(t *testing.T) {
	home := setupFakeHome(t)
	// Add a real transcript
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")
	// Add a non-.txt file to agent-transcripts — should be skipped
	transcriptsDir := filepath.Join(home, ".cursor", "projects", fixtureProjDirName, "agent-transcripts")
	if err := os.WriteFile(filepath.Join(transcriptsDir, "notranscript.dat"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := listAllTranscripts(home)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (non-.txt skipped), got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// metadata.go — readConversationSummaries
// ---------------------------------------------------------------------------

func TestReadConversationSummaries_NonExistentDB(t *testing.T) {
	summaries, err := readConversationSummaries("/tmp/nonexistent/db.sqlite")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent db, got %v", err)
	}
	if summaries != nil {
		t.Errorf("expected nil summaries, got %v", summaries)
	}
}

func TestReadConversationSummaries_WithData(t *testing.T) {
	home := setupFakeHome(t)
	ts := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	dbPath := addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: fixtureConvID,
			Title:          "Fix the bug",
			TLDR:           "Fixed it",
			Overview:       "Overview text",
			Model:          "gpt-4",
			Mode:           "agent",
			UpdatedAt:      ts,
		},
	})

	summaries, err := readConversationSummaries(dbPath)
	if err != nil {
		t.Fatalf("readConversationSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]
	if s.ConversationID != fixtureConvID {
		t.Errorf("ConversationID = %q, want %q", s.ConversationID, fixtureConvID)
	}
	if s.Title != "Fix the bug" {
		t.Errorf("Title = %q, want 'Fix the bug'", s.Title)
	}
	if s.UpdatedAt.UnixMilli() != ts.UnixMilli() {
		t.Errorf("UpdatedAt = %v, want %v", s.UpdatedAt, ts)
	}
}

func TestReadConversationSummaries_NoTable(t *testing.T) {
	// DB exists but has no conversation_summaries table.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Create an unrelated table so the DB file exists.
	if _, err := db.Exec("CREATE TABLE dummy (id TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	summaries, err := readConversationSummaries(dbPath)
	if err != nil {
		t.Fatalf("expected nil error when table missing, got %v", err)
	}
	if summaries != nil {
		t.Errorf("expected nil summaries when table missing, got %v", summaries)
	}
}

func TestReadConversationSummaries_NullUpdatedAt(t *testing.T) {
	// Row with NULL updatedAt — should produce zero time.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE conversation_summaries (
		conversationId TEXT, title TEXT, tldr TEXT, overview TEXT,
		model TEXT, mode TEXT, updatedAt INTEGER)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO conversation_summaries VALUES ('c1','t','tl','ov','m','md',NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	summaries, err := readConversationSummaries(dbPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if !summaries[0].UpdatedAt.IsZero() {
		t.Errorf("expected zero UpdatedAt for NULL, got %v", summaries[0].UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// chatstore.go — readChatStoreMeta / readAllChatMeta / chatMetaCreatedAt
// ---------------------------------------------------------------------------

func TestChatMetaCreatedAt_Zero(t *testing.T) {
	m := chatMeta{CreatedAt: 0}
	got := chatMetaCreatedAt(m)
	if !got.IsZero() {
		t.Errorf("expected zero time for CreatedAt=0, got %v", got)
	}
}

func TestChatMetaCreatedAt_NonZero(t *testing.T) {
	ms := int64(1739091671000)
	m := chatMeta{CreatedAt: ms}
	got := chatMetaCreatedAt(m)
	want := time.UnixMilli(ms)
	if !got.Equal(want) {
		t.Errorf("chatMetaCreatedAt = %v, want %v", got, want)
	}
}

func TestReadChatStoreMeta_NonExistent(t *testing.T) {
	_, err := readChatStoreMeta("/tmp/nonexistent/store.db")
	if err == nil {
		t.Fatal("expected error for non-existent store.db, got nil")
	}
}

func TestReadChatStoreMeta_WithData(t *testing.T) {
	home := setupFakeHome(t)
	meta := chatMeta{
		AgentID:   fixtureConvID,
		Name:      "My session",
		Mode:      "agent",
		CreatedAt: 1739091671000,
		Model:     "claude-3-5-sonnet",
	}
	dbPath := addChatStoreDB(t, home, "workspace1", fixtureConvID, meta)

	got, err := readChatStoreMeta(dbPath)
	if err != nil {
		t.Fatalf("readChatStoreMeta: %v", err)
	}
	if got.AgentID != fixtureConvID {
		t.Errorf("AgentID = %q, want %q", got.AgentID, fixtureConvID)
	}
	if got.Name != "My session" {
		t.Errorf("Name = %q, want 'My session'", got.Name)
	}
	if got.Model != "claude-3-5-sonnet" {
		t.Errorf("Model = %q, want 'claude-3-5-sonnet'", got.Model)
	}
}

func TestReadChatStoreMeta_NoMetaKey(t *testing.T) {
	// store.db exists but has no row with key '0'
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE meta (key TEXT, value TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = readChatStoreMeta(dbPath)
	if err == nil {
		t.Fatal("expected error when key '0' missing, got nil")
	}
}

func TestReadChatStoreMeta_InvalidHex(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE meta (key TEXT, value TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO meta VALUES ('0', 'not-valid-hex!!')")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = readChatStoreMeta(dbPath)
	if err == nil {
		t.Fatal("expected error for invalid hex, got nil")
	}
}

func TestReadChatStoreMeta_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE meta (key TEXT, value TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	// Valid hex but invalid JSON
	hexVal := hex.EncodeToString([]byte("{invalid json"))
	_, err = db.Exec("INSERT INTO meta VALUES ('0', ?)", hexVal)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = readChatStoreMeta(dbPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestReadAllChatMeta_Basic(t *testing.T) {
	home := setupFakeHome(t)
	meta := chatMeta{
		AgentID:   fixtureConvID,
		Name:      "Test session",
		CreatedAt: 1739091671000,
		Model:     "gpt-4",
	}
	addChatStoreDB(t, home, "workspace1", fixtureConvID, meta)

	result := readAllChatMeta(home)
	if len(result) != 1 {
		t.Fatalf("expected 1 chat meta, got %d", len(result))
	}
	if _, ok := result[fixtureConvID]; !ok {
		t.Errorf("expected key %q in result", fixtureConvID)
	}
}

func TestReadAllChatMeta_NoChatsDir(t *testing.T) {
	home := t.TempDir()
	result := readAllChatMeta(home)
	if len(result) != 0 {
		t.Errorf("expected empty map for missing chats dir, got %d entries", len(result))
	}
}

func TestReadAllChatMeta_SkipsNonDirs(t *testing.T) {
	home := setupFakeHome(t)
	chatsDir := filepath.Join(home, ".cursor", "chats")
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a file (not a dir) under chats/
	if err := os.WriteFile(filepath.Join(chatsDir, "notadir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Add a valid workspace dir with an agent
	meta := chatMeta{AgentID: fixtureConvID, Name: "session"}
	addChatStoreDB(t, home, "workspace1", fixtureConvID, meta)

	result := readAllChatMeta(home)
	if len(result) != 1 {
		t.Errorf("expected 1 entry (file under chats skipped), got %d", len(result))
	}
}

func TestReadAllChatMeta_SkipsNonDirAgents(t *testing.T) {
	home := setupFakeHome(t)
	// Create workspace dir
	wsDir := filepath.Join(home, ".cursor", "chats", "workspace1")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a file (not a dir) under the workspace
	if err := os.WriteFile(filepath.Join(wsDir, "notadir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := readAllChatMeta(home)
	if len(result) != 0 {
		t.Errorf("expected 0 entries (non-dir agent skipped), got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// cursor.go — matchesFilter
// ---------------------------------------------------------------------------

func TestMatchesFilter(t *testing.T) {
	now := time.Now()
	old := now.Add(-24 * time.Hour)

	tests := []struct {
		name   string
		sess   model.Session
		opts   source.ListOptions
		cutoff time.Time
		want   bool
	}{
		{
			name: "no filters — passes",
			sess: model.Session{ID: "a", UpdatedAt: now},
			opts: source.ListOptions{},
			want: true,
		},
		{
			name:   "cutoff: recent session passes",
			sess:   model.Session{UpdatedAt: now},
			opts:   source.ListOptions{},
			cutoff: now.Add(-1 * time.Hour),
			want:   true,
		},
		{
			name:   "cutoff: old session filtered out",
			sess:   model.Session{UpdatedAt: old},
			opts:   source.ListOptions{},
			cutoff: now.Add(-1 * time.Hour),
			want:   false,
		},
		{
			name:   "project filter: match",
			sess:   model.Session{UpdatedAt: now, Project: "/Users/foo/myproject"},
			opts:   source.ListOptions{Project: "myproject"},
			cutoff: time.Time{},
			want:   true,
		},
		{
			name:   "project filter: no match",
			sess:   model.Session{UpdatedAt: now, Project: "/Users/foo/other"},
			opts:   source.ListOptions{Project: "myproject"},
			cutoff: time.Time{},
			want:   false,
		},
		{
			name:   "active filter: active session passes",
			sess:   model.Session{UpdatedAt: now, Active: true},
			opts:   source.ListOptions{Active: true},
			cutoff: time.Time{},
			want:   true,
		},
		{
			name:   "active filter: inactive session filtered out",
			sess:   model.Session{UpdatedAt: now, Active: false},
			opts:   source.ListOptions{Active: true},
			cutoff: time.Time{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(tt.sess, tt.opts, tt.cutoff)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cursor.go — extractSnippet
// ---------------------------------------------------------------------------

func TestExtractSnippet(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		matchIdx  int
		matchLen  int
		maxLen    int
		wantExact string
		wantHas   string
	}{
		{
			name:      "short content no ellipsis",
			content:   "short",
			matchIdx:  0,
			matchLen:  5,
			maxLen:    200,
			wantExact: "short",
		},
		{
			name:     "match in middle has ellipsis",
			content:  "aaaaaaaaaaaaaaaa MATCH bbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			matchIdx: 17,
			matchLen: 5,
			maxLen:   20,
			wantHas:  "MATCH",
		},
		{
			name:     "match at start no leading ellipsis",
			content:  "MATCH bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			matchIdx: 0,
			matchLen: 5,
			maxLen:   10,
			wantHas:  "MATCH",
		},
		{
			name:      "newlines converted to spaces",
			content:   "line1\nline2",
			matchIdx:  0,
			matchLen:  5,
			maxLen:    200,
			wantExact: "line1 line2",
		},
		{
			name:     "match near end recalculates start",
			content:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa MATCH",
			matchIdx: 40,
			matchLen: 5,
			maxLen:   15,
			wantHas:  "MATCH",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSnippet(tt.content, tt.matchIdx, tt.matchLen, tt.maxLen)
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("extractSnippet() = %q, want %q", got, tt.wantExact)
			}
			if tt.wantHas != "" && !strings.Contains(got, tt.wantHas) {
				t.Errorf("extractSnippet() = %q, want to contain %q", got, tt.wantHas)
			}
		})
	}
}

func TestExtractSnippet_MatchLenExceedsMaxLen(t *testing.T) {
	// matchLen > maxLen: contextBefore goes negative, clipped to 0
	got := extractSnippet("hello world", 0, 50, 5)
	if got == "" {
		t.Error("expected non-empty snippet")
	}
}

// ---------------------------------------------------------------------------
// cursor.go — cursorSource (Name / List / Get / Search)
// ---------------------------------------------------------------------------

func TestCursorSource_Name(t *testing.T) {
	s := &cursorSource{}
	if s.Name() != model.ToolCursor {
		t.Errorf("Name() = %q, want %q", s.Name(), model.ToolCursor)
	}
}

// setupCursorHome creates a full fake Cursor home with:
// - one transcript file
// - one tracking DB row matching the transcript
// - one chat store DB
// Returns (home, convID, session updatedAt).
func setupCursorHome(t *testing.T) (string, string, time.Time) {
	t.Helper()
	home := setupFakeHome(t)

	const convID = fixtureConvID
	transcript := "user:\nHelp me with Go.\n\nassistant:\nSure, I can help.\n"
	addTranscriptFile(t, home, fixtureProjDirName, convID, transcript)

	updatedAt := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: convID,
			Title:          "Go help",
			TLDR:           "Helping with Go",
			Model:          "claude-3-5-sonnet",
			UpdatedAt:      updatedAt,
		},
	})

	meta := chatMeta{
		AgentID:   convID,
		Name:      "Go session",
		CreatedAt: updatedAt.UnixMilli(),
		Model:     "claude-3-5-sonnet",
	}
	addChatStoreDB(t, home, "workspace1", convID, meta)

	return home, convID, updatedAt
}

func TestList_Basic(t *testing.T) {
	home, convID, _ := setupCursorHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != convID {
		t.Errorf("ID = %q, want %q", sessions[0].ID, convID)
	}
	if sessions[0].Tool != model.ToolCursor {
		t.Errorf("Tool = %q, want cursor", sessions[0].Tool)
	}
	if sessions[0].Title != "Go help" {
		t.Errorf("Title = %q, want 'Go help'", sessions[0].Title)
	}
}

func TestList_Limit(t *testing.T) {
	home, _, _ := setupCursorHome(t)
	// Add a second transcript (orphan, no DB row)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID2, "user:\nhello\n")
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session with Limit=1, got %d", len(sessions))
	}
}

func TestList_SinceFilter(t *testing.T) {
	home, _, _ := setupCursorHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	// 1 nanosecond Since: all old sessions excluded
	sessions, err := s.List(source.ListOptions{Since: 1})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions with very short Since, got %d", len(sessions))
	}
}

func TestList_ProjectFilter(t *testing.T) {
	home, _, _ := setupCursorHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}

	// Matching project substring
	sessions, err := s.List(source.ListOptions{Project: "myproject"})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session matching project, got %d", len(sessions))
	}

	// Non-matching project
	sessions, err = s.List(source.ListOptions{Project: "zzznomatch"})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions with non-matching project, got %d", len(sessions))
	}
}

func TestList_OrphanTranscript(t *testing.T) {
	// No tracking DB — pure orphan transcript path
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nHello from orphan\n")
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 orphan session, got %d", len(sessions))
	}
	// Preview should come from first user message
	if !strings.Contains(sessions[0].Preview, "Hello from orphan") {
		t.Errorf("Preview = %q, want to contain 'Hello from orphan'", sessions[0].Preview)
	}
}

func TestList_OrphanTranscriptWithChatMeta(t *testing.T) {
	// Orphan transcript (no tracking DB) but has chat store metadata
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nHello\n")
	meta := chatMeta{
		AgentID:   fixtureConvID,
		Name:      "Named session",
		CreatedAt: time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC).UnixMilli(),
		Model:     "gpt-4o",
	}
	addChatStoreDB(t, home, "ws1", fixtureConvID, meta)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "Named session" {
		t.Errorf("Title = %q, want 'Named session'", sessions[0].Title)
	}
	if sessions[0].Model != "gpt-4o" {
		t.Errorf("Model = %q, want 'gpt-4o'", sessions[0].Model)
	}
}

func TestList_DBWithNoTranscript(t *testing.T) {
	// DB has a summary but no matching transcript file — UpdatedAt from DB used
	home := setupFakeHome(t)
	updatedAt := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: fixtureConvID,
			Title:          "No transcript",
			UpdatedAt:      updatedAt,
		},
	})
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Preview != "No transcript" {
		t.Errorf("Preview = %q, want 'No transcript'", sessions[0].Preview)
	}
}

func TestList_DBEntryNoTitleUsesTLDR(t *testing.T) {
	// Summary has no title but has TLDR — preview should come from TLDR
	home := setupFakeHome(t)
	addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: fixtureConvID,
			Title:          "",
			TLDR:           "The TLDR summary",
			UpdatedAt:      time.Now(),
		},
	})
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Preview != "The TLDR summary" {
		t.Errorf("Preview = %q, want 'The TLDR summary'", sessions[0].Preview)
	}
}

func TestList_DBWithZeroTimestampUsesFileMTime(t *testing.T) {
	// DB entry with zero UpdatedAt — should fall back to file mtime
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")
	addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: fixtureConvID,
			Title:          "Zero ts",
			// UpdatedAt is zero time
		},
	})
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// UpdatedAt and StartedAt should be set from file mtime (not zero)
	if sessions[0].UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero when DB ts is zero (file mtime used)")
	}
}

func TestList_NoProjectsDir(t *testing.T) {
	// No .cursor/projects — transcripts list is empty, DB may or may not exist.
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestGet_ExactMatch(t *testing.T) {
	home, convID, _ := setupCursorHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sess, err := s.Get(convID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.ID != convID {
		t.Errorf("ID = %q, want %q", sess.ID, convID)
	}
	if sess.Tool != model.ToolCursor {
		t.Errorf("Tool = %q, want cursor", sess.Tool)
	}
	if len(sess.Messages) == 0 {
		t.Error("expected messages, got none")
	}
	// Title comes from DB
	if sess.Title != "Go help" {
		t.Errorf("Title = %q, want 'Go help'", sess.Title)
	}
}

func TestGet_PrefixMatch(t *testing.T) {
	home, convID, _ := setupCursorHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	prefix := convID[:8]
	sess, err := s.Get(prefix)
	if err != nil {
		t.Fatalf("Get(%q) error: %v", prefix, err)
	}
	if sess == nil {
		t.Fatal("expected session for prefix match, got nil")
	}
	if sess.ID != convID {
		t.Errorf("ID = %q, want %q", sess.ID, convID)
	}
}

func TestGet_NotFound(t *testing.T) {
	home := setupFakeHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	_, err := s.Get("nonexistentid")
	if err == nil {
		t.Fatal("expected error for not-found session, got nil")
	}
}

func TestGet_NoTitleDerivesFromMessages(t *testing.T) {
	// No DB, transcript only — title should come from first user message
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nDerived title from user message\n")
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sess, err := s.Get(fixtureConvID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !strings.Contains(sess.Title, "Derived title from user message") {
		t.Errorf("Title = %q, want to contain 'Derived title from user message'", sess.Title)
	}
	// Preview should equal title when summary is empty
	if sess.Preview == "" {
		t.Error("expected non-empty Preview")
	}
}

func TestGet_PreviewFromSummary(t *testing.T) {
	// Title is empty, summary is set via DB — preview should come from summary
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "")
	addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: fixtureConvID,
			Title:          "",
			TLDR:           "the summary text",
			UpdatedAt:      time.Now(),
		},
	})
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sess, err := s.Get(fixtureConvID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !strings.Contains(sess.Preview, "the summary text") {
		t.Errorf("Preview = %q, want to contain 'the summary text'", sess.Preview)
	}
}

func TestSearch_HitAndMiss(t *testing.T) {
	home, _, _ := setupCursorHome(t)
	t.Setenv("HOME", home)

	s := &cursorSource{}

	t.Run("hit", func(t *testing.T) {
		results, err := s.Search("Help me with Go", source.ListOptions{})
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result, got 0")
		}
		if len(results[0].Matches) == 0 {
			t.Fatal("expected matches, got 0")
		}
		snippet := results[0].Matches[0].Snippet
		if !strings.Contains(strings.ToLower(snippet), "help me with go") {
			t.Errorf("snippet %q does not contain query", snippet)
		}
	})

	t.Run("miss", func(t *testing.T) {
		results, err := s.Search("zzznomatchzzz", source.ListOptions{})
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		results, err := s.Search("HELP ME WITH GO", source.ListOptions{})
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected result for uppercase query, got 0")
		}
	})
}

func TestSearch_SessionNotInTranscriptMap(t *testing.T) {
	// DB has a session but no transcript on disk — session appears in List but
	// transcript lookup in Search returns "not ok" → session is skipped.
	home := setupFakeHome(t)
	addTrackingDB(t, home, []conversationSummary{
		{
			ConversationID: fixtureConvID,
			Title:          "No file",
			UpdatedAt:      time.Now(),
		},
	})
	t.Setenv("HOME", home)

	s := &cursorSource{}
	results, err := s.Search("No file", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	// No transcript file → no search results
	if len(results) != 0 {
		t.Errorf("expected 0 results (no transcript), got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Additional branch coverage
// ---------------------------------------------------------------------------

// addWrongSchemaTrackingDB creates a DB where conversation_summaries has
// fewer columns than expected, so Query returns an error on real query.
func addWrongSchemaTrackingDB(t *testing.T, homeDir string) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".cursor", "ai-tracking")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create ai-tracking dir: %v", err)
	}
	dbPath := filepath.Join(dir, "ai-code-tracking.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Table exists but only has conversationId and title — missing required columns.
	_, err = db.Exec(`CREATE TABLE conversation_summaries (conversationId TEXT, title TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO conversation_summaries VALUES ('c1', 'title1')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return dbPath
}

func TestReadConversationSummaries_QueryError(t *testing.T) {
	home := t.TempDir()
	dbPath := addWrongSchemaTrackingDB(t, home)

	_, err := readConversationSummaries(dbPath)
	if err == nil {
		t.Fatal("expected error when querying wrong schema, got nil")
	}
}

func TestList_TrackingDBQueryError(t *testing.T) {
	// DB exists with wrong schema → readConversationSummaries returns error →
	// cursor.go List prints warning to stderr, continues with summaries=nil.
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")
	addWrongSchemaTrackingDB(t, home)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	// Should still return the orphan transcript session despite DB error
	if len(sessions) != 1 {
		t.Errorf("expected 1 session (orphan), got %d", len(sessions))
	}
}

func TestReadAllChatMeta_UnreadableAgentsDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, chmod 000 won't block access")
	}
	home := setupFakeHome(t)
	chatsDir := filepath.Join(home, ".cursor", "chats")

	// Create a workspace dir with restricted permissions so ReadDir on it fails
	wsDir := filepath.Join(chatsDir, "workspace1")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a subdir inside the workspace to make it a valid workspace, but chmod 000
	if err := os.Chmod(wsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(wsDir, 0o755) //nolint:errcheck
	})

	// Should not panic — error reading agents dir is silently skipped
	result := readAllChatMeta(home)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestReadAllChatMeta_StoreDBError(t *testing.T) {
	// store.db exists but has no meta table → readChatStoreMeta returns error → skipped
	home := setupFakeHome(t)
	wsDir := filepath.Join(home, ".cursor", "chats", "workspace1", fixtureConvID)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(wsDir, "store.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// No meta table — readChatStoreMeta will fail QueryRow
	db.Exec("CREATE TABLE dummy (x TEXT)") //nolint:errcheck
	db.Close()

	result := readAllChatMeta(home)
	if len(result) != 0 {
		t.Errorf("expected 0 entries (store.db error skipped), got %d", len(result))
	}
}

func TestFindTranscriptFile_NonDirEntry(t *testing.T) {
	// Projects dir contains a file (not a dir) — should be skipped.
	// Name it with an "A" prefix so it sorts BEFORE the project dir name
	// ("A" < "U"), ensuring the non-dir entry is encountered before the match.
	home := setupFakeHome(t)
	projectsDir := filepath.Join(home, ".cursor", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a regular file that sorts before fixtureProjDirName alphabetically.
	if err := os.WriteFile(filepath.Join(projectsDir, "AAA-not-a-dir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also add a real project
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")

	_, filePath := findTranscriptFile(home, fixtureConvID)
	if filePath == "" {
		t.Fatal("expected to find transcript file, got empty")
	}
}

func TestListAllTranscripts_NonDirProjectEntry(t *testing.T) {
	// Projects dir contains a regular file — should be skipped.
	// Name with "A" prefix so it sorts before the project dir.
	home := setupFakeHome(t)
	projectsDir := filepath.Join(home, ".cursor", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a regular file entry under projects/ (sorts before fixtureProjDirName)
	if err := os.WriteFile(filepath.Join(projectsDir, "AAA-file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also add a valid project
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")

	entries := listAllTranscripts(home)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (non-dir project skipped), got %d", len(entries))
	}
}

func TestListAllTranscripts_DirEntryInTranscriptsDir(t *testing.T) {
	// agent-transcripts contains a subdirectory — should be skipped.
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")
	// Add a subdirectory inside agent-transcripts
	subDir := filepath.Join(home, ".cursor", "projects", fixtureProjDirName, "agent-transcripts", "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	entries := listAllTranscripts(home)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (subdir skipped), got %d", len(entries))
	}
}

func TestListAllTranscripts_NoAgentTranscriptsDir(t *testing.T) {
	// Project dir exists but no agent-transcripts subdir — os.ReadDir fails → continue.
	home := setupFakeHome(t)
	projectsDir := filepath.Join(home, ".cursor", "projects")
	projDir := filepath.Join(projectsDir, fixtureProjDirName)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No agent-transcripts subdir
	entries := listAllTranscripts(home)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestGet_ParseTranscriptError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, chmod 000 won't block access")
	}
	// Transcript file exists but is unreadable — parseTranscript returns error.
	home := setupFakeHome(t)
	path := addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")

	// Make file unreadable after creation
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) //nolint:errcheck

	t.Setenv("HOME", home)

	s := &cursorSource{}
	_, err := s.Get(fixtureConvID)
	if err == nil {
		t.Fatal("expected error for unreadable transcript, got nil")
	}
}

func TestSearch_ParseTranscriptError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, chmod 000 won't block access")
	}
	// Transcript exists and is in List results, but unreadable by Search.
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello world\n")
	t.Setenv("HOME", home)

	// Run List first to confirm session appears
	s := &cursorSource{}
	sessions, _ := s.List(source.ListOptions{})
	if len(sessions) != 1 {
		t.Fatalf("setup: expected 1 session, got %d", len(sessions))
	}

	// Now make the transcript unreadable
	transcriptPath := filepath.Join(home, ".cursor", "projects", fixtureProjDirName, "agent-transcripts", fixtureConvID+".txt")
	if err := os.Chmod(transcriptPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(transcriptPath, 0o644) }) //nolint:errcheck

	results, err := s.Search("hello world", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	// Session is skipped when parseTranscript fails
	if len(results) != 0 {
		t.Errorf("expected 0 results (transcript unreadable), got %d", len(results))
	}
}

func TestList_OrphanFilterExcluded(t *testing.T) {
	// Orphan transcript that is excluded by matchesFilter.
	home := setupFakeHome(t)
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "user:\nhello\n")
	t.Setenv("HOME", home)

	s := &cursorSource{}
	// Use very short Since to exclude the old file
	sessions, err := s.List(source.ListOptions{Since: 1})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions (orphan filtered by Since), got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// os.UserHomeDir() error paths
// ---------------------------------------------------------------------------

func TestList_HomeDir_Error(t *testing.T) {
	// Setting HOME="" causes os.UserHomeDir() to return an error on Unix/macOS.
	t.Setenv("HOME", "")
	s := &cursorSource{}
	_, err := s.List(source.ListOptions{})
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestGet_HomeDir_Error(t *testing.T) {
	t.Setenv("HOME", "")
	s := &cursorSource{}
	_, err := s.Get("anyid")
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestSearch_HomeDir_Error(t *testing.T) {
	t.Setenv("HOME", "")
	s := &cursorSource{}
	_, err := s.Search("query", source.ListOptions{})
	if err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestProjectPathFromDir_HomeDir_Error(t *testing.T) {
	// os.UserHomeDir() error → fallback naive decode
	t.Setenv("HOME", "")
	// Any encoded dir name — should fall back to naive decode
	got := projectPathFromDir("Users-foo-bar")
	// Naive decode: "/" + strings.ReplaceAll("Users-foo-bar", "-", "/")
	if got != "/Users/foo/bar" {
		t.Errorf("projectPathFromDir with no HOME = %q, want %q", got, "/Users/foo/bar")
	}
}

func TestList_OrphanChatMetaNameNoPreview(t *testing.T) {
	// Orphan transcript where parseTranscript yields only assistant messages
	// (no user messages → preview=""), but chatMeta has a name.
	// Covers the `sess.Preview == ""` branch in the chatMeta enrichment block.
	home := setupFakeHome(t)
	// Transcript with only assistant content — no user message
	addTranscriptFile(t, home, fixtureProjDirName, fixtureConvID, "A:\nOnly assistant response here.\n")
	meta := chatMeta{
		AgentID:   fixtureConvID,
		Name:      "Session from chatMeta",
		CreatedAt: time.Now().UnixMilli(),
		Model:     "gpt-4",
	}
	addChatStoreDB(t, home, "ws1", fixtureConvID, meta)
	t.Setenv("HOME", home)

	s := &cursorSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// Preview should come from chatMeta.Name since transcript had no user messages
	if sessions[0].Preview != "Session from chatMeta" {
		t.Errorf("Preview = %q, want 'Session from chatMeta'", sessions[0].Preview)
	}
}

// ---------------------------------------------------------------------------
// readConversationSummaries — scan error (NULL conversationId)
// ---------------------------------------------------------------------------

// TestReadConversationSummaries_ScanError verifies that a NULL value in the
// conversationId column causes rows.Scan to fail, returning a
// "scan conversation_summaries row" error.
func TestReadConversationSummaries_ScanError(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".cursor", "ai-tracking")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "ai-code-tracking.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE conversation_summaries (
		conversationId TEXT,
		title         TEXT,
		tldr          TEXT,
		overview      TEXT,
		model         TEXT,
		mode          TEXT,
		updatedAt     INTEGER
	)`)
	if err != nil {
		db.Close()
		t.Fatalf("create table: %v", err)
	}
	// NULL conversationId: scanning into plain string fails.
	_, err = db.Exec(`INSERT INTO conversation_summaries VALUES (NULL,'t','','','m','c',0)`)
	if err != nil {
		db.Close()
		t.Fatalf("insert: %v", err)
	}
	db.Close()

	_, err = readConversationSummaries(dbPath)
	if err == nil {
		t.Fatal("expected scan error for NULL conversationId, got nil")
	}
	if !strings.Contains(err.Error(), "scan conversation_summaries row") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// parseTranscript — scanner error (line > 1 MB)
// ---------------------------------------------------------------------------

// TestParseTranscript_ScannerError verifies that a transcript line exceeding
// the scanner's 1 MB buffer limit causes scanner.Err() to return an error.
func TestParseTranscript_ScannerError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big-transcript.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Write one content line longer than 1 MB (scanner buffer limit = 1024*1024).
	bigLine := strings.Repeat("a", 1024*1024+1)
	if _, err := f.WriteString("user:\n" + bigLine + "\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, err = parseTranscript(path)
	if err == nil {
		t.Fatal("expected scanner error for line > 1 MB, got nil")
	}
}
