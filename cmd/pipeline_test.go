package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/psacc/omnisess/internal/index"
	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

// TestPipeline_RealFixture is the end-to-end smoke test for the transcript
// index: real Claude JSONL → claude.Source.Get → SessionFromModel →
// EnsureSession → QuerySession. It uses the parser-package testdata fixture
// (`tool_use_result.jsonl`) which already exercises Read/Write/Edit/Bash/
// Skill/MCP/Agent/TaskCreate/orphan combinations. Catches contract drift
// across the source→index boundary that unit tests at either layer would
// miss in isolation.
func TestPipeline_RealFixture(t *testing.T) {
	// Jailed HOME comes from TestMain; we still need to physically lay out
	// the fixture under ~/.claude/projects/<encoded>/<sessionID>.jsonl so
	// claudeSource.Get's glob can find it.
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME must be set (TestMain jails it)")
	}
	const sessID = "pipeline-fixture-0000-0000-0000-000000000001"
	projDir := filepath.Join(home, ".claude", "projects", "-tmp-pipeline-fixture")
	if err := os.MkdirAll(projDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Tests share the package-level jailed HOME (see TestMain); without cleanup
	// later tests (e.g. TestRunStats_WindowEmpty) would silently lazy-index
	// this fixture and inflate their session counts.
	t.Cleanup(func() { _ = os.RemoveAll(projDir) })
	// Locate the source-package fixture relative to this test file.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	srcFixture := filepath.Join(wd, "..", "internal", "source", "claude", "testdata", "tool_use_result.jsonl")
	data, err := os.ReadFile(srcFixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(projDir, sessID+".jsonl")
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Drive the pipeline: claude.Source.Get → SessionFromModel → EnsureSession.
	srcs := source.ByName(model.ToolClaude)
	if len(srcs) == 0 {
		t.Fatal("claude source not registered")
	}
	sess, err := srcs[0].Get(sessID)
	if err != nil {
		t.Fatalf("claude.Get: %v", err)
	}
	if sess == nil {
		t.Fatal("claude.Get returned nil session")
	}

	withTempIndex(t)
	idx, err := openDefaultIndex()
	if err != nil {
		t.Fatalf("openDefaultIndex: %v", err)
	}
	defer idx.Close()

	idxSess := index.SessionFromModel(sess, "anthropic")
	if idxSess == nil {
		t.Fatal("SessionFromModel returned nil")
	}
	// Use --full so we also exercise the payload path.
	if err := idx.EnsureSession(dst, sessID, true, false, idxSess); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	stats, err := idx.QuerySession(sessID)
	if err != nil {
		t.Fatalf("QuerySession: %v", err)
	}
	if stats == nil {
		t.Fatal("QuerySession returned nil")
	}

	// Fixture sanity (all numbers derived from tool_use_result.jsonl):
	//   - 11 distinct tool_use blocks (R1, E1, B1, R2, S1, B2, W1, MCP1, AG1, TC1, ORPHAN)
	//   - 3 errors (R2 via tool_result.is_error, S1 via success:false, B2 via "Error:")
	//   - Read tool: 2 calls, 1 error
	//   - Edit tool: 1 call, 0 errors
	//   - Bash tool: 2 calls, 1 error
	//   - Write tool: 1 call, 0 errors
	//   - Files touched: /Users/x/proj/a.go (R1+E1), /Users/x/proj/b.go (W1),
	//     /nonexistent.go (R2). /Users/x/proj/a.go gets 1 read + 1 edit;
	//     /Users/x/proj/b.go gets 1 write (file_content_size=11); /nonexistent.go
	//     gets 1 read.
	if stats.ConversationID != sessID {
		t.Errorf("ConversationID = %q, want %q", stats.ConversationID, sessID)
	}
	if stats.ProviderName != "anthropic" {
		t.Errorf("ProviderName = %q, want anthropic", stats.ProviderName)
	}
	if stats.ToolCallCount != 11 {
		t.Errorf("ToolCallCount = %d, want 11", stats.ToolCallCount)
	}
	if stats.ErrorCount != 3 {
		t.Errorf("ErrorCount = %d, want 3", stats.ErrorCount)
	}
	if !stats.HasFullPayloads {
		t.Error("HasFullPayloads = false, want true (indexed with --full)")
	}

	// Spot-check per-tool counts.
	byName := map[string]index.ToolCount{}
	for _, tc := range stats.Tools {
		byName[tc.Name] = tc
	}
	for _, want := range []struct {
		name       string
		count      int
		errorCount int
	}{
		{"Read", 2, 1},
		{"Edit", 1, 0},
		{"Bash", 2, 1},
		{"Write", 1, 0},
		{"Skill", 1, 1},
	} {
		got, ok := byName[want.name]
		if !ok {
			t.Errorf("tool %q missing from stats", want.name)
			continue
		}
		if got.Count != want.count {
			t.Errorf("tool %q count = %d, want %d", want.name, got.Count, want.count)
		}
		if got.ErrorCount != want.errorCount {
			t.Errorf("tool %q errorCount = %d, want %d", want.name, got.ErrorCount, want.errorCount)
		}
	}

	// Spot-check per-file activity (only Read/Write/Edit produce file rows).
	byFile := map[string]index.FileActivity{}
	for _, f := range stats.Files {
		byFile[f.Path] = f
	}
	if a := byFile["/Users/x/proj/a.go"]; a.Reads != 1 || a.Edits != 1 {
		t.Errorf("/Users/x/proj/a.go = %+v, want Reads=1 Edits=1", a)
	}
	if a := byFile["/Users/x/proj/a.go"]; a.LinesAdded != 2 || a.LinesRemoved != 1 {
		t.Errorf("/Users/x/proj/a.go lines = +%d/-%d, want +2/-1", a.LinesAdded, a.LinesRemoved)
	}
	if b := byFile["/Users/x/proj/b.go"]; b.Writes != 1 {
		t.Errorf("/Users/x/proj/b.go = %+v, want Writes=1", b)
	}
	if n := byFile["/nonexistent.go"]; n.Reads != 1 {
		t.Errorf("/nonexistent.go = %+v, want Reads=1", n)
	}

	// Token totals from the fixture's assistant messages:
	//   100+50+10 = 160 input, 40+20+5 = 65 output, 1000 cache_create, 500 cache_read.
	// Only assistant messages whose `usage` field is present contribute; some
	// later assistant lines in the fixture have no usage block.
	if stats.TotalInputTokens != 160 {
		t.Errorf("TotalInputTokens = %d, want 160", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 65 {
		t.Errorf("TotalOutputTokens = %d, want 65", stats.TotalOutputTokens)
	}
	if stats.TotalCacheCreateTokens != 1000 {
		t.Errorf("TotalCacheCreateTokens = %d, want 1000", stats.TotalCacheCreateTokens)
	}
	if stats.TotalCacheReadTokens != 500 {
		t.Errorf("TotalCacheReadTokens = %d, want 500", stats.TotalCacheReadTokens)
	}
}
