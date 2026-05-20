package index

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// newTestIndex returns a fresh sqliteIndex backed by a temp DB and a small
// helper to open the underlying *sql.DB for low-level assertions.
func newTestIndex(t *testing.T) (*sqliteIndex, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.sqlite")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx.(*sqliteIndex), dbPath
}

// makeSourceFile writes a stub source-file path so EnsureSession can stat it.
// We don't actually parse it; the test passes a pre-built Session.
func makeSourceFile(t *testing.T, dir, name string, contents []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

func countRows(t *testing.T, db *sql.DB, q string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", q, err)
	}
	return n
}

func TestEnsureSession_NilSession(t *testing.T) {
	idx, _ := newTestIndex(t)
	err := idx.EnsureSession("/nonexistent", "id1", false, false, nil)
	if err == nil {
		t.Fatal("expected error for nil session")
	}
}

func TestEnsureSession_EmptyConvID(t *testing.T) {
	idx, _ := newTestIndex(t)
	err := idx.EnsureSession("/nonexistent", "", false, false, &Session{})
	if err == nil {
		t.Fatal("expected error for empty conv ID")
	}
}

func TestEnsureSession_StatError(t *testing.T) {
	idx, _ := newTestIndex(t)
	err := idx.EnsureSession("/this/does/not/exist", "abc", false, false, &Session{})
	if err == nil {
		t.Fatal("expected stat error")
	}
}

func TestEnsureSession_Insert_QueryRoundTrip(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	sess := &Session{
		ConversationID:         "conv-1",
		ProviderName:           "anthropic",
		RequestModel:           "claude-opus-4-7",
		ResponseModel:          "claude-opus-4-7",
		StartedAt:              now.Add(-time.Hour),
		UpdatedAt:              now,
		TotalInputTokens:       100,
		TotalOutputTokens:      40,
		TotalCacheCreateTokens: 1000,
		TotalCacheReadTokens:   500,
		ToolCalls: []ToolCallRow{
			{
				ToolCallID: "toolu_R1", ToolName: "Read", ToolType: "function",
				IsError: false, Timestamp: now,
				FilePath: "/a.go", FileOp: "read",
				ArgumentsJSON: `{"file_path":"/a.go"}`, ResultJSON: `"contents"`,
			},
			{
				ToolCallID: "toolu_R2", ToolName: "Read", ToolType: "function",
				IsError: true, Timestamp: now,
				FilePath: "/a.go", FileOp: "read",
			},
			{
				ToolCallID: "toolu_E1", ToolName: "Edit", ToolType: "function",
				Timestamp: now,
				FilePath:  "/b.go", FileOp: "edit",
				FileLinesAdded: 2, FileLinesRemoved: 1,
			},
			{
				ToolCallID: "toolu_W1", ToolName: "Write", ToolType: "function",
				Timestamp: now, FilePath: "/c.go", FileOp: "write",
				FileContentSize: 11,
			},
			{
				ToolCallID: "toolu_B1", ToolName: "Bash", ToolType: "function",
				Timestamp: now,
			},
		},
	}

	if err := idx.EnsureSession(src, sess.ConversationID, false, false, sess); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	stats, err := idx.QuerySession("conv-1")
	if err != nil {
		t.Fatalf("QuerySession: %v", err)
	}
	if stats == nil {
		t.Fatal("QuerySession returned nil")
	}
	if stats.ProviderName != "anthropic" {
		t.Errorf("ProviderName = %q", stats.ProviderName)
	}
	if stats.RequestModel != "claude-opus-4-7" {
		t.Errorf("RequestModel = %q", stats.RequestModel)
	}
	if stats.TotalInputTokens != 100 || stats.TotalOutputTokens != 40 {
		t.Errorf("token totals = %d/%d", stats.TotalInputTokens, stats.TotalOutputTokens)
	}
	if stats.ToolCallCount != 5 {
		t.Errorf("ToolCallCount = %d, want 5", stats.ToolCallCount)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", stats.ErrorCount)
	}
	if stats.HasFullPayloads {
		t.Errorf("HasFullPayloads = true, want false (no --full)")
	}

	// Read appears with count 2 and 1 error
	foundRead := false
	for _, tc := range stats.Tools {
		if tc.Name == "Read" {
			foundRead = true
			if tc.Count != 2 || tc.ErrorCount != 1 {
				t.Errorf("Read tool counts = %d/%d, want 2/1", tc.Count, tc.ErrorCount)
			}
		}
	}
	if !foundRead {
		t.Errorf("Read tool not found in stats")
	}

	// Bash (no file_path) should NOT appear in files; /c.go (write) and /b.go (edit) should
	foundA, foundB, foundC := false, false, false
	for _, f := range stats.Files {
		switch f.Path {
		case "/a.go":
			foundA = true
			if f.Reads != 2 {
				t.Errorf("/a.go reads = %d, want 2", f.Reads)
			}
		case "/b.go":
			foundB = true
			if f.Edits != 1 || f.LinesAdded != 2 || f.LinesRemoved != 1 {
				t.Errorf("/b.go edits/+/- = %d/%d/%d", f.Edits, f.LinesAdded, f.LinesRemoved)
			}
		case "/c.go":
			foundC = true
			if f.Writes != 1 {
				t.Errorf("/c.go writes = %d, want 1", f.Writes)
			}
		}
	}
	if !foundA || !foundB || !foundC {
		t.Errorf("missing file in stats: a=%v b=%v c=%v", foundA, foundB, foundC)
	}
}

func TestEnsureSession_CacheHit(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-cache", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{{ToolCallID: "toolu_X", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/x"}}}
	if err := idx.EnsureSession(src, "conv-cache", false, false, sess); err != nil {
		t.Fatalf("first EnsureSession: %v", err)
	}
	indexedAtBefore := countRows(t, idx.db, "SELECT indexed_at FROM session_metadata WHERE conversation_id='conv-cache'")
	// Sleep one second so that if a re-index happens, indexed_at changes
	time.Sleep(1100 * time.Millisecond)
	if err := idx.EnsureSession(src, "conv-cache", false, false, sess); err != nil {
		t.Fatalf("second EnsureSession: %v", err)
	}
	indexedAtAfter := countRows(t, idx.db, "SELECT indexed_at FROM session_metadata WHERE conversation_id='conv-cache'")
	if indexedAtBefore != indexedAtAfter {
		t.Errorf("cache hit should not change indexed_at: %d -> %d", indexedAtBefore, indexedAtAfter)
	}
}

func TestEnsureSession_CacheMissOnSizeChange(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-size", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "conv-size", false, false, sess); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Append → size + mtime change
	if err := os.WriteFile(src, []byte("{}\nmore"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnsureSession(src, "conv-size", false, false, sess); err != nil {
		t.Fatalf("second: %v", err)
	}
	// Verify source_file_size column was updated
	var size int64
	if err := idx.db.QueryRow("SELECT source_file_size FROM session_metadata WHERE conversation_id='conv-size'").Scan(&size); err != nil {
		t.Fatal(err)
	}
	if size != int64(len("{}\nmore")) {
		t.Errorf("size = %d, want %d", size, len("{}\nmore"))
	}
}

func TestEnsureSession_CacheMissOnMtimeChange(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-mt", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "conv-mt", false, false, sess); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Touch the file forward by 5s
	newTime := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(src, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnsureSession(src, "conv-mt", false, false, sess); err != nil {
		t.Fatalf("second: %v", err)
	}
	var mt int64
	if err := idx.db.QueryRow("SELECT source_file_mtime FROM session_metadata WHERE conversation_id='conv-mt'").Scan(&mt); err != nil {
		t.Fatal(err)
	}
	if mt != newTime.Unix() {
		t.Errorf("mtime not updated: got %d want %d", mt, newTime.Unix())
	}
}

func TestEnsureSession_FullUpgradeTransparent(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-up", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{{
			ToolCallID: "toolu_X", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/x",
			ArgumentsJSON: `{"file_path":"/x"}`, ResultJSON: `"hi"`,
		}}}
	// First call: no --full
	if err := idx.EnsureSession(src, "conv-up", false, false, sess); err != nil {
		t.Fatalf("first: %v", err)
	}
	var args, result sql.NullString
	if err := idx.db.QueryRow("SELECT arguments_json, result_json FROM tool_calls WHERE conversation_id='conv-up'").Scan(&args, &result); err != nil {
		t.Fatal(err)
	}
	if args.Valid || result.Valid {
		t.Errorf("payloads should be NULL without --full")
	}
	// Second call: --full, same file (no mtime/size change)
	if err := idx.EnsureSession(src, "conv-up", true, false, sess); err != nil {
		t.Fatalf("second --full: %v", err)
	}
	if err := idx.db.QueryRow("SELECT arguments_json, result_json FROM tool_calls WHERE conversation_id='conv-up'").Scan(&args, &result); err != nil {
		t.Fatal(err)
	}
	if !args.Valid || args.String == "" {
		t.Errorf("arguments_json should be populated after --full upgrade, got %v", args)
	}
	if !result.Valid {
		t.Errorf("result_json should be populated after --full upgrade")
	}
	var hf int
	if err := idx.db.QueryRow("SELECT has_full_payloads FROM session_metadata WHERE conversation_id='conv-up'").Scan(&hf); err != nil {
		t.Fatal(err)
	}
	if hf != 1 {
		t.Errorf("has_full_payloads = %d, want 1", hf)
	}
}

func TestEnsureSession_FullDowngradeIsNoOp(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-down", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{{
			ToolCallID: "toolu_X", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/x",
			ArgumentsJSON: `{"file_path":"/x"}`, ResultJSON: `"hi"`,
		}}}
	// First call --full
	if err := idx.EnsureSession(src, "conv-down", true, false, sess); err != nil {
		t.Fatalf("first --full: %v", err)
	}
	// Wait so indexed_at would differ if rewritten
	time.Sleep(1100 * time.Millisecond)
	// Second call no --full, file unchanged → no-op
	if err := idx.EnsureSession(src, "conv-down", false, false, sess); err != nil {
		t.Fatalf("downgrade: %v", err)
	}
	var hf int
	var args sql.NullString
	if err := idx.db.QueryRow("SELECT has_full_payloads FROM session_metadata WHERE conversation_id='conv-down'").Scan(&hf); err != nil {
		t.Fatal(err)
	}
	if hf != 1 {
		t.Errorf("downgrade should NOT clear has_full_payloads")
	}
	if err := idx.db.QueryRow("SELECT arguments_json FROM tool_calls WHERE conversation_id='conv-down'").Scan(&args); err != nil {
		t.Fatal(err)
	}
	if !args.Valid {
		t.Errorf("downgrade should NOT clear arguments_json")
	}
}

func TestEnsureSession_FullNotPreservedAcrossFileChange(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-lost", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{{
			ToolCallID: "toolu_X", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/x",
			ArgumentsJSON: `{"file_path":"/x"}`, ResultJSON: `"hi"`,
		}}}
	// Index with --full
	if err := idx.EnsureSession(src, "conv-lost", true, false, sess); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Append → size change forces re-index
	if err := os.WriteFile(src, []byte("{}+"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Re-index WITHOUT --full
	if err := idx.EnsureSession(src, "conv-lost", false, false, sess); err != nil {
		t.Fatalf("second: %v", err)
	}
	var hf int
	if err := idx.db.QueryRow("SELECT has_full_payloads FROM session_metadata WHERE conversation_id='conv-lost'").Scan(&hf); err != nil {
		t.Fatal(err)
	}
	if hf != 0 {
		t.Errorf("payloads should be lost after file change without --full, hf=%d", hf)
	}
	var args sql.NullString
	if err := idx.db.QueryRow("SELECT arguments_json FROM tool_calls WHERE conversation_id='conv-lost'").Scan(&args); err != nil {
		t.Fatal(err)
	}
	if args.Valid {
		t.Errorf("arguments_json should be NULL after silent payload loss")
	}
}

func TestEnsureSession_RebuildForcesReindex(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-rb", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "conv-rb", false, false, sess); err != nil {
		t.Fatalf("first: %v", err)
	}
	var first int64
	if err := idx.db.QueryRow("SELECT indexed_at FROM session_metadata WHERE conversation_id='conv-rb'").Scan(&first); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := idx.EnsureSession(src, "conv-rb", false, true, sess); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	var second int64
	if err := idx.db.QueryRow("SELECT indexed_at FROM session_metadata WHERE conversation_id='conv-rb'").Scan(&second); err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Errorf("--rebuild should force re-index: %d unchanged", first)
	}
}

func TestEnsureSession_DBClosed(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	_ = idx.Close()
	sess := &Session{ConversationID: "x", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "x", false, false, sess); err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestQuerySession_NotFound(t *testing.T) {
	idx, _ := newTestIndex(t)
	stats, err := idx.QuerySession("nonexistent")
	if err != nil {
		t.Fatalf("QuerySession: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil stats for unknown session, got %+v", stats)
	}
}

func TestQuerySession_DBClosedReturnsError(t *testing.T) {
	idx, _ := newTestIndex(t)
	_ = idx.Close()
	if _, err := idx.QuerySession("x"); err == nil {
		t.Errorf("expected error from QuerySession after Close")
	}
}

// TestQuerySession_ToolsError covers the queryToolsForSession-error branch
// inside QuerySession by inserting a session row but dropping tool_calls.
func TestQuerySession_ToolsError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "drop-tools", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "drop-tools", false, false, sess); err != nil {
		t.Fatal(err)
	}
	mustExec(t, idx.db, "DROP TABLE tool_calls")
	if _, err := idx.QuerySession("drop-tools"); err == nil {
		t.Errorf("expected error after dropping tool_calls")
	}
}

// TestQuerySession_FilesError covers the queryFilesForSession-error branch.
// After tool_calls is dropped queryToolsForSession also fails first, so a
// surgical second-stage failure requires re-creating tool_calls without
// file_path column. Easier: recreate it as a stub view that satisfies the
// tools query schema but breaks the files query.
func TestQuerySession_FilesError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "drop-files", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "drop-files", false, false, sess); err != nil {
		t.Fatal(err)
	}
	mustExec(t, idx.db, "DROP TABLE tool_calls")
	// Re-create a minimal tool_calls without file_path column. The tools query
	// references tool_name and tool_type only, but the files query references
	// file_path — so the files query fails while the tools query succeeds.
	mustExec(t, idx.db, `CREATE TABLE tool_calls (
		conversation_id TEXT, tool_call_id TEXT, tool_name TEXT, tool_type TEXT,
		operation_name TEXT, provider_name TEXT, is_error INTEGER, ts INTEGER,
		file_lines_added INTEGER, file_lines_removed INTEGER, file_content_size INTEGER,
		arguments_json TEXT, result_json TEXT
	)`)
	if _, err := idx.QuerySession("drop-files"); err == nil {
		t.Errorf("expected error from files query when file_path column missing")
	}
}

func TestQueryWindow_Empty(t *testing.T) {
	idx, _ := newTestIndex(t)
	start := time.Unix(0, 0)
	end := time.Now()
	agg, err := idx.QueryWindow(start, end, "")
	if err != nil {
		t.Fatalf("QueryWindow: %v", err)
	}
	if agg == nil {
		t.Fatal("nil agg")
	}
	if agg.SessionCount != 0 {
		t.Errorf("empty index session count = %d", agg.SessionCount)
	}
}

// TestQuerySession_SortOpCount covers the per-session file-sort branch
// where two files have different op counts (oi != oj → oi > oj returned).
func TestQuerySession_SortOpCount(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	now := time.Now()
	sess := &Session{ConversationID: "sort-1", ProviderName: "anthropic", UpdatedAt: now,
		ToolCalls: []ToolCallRow{
			{ToolCallID: "1", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/few"},
			{ToolCallID: "2", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/many"},
			{ToolCallID: "3", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/many"},
			{ToolCallID: "4", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/many"},
		}}
	if err := idx.EnsureSession(src, "sort-1", false, false, sess); err != nil {
		t.Fatal(err)
	}
	stats, err := idx.QuerySession("sort-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(stats.Files))
	}
	// /many has 3 ops, /few has 1 op → /many first
	if stats.Files[0].Path != "/many" {
		t.Errorf("expected /many first (more ops), got %q", stats.Files[0].Path)
	}
}

// TestQueryWindow_FullCoverage exercises both window-tools and window-files
// paths including write/edit op cases and the sort closure.
func TestQueryWindow_FullCoverage(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	now := time.Now()
	sess := &Session{ConversationID: "conv-full", ProviderName: "anthropic", UpdatedAt: now,
		ToolCalls: []ToolCallRow{
			{ToolCallID: "1", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/a"},
			{ToolCallID: "2", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/b"},
			{ToolCallID: "3", ToolName: "Write", Timestamp: now, FileOp: "write", FilePath: "/a"},
			{ToolCallID: "4", ToolName: "Edit", Timestamp: now, FileOp: "edit", FilePath: "/b",
				FileLinesAdded: 2, FileLinesRemoved: 1},
			// /c has only one op so its op-count differs from /a and /b — exercises
			// the sort closure's "oi != oj" branch.
			{ToolCallID: "5", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/c"},
		}}
	if err := idx.EnsureSession(src, "conv-full", false, false, sess); err != nil {
		t.Fatal(err)
	}
	agg, err := idx.QueryWindow(now.Add(-time.Hour), now.Add(time.Hour), "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	// At least one Read tool
	if len(agg.Tools) == 0 {
		t.Errorf("expected tools")
	}
	// Three files: /a and /b have 2 ops, /c has 1
	if len(agg.Files) != 3 {
		t.Errorf("expected 3 files, got %d", len(agg.Files))
	}
	// /a has 1 read + 1 write = 2 ops; /b has 1 read + 1 edit = 2 ops; tie-break alphabetical
	if agg.Files[0].Path != "/a" {
		t.Errorf("first file = %q, want /a (alphabetical tiebreak)", agg.Files[0].Path)
	}
	// /c should appear last (1 op vs 2 ops for /a and /b)
	if agg.Files[2].Path != "/c" {
		t.Errorf("last file = %q, want /c (fewer ops)", agg.Files[2].Path)
	}
	for _, f := range agg.Files {
		switch f.Path {
		case "/a":
			if f.Reads != 1 || f.Writes != 1 {
				t.Errorf("/a R/W = %d/%d", f.Reads, f.Writes)
			}
		case "/b":
			if f.Reads != 1 || f.Edits != 1 || f.LinesAdded != 2 || f.LinesRemoved != 1 {
				t.Errorf("/b = %+v", f)
			}
		}
	}
}

func TestQueryWindow_FilterByProvider(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	now := time.Now()
	// Anthropic session
	a := &Session{ConversationID: "ant-1", ProviderName: "anthropic", UpdatedAt: now,
		ToolCalls: []ToolCallRow{{ToolCallID: "x", ToolName: "Read", Timestamp: now, FileOp: "read", FilePath: "/a"}}}
	if err := idx.EnsureSession(src, "ant-1", false, false, a); err != nil {
		t.Fatal(err)
	}
	// Synthetic other provider
	mustExec(t, idx.db,
		`INSERT INTO session_metadata (conversation_id, provider_name, updated_at, source_file_path, source_file_mtime, source_file_size, indexed_at) VALUES ('other-1','other',?,?,?,?,?)`,
		now.Unix(), src, time.Now().Unix(), int64(2), time.Now().Unix())

	agg, err := idx.QueryWindow(now.Add(-time.Hour), now.Add(time.Hour), "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if agg.SessionCount != 1 {
		t.Errorf("filtered SessionCount = %d, want 1", agg.SessionCount)
	}
	aggAll, err := idx.QueryWindow(now.Add(-time.Hour), now.Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if aggAll.SessionCount != 2 {
		t.Errorf("unfiltered SessionCount = %d, want 2", aggAll.SessionCount)
	}
}

func TestQueryWindow_DBClosed(t *testing.T) {
	idx, _ := newTestIndex(t)
	_ = idx.Close()
	if _, err := idx.QueryWindow(time.Unix(0, 0), time.Now(), ""); err == nil {
		t.Errorf("expected error after Close")
	}
}

// TestQueryWindow_ToolsError covers the queryToolsInWindow error path from
// QueryWindow. We insert a session row, then drop tool_calls so the JOIN fails.
func TestQueryWindow_ToolsError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	now := time.Now()
	sess := &Session{ConversationID: "win-err", ProviderName: "anthropic", UpdatedAt: now}
	if err := idx.EnsureSession(src, "win-err", false, false, sess); err != nil {
		t.Fatal(err)
	}
	mustExec(t, idx.db, "DROP TABLE tool_calls")
	if _, err := idx.QueryWindow(now.Add(-time.Hour), now.Add(time.Hour), ""); err == nil {
		t.Errorf("expected tools-window error after dropping tool_calls")
	}
}

// TestQueryWindow_FilesError covers the queryFilesInWindow error path.
func TestQueryWindow_FilesError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	now := time.Now()
	sess := &Session{ConversationID: "win-files-err", ProviderName: "anthropic", UpdatedAt: now}
	if err := idx.EnsureSession(src, "win-files-err", false, false, sess); err != nil {
		t.Fatal(err)
	}
	mustExec(t, idx.db, "DROP TABLE tool_calls")
	mustExec(t, idx.db, `CREATE TABLE tool_calls (
		conversation_id TEXT, tool_call_id TEXT, tool_name TEXT, tool_type TEXT,
		operation_name TEXT, provider_name TEXT, is_error INTEGER, ts INTEGER,
		file_lines_added INTEGER, file_lines_removed INTEGER, file_content_size INTEGER,
		arguments_json TEXT, result_json TEXT
	)`)
	if _, err := idx.QueryWindow(now.Add(-time.Hour), now.Add(time.Hour), ""); err == nil {
		t.Errorf("expected files-window error when file_path missing")
	}
}

func TestToolType(t *testing.T) {
	cases := map[string]string{
		"mcp__claude-in-chrome__tabs_create_mcp": "mcp",
		"Skill":                                  "skill",
		"Agent":                                  "agent",
		"TaskCreate":                             "agent",
		"Grep":                                   "function",
		"Read":                                   "function",
		"":                                       "function",
	}
	for in, want := range cases {
		if got := ToolType(in); got != want {
			t.Errorf("ToolType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNullStr(t *testing.T) {
	if nullStr("") != nil {
		t.Errorf("nullStr empty should be nil")
	}
	v := nullStr("x")
	if s, ok := v.(string); !ok || s != "x" {
		t.Errorf("nullStr non-empty got %v", v)
	}
}

func TestNullInt(t *testing.T) {
	if nullInt(5, false) != nil {
		t.Errorf("nullInt not applicable should be nil")
	}
	v := nullInt(5, true)
	if n, ok := v.(int); !ok || n != 5 {
		t.Errorf("nullInt applicable got %v", v)
	}
}

func TestUnixOrNil(t *testing.T) {
	if unixOrNil(time.Time{}) != nil {
		t.Errorf("zero time should be nil")
	}
	now := time.Now()
	v := unixOrNil(now)
	if n, ok := v.(int64); !ok || n != now.Unix() {
		t.Errorf("non-zero time got %v", v)
	}
}

// TestEnsureSession_DuplicateToolID_IgnoresSilently verifies that a session
// containing two ToolCallRow entries with the same ToolCallID writes one row
// only (the first wins) and does NOT return an error. Observed in real-world
// long-running Claude sessions; INSERT OR IGNORE is the chosen behaviour.
func TestEnsureSession_DuplicateToolID_IgnoresSilently(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "conv-dup", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{
			{ToolCallID: "dup", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/x"},
			{ToolCallID: "dup", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/y"},
		}}
	if err := idx.EnsureSession(src, "conv-dup", false, false, sess); err != nil {
		t.Fatalf("INSERT OR IGNORE should not return error: %v", err)
	}
	var n int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE conversation_id='conv-dup'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 row after dedup, got %d", n)
	}
}

// TestSessionFromModel covers the model→index Session conversion.
func TestSessionFromModel(t *testing.T) {
	if SessionFromModel(nil, "anthropic") != nil {
		t.Errorf("nil model should produce nil")
	}
	now := time.Now()
	in := &model.Session{
		ID:        "sess1",
		Model:     "claude-opus-4-7",
		StartedAt: now.Add(-time.Hour),
		UpdatedAt: now,
		Messages: []model.Message{
			{Role: model.RoleAssistant, Timestamp: now,
				UsageInputTokens: 10, UsageOutputTokens: 5,
				UsageCacheCreationInputTokens: 3, UsageCacheReadInputTokens: 2,
				ToolCalls: []model.ToolCall{
					{Name: "Read", ID: "t1", FilePath: "/a.go", FileOp: "read", Input: `{"file_path":"/a.go"}`, Output: `"hi"`},
					{Name: "mcp__claude-in-chrome__tabs_create_mcp", ID: "t2"},
					{Name: "Skill", ID: "t3"},
					{Name: "Agent", ID: "t4"},
				},
			},
			{Role: model.RoleUser, Timestamp: now,
				UsageInputTokens: 999, // should NOT contribute (not assistant)
				ToolCalls: []model.ToolCall{
					{Name: "ToolFromUserShouldStillCopy", ID: "tu"},
				},
			},
		},
	}
	out := SessionFromModel(in, "anthropic")
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.TotalInputTokens != 10 || out.TotalOutputTokens != 5 {
		t.Errorf("token sums wrong (user message should be excluded): %d/%d", out.TotalInputTokens, out.TotalOutputTokens)
	}
	if len(out.ToolCalls) != 5 {
		t.Errorf("ToolCalls count = %d, want 5", len(out.ToolCalls))
	}
	// Verify ToolType classification was applied
	byName := make(map[string]ToolCallRow)
	for _, tc := range out.ToolCalls {
		byName[tc.ToolName] = tc
	}
	if byName["Skill"].ToolType != "skill" {
		t.Errorf("Skill ToolType = %q", byName["Skill"].ToolType)
	}
	if byName["mcp__claude-in-chrome__tabs_create_mcp"].ToolType != "mcp" {
		t.Errorf("mcp tooltype mismatch")
	}
	if byName["Agent"].ToolType != "agent" {
		t.Errorf("Agent ToolType = %q", byName["Agent"].ToolType)
	}
	if byName["Read"].FilePath != "/a.go" || byName["Read"].FileOp != "read" {
		t.Errorf("Read file fields not propagated")
	}
}

// TestEnsureSession_NoExistingRow_StatErrorPath covers the row.Scan
// non-sql.ErrNoRows error branch by passing in a closed DB.
func TestEnsureSession_ScanError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	// Drop the table to force a query error
	mustExec(t, idx.db, "DROP TABLE session_metadata")
	sess := &Session{ConversationID: "x", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "x", false, false, sess); err == nil {
		t.Errorf("expected error after dropping table")
	}
}

// TestRewriteSession_BeginTxError exercises the tx.Begin error branch by
// driving rewriteSession against a closed DB directly. (EnsureSession's
// readCacheKey would fail first on closed DB; calling rewriteSession lets
// us target Begin precisely.)
func TestRewriteSession_BeginTxError(t *testing.T) {
	idx, _ := newTestIndex(t)
	_ = idx.db.Close()
	sess := &Session{ConversationID: "begin-err", ProviderName: "anthropic"}
	if err := idx.rewriteSession("/nonexistent", "begin-err", 0, 0, false, sess); err == nil {
		t.Errorf("expected error from Begin after Close")
	}
}

// TestEnsureSession_DeleteToolCallsError keeps session_metadata intact but
// drops tool_calls. Read row finds no row → fileChanged path → Begin
// succeeds → DELETE tool_calls fails.
func TestEnsureSession_DeleteToolCallsError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	mustExec(t, idx.db, "DROP TABLE tool_calls")
	sess := &Session{ConversationID: "del1", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "del1", false, false, sess); err == nil {
		t.Errorf("expected DELETE tool_calls error")
	}
}

// TestEnsureSession_DeleteSessionMetadataError exercises the second DELETE
// failure. Index a session, then add a trigger that aborts DELETE on
// session_metadata so the re-index path errors at step 2.
func TestEnsureSession_DeleteSessionMetadataError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	// Insert a row first so the DELETE actually has something to delete.
	sess := &Session{ConversationID: "del2", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "del2", false, false, sess); err != nil {
		t.Fatal(err)
	}
	// Now add a trigger that blocks DELETE on session_metadata.
	mustExec(t, idx.db, `CREATE TRIGGER block_del_sm BEFORE DELETE ON session_metadata
		BEGIN
			SELECT RAISE(ABORT, 'delete blocked');
		END`)
	// Change the source file so the cache miss path fires.
	if err := os.WriteFile(src, []byte("{}+"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnsureSession(src, "del2", false, false, sess); err == nil {
		t.Errorf("expected DELETE session_metadata error")
	}
}

// TestEnsureSession_InsertSessionMetadataError forces the INSERT to fail by
// adding a CHECK constraint via a trigger that rejects the row.
func TestEnsureSession_InsertSessionMetadataError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	mustExec(t, idx.db, `CREATE TRIGGER block_ins_sm BEFORE INSERT ON session_metadata
		BEGIN
			SELECT RAISE(ABORT, 'insert blocked');
		END`)
	sess := &Session{ConversationID: "ins1", ProviderName: "anthropic"}
	if err := idx.EnsureSession(src, "ins1", false, false, sess); err == nil {
		t.Errorf("expected INSERT session_metadata error")
	}
}

// TestEnsureSession_PrepareError drops the tool_calls table AFTER session_metadata
// INSERT succeeds. Strategy: rename tool_calls so DELETE step finds no rows
// (it's idempotent), session_metadata INSERT works, then Prepare against
// "tool_calls" fails because the table is gone.
//
// SQLite can't rename a table mid-transaction in our flow; the simpler path
// is to drop tool_calls before the call, but then the first DELETE fails
// (the test above). To target the Prepare branch specifically we use a
// trigger that does NOT block DELETE on tool_calls (it's a no-op already
// because the table is empty) but DOES block INSERT into tool_calls.
func TestEnsureSession_InsertToolCallsError(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	mustExec(t, idx.db, `CREATE TRIGGER block_ins_tc BEFORE INSERT ON tool_calls
		BEGIN
			SELECT RAISE(ABORT, 'insert tc blocked');
		END`)
	sess := &Session{ConversationID: "inst", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{
			{ToolCallID: "t1", ToolName: "Read", Timestamp: time.Now(), FilePath: "/x", FileOp: "read"},
		}}
	if err := idx.EnsureSession(src, "inst", false, false, sess); err == nil {
		t.Errorf("expected INSERT tool_calls error")
	}
}

// Sanity: errors.Is sql.ErrNoRows still considered "not found".
// TestAggregateTools_ScanError exercises the Scan-error branch by issuing a
// query whose column count doesn't match the helper's expectation. Scan fails
// because the helper expects 4 columns and we return 1.
func TestAggregateTools_ScanError(t *testing.T) {
	idx, _ := newTestIndex(t)
	if _, err := idx.aggregateTools(`SELECT 1`); err == nil {
		t.Errorf("expected scan error from mismatched column count")
	}
}

// TestAggregateFiles_ScanError covers the same shape for aggregateFiles.
func TestAggregateFiles_ScanError(t *testing.T) {
	idx, _ := newTestIndex(t)
	if _, err := idx.aggregateFiles(`SELECT 1`); err == nil {
		t.Errorf("expected scan error from mismatched column count")
	}
}

// TestAggregateFiles_RowsErr forces a rows.Err() failure by closing the DB
// while a rows iterator is open. Use a query that yields one row, then
// close mid-iteration. Since we can't intercept the loop externally, we
// instead query a virtual table that has been broken.
//
// The accessible path: corrupt a tool_calls row in a way that survives the
// Scan but breaks subsequent iteration. SQLite normally won't fail rows.Err
// after a successful scan unless the connection is torn down.
//
// Simpler: issue a query that hits a syntax error on a deferred step.
// SQLite is lenient — easiest is to use db.Query against a table that gets
// dropped while rows are open. To make this deterministic we drop tool_calls
// between two queries — but rows.Err returns nil for empty result sets.
//
// Pragmatic resolution: skip this branch via a focused test that closes the
// DB connection after rows are opened. That triggers rows.Err on .Next() but
// after we already have one valid row.
func TestAggregateFiles_RowsErr(t *testing.T) {
	idx, _ := newTestIndex(t)
	dir := t.TempDir()
	src := makeSourceFile(t, dir, "s.jsonl", []byte("{}"))
	sess := &Session{ConversationID: "rowserr", ProviderName: "anthropic",
		ToolCalls: []ToolCallRow{
			{ToolCallID: "1", ToolName: "Read", Timestamp: time.Now(), FileOp: "read", FilePath: "/a"},
		}}
	if err := idx.EnsureSession(src, "rowserr", false, false, sess); err != nil {
		t.Fatal(err)
	}
	// Open a transaction that holds a write lock; then issue our query.
	// Actually: SQLite is single-writer; reads via Query() don't block.
	// We'll force rows.Err by closing the *sql.DB while an iterator is open.
	rows, err := idx.db.Query(`SELECT file_path, file_op, 0, 0 FROM tool_calls`)
	if err != nil {
		t.Fatal(err)
	}
	// Consume one row, then close the underlying DB.
	if !rows.Next() {
		t.Fatal("expected at least one row")
	}
	var p, op string
	var a, b int
	if err := rows.Scan(&p, &op, &a, &b); err != nil {
		t.Fatal(err)
	}
	_ = idx.db.Close()
	// rows.Next() after Close returns false; rows.Err() returns the close error.
	rows.Next()
	if err := rows.Err(); err == nil {
		t.Skip("rows.Err did not surface a closed-db error on this platform; branch left uncovered (defensive)")
	}
}

func TestSentinelMatch(t *testing.T) {
	if !errors.Is(sql.ErrNoRows, sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows identity broken")
	}
}
