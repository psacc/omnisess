package cmd

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/psacc/omnisess/internal/index"
	"github.com/psacc/omnisess/internal/model"
)

// dropTableRaw connects to dbPath via the modernc.org/sqlite driver (not
// through index.Open which would re-create tables) and drops the given table.
// Used to corrupt the index for error-path tests.
func dropTableRaw(t *testing.T, dbPath, table string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("DROP TABLE " + table); err != nil {
		t.Fatalf("drop %s: %v", table, err)
	}
}

func resetStatsFlags() {
	flagStatsSession = ""
	flagStatsWindow = ""
	flagStatsTool = ""
	flagStatsFull = false
	flagStatsJSON = false
}

func TestRunStats_MutexConflict(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsSession = "abc"
	flagStatsWindow = "7d"
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err == nil {
		t.Errorf("expected mutex error")
	} else if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunStats_InvalidWindow(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "not-a-duration"
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err == nil {
		t.Errorf("expected invalid-window error")
	}
}

func TestRunStats_SessionNotFound(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsSession = "claude:no-such-session"
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err == nil {
		t.Errorf("expected session-not-found error")
	} else if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunStats_OpenIndexError(t *testing.T) {
	t.Setenv("OMNISESS_INDEX_PATH", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "1d"
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err == nil {
		t.Errorf("expected open-index error")
	}
}

func TestRunStats_WindowEmpty(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "1d"
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Errorf("runStatsTo: %v", err)
	}
	if !strings.Contains(w.String(), "Sessions: 0") {
		t.Errorf("expected zero sessions: %q", w.String())
	}
}

func TestRunStats_WindowDefault7d(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	// No --session, no --window → default 7d
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Errorf("runStatsTo: %v", err)
	}
	if !strings.Contains(w.String(), "Window: 7d") {
		t.Errorf("expected default 7d window: %q", w.String())
	}
}

func TestRunStats_WindowJSON(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "1d"
	flagStatsJSON = true
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Errorf("runStatsTo: %v", err)
	}
	var got index.AggregateStats
	if err := json.Unmarshal([]byte(w.String()), &got); err != nil {
		t.Errorf("invalid JSON output: %v\n%q", err, w.String())
	}
}

func TestRunStats_WindowToolFilter(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "1d"
	flagStatsTool = "claude"
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Errorf("runStatsTo: %v", err)
	}
}

func TestRunStats_SessionHappyPath(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	resetFlags()
	resetStatsFlags()
	flagStatsSession = "claude:" + makeUUID(0)
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Fatalf("runStatsTo: %v", err)
	}
	if !strings.Contains(w.String(), "Session: "+makeUUID(0)) {
		t.Errorf("expected session header: %q", w.String())
	}
	if !strings.Contains(w.String(), "Tokens:") {
		t.Errorf("expected tokens line: %q", w.String())
	}
}

func TestRunStats_SessionHappyPath_JSON(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	resetFlags()
	resetStatsFlags()
	flagStatsSession = "claude:" + makeUUID(0)
	flagStatsJSON = true
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Fatalf("runStatsTo: %v", err)
	}
	var got index.SessionStats
	if err := json.Unmarshal([]byte(w.String()), &got); err != nil {
		t.Errorf("invalid JSON: %v\n%q", err, w.String())
	}
	if got.ConversationID != makeUUID(0) {
		t.Errorf("ConversationID = %q", got.ConversationID)
	}
}

// TestRunStats_SessionWithoutPrefix exercises splitQualifiedID's no-colon branch.
func TestRunStats_SessionWithoutPrefix(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	resetFlags()
	resetStatsFlags()
	flagStatsSession = makeUUID(0)
	var w, wErr strings.Builder
	if err := runStatsTo(&w, &wErr); err != nil {
		t.Fatalf("runStatsTo: %v", err)
	}
}

func TestRunStats_CobraAdapter(t *testing.T) {
	withTempIndex(t)
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "1d"
	silenceOutput(t)
	if err := runStats(newNoopCmd(), nil); err != nil {
		t.Errorf("runStats: %v", err)
	}
}

// TestSplitQualifiedID covers the helper directly.
func TestSplitQualifiedID(t *testing.T) {
	cases := []struct {
		in       string
		wantTool model.Tool
		wantID   string
	}{
		{"claude:abc", model.ToolClaude, "abc"},
		{"cursor:xyz", model.ToolCursor, "xyz"},
		{"plain-id", model.ToolClaude, "plain-id"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			tool, id := splitQualifiedID(c.in)
			if tool != c.wantTool || id != c.wantID {
				t.Errorf("splitQualifiedID(%q) = %q,%q want %q,%q", c.in, tool, id, c.wantTool, c.wantID)
			}
		})
	}
}

func TestWriteSessionStatsText(t *testing.T) {
	s := &index.SessionStats{
		ConversationID: "c1",
		ProviderName:   "anthropic",
		RequestModel:   "claude-opus-4-7",
		StartedAt:      time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 5, 15, 9, 5, 0, 0, time.UTC),
		Tools:          []index.ToolCount{{Name: "Read", Count: 3, ErrorCount: 1}},
		Files: []index.FileActivity{
			{Path: "/a.go", Reads: 2},
			{Path: "/b.go", Edits: 1, LinesAdded: 2, LinesRemoved: 1},
		},
	}
	var w strings.Builder
	writeSessionStatsText(&w, s)
	out := w.String()
	for _, want := range []string{"Session: c1", "anthropic", "claude-opus-4-7",
		"Read", "/a.go", "/b.go", "Started:", "Updated:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteSessionStatsText_Empty(t *testing.T) {
	// Empty stats: no tools, no files, no timestamps — should still render header.
	s := &index.SessionStats{ConversationID: "empty"}
	var w strings.Builder
	writeSessionStatsText(&w, s)
	out := w.String()
	if !strings.Contains(out, "Session: empty") {
		t.Errorf("expected session header: %q", out)
	}
	if strings.Contains(out, "Started:") || strings.Contains(out, "Updated:") {
		t.Errorf("zero-time fields should be omitted: %q", out)
	}
	if strings.Contains(out, "Tools") {
		t.Errorf("empty tools should not render table header: %q", out)
	}
	if strings.Contains(out, "Files") {
		t.Errorf("empty files should not render table header: %q", out)
	}
}

func TestWriteAggregateStatsText(t *testing.T) {
	a := &index.AggregateStats{
		WindowStart:      time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		WindowEnd:        time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
		SessionCount:     3,
		ToolCallCount:    10,
		ErrorCount:       1,
		TotalInputTokens: 100,
		Tools:            []index.ToolCount{{Name: "Read", Count: 5}},
		Files:            []index.FileActivity{{Path: "/a", Reads: 5}},
	}
	var w strings.Builder
	writeAggregateStatsText(&w, a, "7d")
	out := w.String()
	for _, want := range []string{"Window: 7d", "Sessions: 3", "Tools", "Files", "Read", "/a"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteAggregateStatsText_Empty(t *testing.T) {
	a := &index.AggregateStats{
		WindowStart: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
	}
	var w strings.Builder
	writeAggregateStatsText(&w, a, "7d")
	out := w.String()
	if strings.Contains(out, "Tools\n") {
		t.Errorf("empty tools should not render: %q", out)
	}
	if strings.Contains(out, "Files\n") {
		t.Errorf("empty files should not render: %q", out)
	}
}

func TestEmptyDash(t *testing.T) {
	if emptyDash("") != "-" {
		t.Errorf("emptyDash empty = %q", emptyDash(""))
	}
	if emptyDash("x") != "x" {
		t.Errorf("emptyDash non-empty = %q", emptyDash("x"))
	}
}

// TestLazyIndexOne covers the lazy-populate helper used by runStatsWindow.
func TestLazyIndexOne_GetError(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var wErr strings.Builder
	lazyIndexOne(&wErr, &getErrSource{}, idx, "x", false)
	// No assertion on output; just ensure no panic.
}

func TestLazyIndexOne_NoFilePath(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var wErr strings.Builder
	lazyIndexOne(&wErr, &getSessionSource{}, idx, "no-such-id", false)
	// no panic.
}

func TestLazyIndexOne_EnsureSessionError(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	_ = idx.Close()
	var wErr strings.Builder
	lazyIndexOne(&wErr, &fakeClaudeSrc{}, idx, makeUUID(0), false)
	if !strings.Contains(wErr.String(), "lazy-index") {
		t.Errorf("expected lazy-index warning: %q", wErr.String())
	}
}

// TestLazyIndexWindow_ListError covers the "warning: list claude sessions"
// branch via an errSource mock.
func TestLazyIndexWindow_ListError(t *testing.T) {
	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	var wErr strings.Builder
	lazyIndexWindow(&wErr, &errSource{}, idx, time.Hour, false)
	if !strings.Contains(wErr.String(), "warning: list claude sessions") {
		t.Errorf("expected list warning: %q", wErr.String())
	}
}

// TestLazyIndexWindow_Happy covers the happy path including the per-session
// loop body.
func TestLazyIndexWindow_Happy(t *testing.T) {
	withTempIndex(t)
	seedFakeClaudeSessions(t, 1)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	src := &fakeClaudeMultiSrc{ids: []string{makeUUID(0)}}
	var wErr strings.Builder
	lazyIndexWindow(&wErr, src, idx, time.Hour, false)
}

// TestRunStatsWindow_QueryError forces QueryWindow to error by calling
// runStatsWindow directly against an idx whose session_metadata has been
// dropped. Going via runStatsTo would re-migrate it back.
func TestRunStatsWindow_QueryError(t *testing.T) {
	dbPath := withTempIndex(t)
	idx, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// idx still references the live DB; drop the table via a separate handle
	// (the test helper opens its own sql.DB to avoid touching the index).
	dropTableRaw(t, dbPath, "session_metadata")
	defer idx.Close()
	resetFlags()
	resetStatsFlags()
	flagStatsWindow = "1d"
	var w, wErr strings.Builder
	if err := runStatsWindow(&w, &wErr, idx); err == nil {
		t.Errorf("expected query window error after dropping table")
	}
}

// TestRunStatsSession_QueryError forces QuerySession to error by dropping
// session_metadata in the live idx handle.
func TestRunStatsSession_QueryError(t *testing.T) {
	dbPath := withTempIndex(t)
	idx, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	dropTableRaw(t, dbPath, "session_metadata")
	defer idx.Close()
	resetFlags()
	resetStatsFlags()
	flagStatsSession = "claude:abc"
	var w, wErr strings.Builder
	if err := runStatsSession(&w, &wErr, idx); err == nil {
		t.Errorf("expected query session error after dropping table")
	}
}

func TestWriteJSON_Error(t *testing.T) {
	// json.Marshal on a chan fails. Indirectly cover the error wrap.
	var w strings.Builder
	if err := writeJSON(&w, make(chan int)); err == nil {
		t.Errorf("expected encode error")
	}
}
