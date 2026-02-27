package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

// ---------------------------------------------------------------------------
// Fixture constants
// ---------------------------------------------------------------------------

const (
	fixtureSessionID   = "aabbccdd-1234-5678-9abc-def012345678"
	fixtureSessionID2  = "11223344-aaaa-bbbb-cccc-ddddeeeeffff"
	fixtureSessionFile = "testdata/session-aabbccdd-1234-5678-9abc-def012345678.jsonl"
)

// ---------------------------------------------------------------------------
// setupFakeHome builds a minimal ~/.codex layout in a temp dir and returns
// the home directory path and the session file path it created.
//
// Layout:
//
//	<home>/.codex/history.jsonl           (copy of testdata/history.jsonl)
//	<home>/.codex/sessions/2026/02/09/
//	    rollout-20260209T100111-<uuid>.jsonl  (copy of testdata/session-<uuid>.jsonl)
//
// ---------------------------------------------------------------------------
func setupFakeHome(t *testing.T) (homeDir, sessionPath string) {
	t.Helper()
	home := t.TempDir()

	// Create directory structure
	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "02", "09")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("create sessions dir: %v", err)
	}

	// Copy history.jsonl
	histData, err := os.ReadFile("testdata/history.jsonl")
	if err != nil {
		t.Fatalf("read testdata/history.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "history.jsonl"), histData, 0o644); err != nil {
		t.Fatalf("write history.jsonl: %v", err)
	}

	// Copy session file into the sessions tree with the real naming pattern
	sessData, err := os.ReadFile(fixtureSessionFile)
	if err != nil {
		t.Fatalf("read %s: %v", fixtureSessionFile, err)
	}
	sessionPath = filepath.Join(sessionsDir, "rollout-20260209T100111-"+fixtureSessionID+".jsonl")
	if err := os.WriteFile(sessionPath, sessData, 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}

	return home, sessionPath
}

// ---------------------------------------------------------------------------
// 3.3  parseHistoryLine — table-driven
// ---------------------------------------------------------------------------

func TestParseHistoryLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantID   string
		wantTs   int64
		wantText string
		wantErr  bool
	}{
		{
			name:     "valid entry",
			line:     `{"session_id":"aabbccdd-1234-5678-9abc-def012345678","ts":1739091671,"text":"compare AGENTS.md with CLAUDE.md"}`,
			wantID:   "aabbccdd-1234-5678-9abc-def012345678",
			wantTs:   1739091671,
			wantText: "compare AGENTS.md with CLAUDE.md",
		},
		{
			name:    "malformed JSON",
			line:    `{invalid`,
			wantErr: true,
		},
		{
			name:     "missing optional fields",
			line:     `{"session_id":"abc123"}`,
			wantID:   "abc123",
			wantTs:   0,
			wantText: "",
		},
		{
			name:     "empty session_id is valid parse",
			line:     `{"session_id":"","ts":1000,"text":"hi"}`,
			wantID:   "",
			wantTs:   1000,
			wantText: "hi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseHistoryLine([]byte(tt.line))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.SessionID != tt.wantID {
				t.Errorf("SessionID = %q, want %q", entry.SessionID, tt.wantID)
			}
			if entry.Ts != tt.wantTs {
				t.Errorf("Ts = %d, want %d", entry.Ts, tt.wantTs)
			}
			if entry.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", entry.Text, tt.wantText)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3.4  parseSessionFile — table-driven
// ---------------------------------------------------------------------------

func TestParseSessionFile(t *testing.T) {
	t.Run("messages in order with role mapping and cwd", func(t *testing.T) {
		msgs, cwd, err := parseSessionFile(fixtureSessionFile)
		if err != nil {
			t.Fatalf("parseSessionFile: %v", err)
		}

		if cwd != "/Users/testuser/prj/myproject" {
			t.Errorf("cwd = %q, want /Users/testuser/prj/myproject", cwd)
		}

		if len(msgs) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(msgs))
		}

		// developer → RoleUser
		if msgs[0].Role != model.RoleUser {
			t.Errorf("msgs[0].Role = %q, want %q", msgs[0].Role, model.RoleUser)
		}
		if msgs[0].Content != "compare AGENTS.md with CLAUDE.md" {
			t.Errorf("msgs[0].Content = %q", msgs[0].Content)
		}

		// assistant → RoleAssistant
		if msgs[1].Role != model.RoleAssistant {
			t.Errorf("msgs[1].Role = %q, want %q", msgs[1].Role, model.RoleAssistant)
		}
		if msgs[1].Content != "Both files define agent behavior. AGENTS.md is more concise." {
			t.Errorf("msgs[1].Content = %q", msgs[1].Content)
		}

		if msgs[2].Role != model.RoleUser {
			t.Errorf("msgs[2].Role = %q, want %q", msgs[2].Role, model.RoleUser)
		}
		if msgs[3].Role != model.RoleAssistant {
			t.Errorf("msgs[3].Role = %q, want %q", msgs[3].Role, model.RoleAssistant)
		}
	})

	t.Run("timestamps are set", func(t *testing.T) {
		msgs, _, err := parseSessionFile(fixtureSessionFile)
		if err != nil {
			t.Fatalf("parseSessionFile: %v", err)
		}
		for i, m := range msgs {
			if m.Timestamp.IsZero() {
				t.Errorf("msgs[%d].Timestamp is zero", i)
			}
		}
	})

	t.Run("malformed lines are skipped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.jsonl")
		content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"id":"x","cwd":"/tmp"}}` + "\n" +
			`{bad json` + "\n" +
			`{"timestamp":"2026-02-09T10:01:12.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		msgs, cwd, err := parseSessionFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cwd != "/tmp" {
			t.Errorf("cwd = %q, want /tmp", cwd)
		}
		if len(msgs) != 1 {
			t.Errorf("expected 1 message (malformed line skipped), got %d", len(msgs))
		}
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		_, _, err := parseSessionFile("testdata/nonexistent.jsonl")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("event_msg lines are skipped", func(t *testing.T) {
		// event_msg and response_item carry the same conversation content.
		// Only response_item is parsed to avoid duplicates.
		dir := t.TempDir()
		path := filepath.Join(dir, "event_msg_test.jsonl")
		content := `{"timestamp":"2026-02-09T10:01:11.966Z","type":"session_meta","payload":{"id":"x","cwd":"/tmp"}}` + "\n" +
			`{"timestamp":"2026-02-09T10:01:12.000Z","type":"event_msg","payload":{"type":"user_message","message":"hello from event"}}` + "\n" +
			`{"timestamp":"2026-02-09T10:01:13.000Z","type":"event_msg","payload":{"type":"agent_message","message":"response from agent"}}` + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		msgs, _, err := parseSessionFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 0 {
			t.Fatalf("expected 0 messages (event_msg skipped), got %d", len(msgs))
		}
	})
}

// ---------------------------------------------------------------------------
// 3.5  findSessionFile — table-driven (uses temp dir)
// ---------------------------------------------------------------------------

func TestFindSessionFile(t *testing.T) {
	home, sessionPath := setupFakeHome(t)
	_ = sessionPath

	t.Run("exact match", func(t *testing.T) {
		got := findSessionFile(home, fixtureSessionID)
		if got == "" {
			t.Fatal("expected a path, got empty string")
		}
		if !strings.Contains(got, fixtureSessionID) {
			t.Errorf("path %q does not contain session ID", got)
		}
	})

	t.Run("no match returns empty string", func(t *testing.T) {
		got := findSessionFile(home, "00000000-0000-0000-0000-000000000000")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// 3.6  List() — integration-style (deduplication, ordering, preview)
// ---------------------------------------------------------------------------

func TestList(t *testing.T) {
	home, _ := setupFakeHome(t)

	// Patch os.UserHomeDir by overriding via environment isn't practical here;
	// instead we exercise loadHistory directly against our fake home by writing
	// a thin wrapper test that calls the package-private function with our dir.
	// We test List() via the source interface with a patched home using
	// the unexported codexDir helper — not ideal, so we test loadHistory + findSessionFile
	// directly and verify the session source via integration test with env.

	// Use loadHistory with the fake home by temporarily overriding HOME.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer func() { os.Setenv("HOME", origHome) }() //nolint:errcheck

	s := &codexSource{}
	sessions, err := s.List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	// history.jsonl has 2 unique sessions: fixtureSessionID and fixtureSessionID2
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// First session should be fixtureSessionID (latest ts = 1739095271 > 1739080000)
	if sessions[0].ID != fixtureSessionID {
		t.Errorf("sessions[0].ID = %q, want %q", sessions[0].ID, fixtureSessionID)
	}
	if sessions[0].Tool != model.ToolCodex {
		t.Errorf("sessions[0].Tool = %q, want %q", sessions[0].Tool, model.ToolCodex)
	}
	// StartedAt should be from earliest ts (1739091671)
	wantStarted := time.Unix(1739091671, 0)
	if !sessions[0].StartedAt.Equal(wantStarted) {
		t.Errorf("sessions[0].StartedAt = %v, want %v", sessions[0].StartedAt, wantStarted)
	}
	// Preview from earliest text entry
	if sessions[0].Preview != "compare AGENTS.md with CLAUDE.md" {
		t.Errorf("sessions[0].Preview = %q", sessions[0].Preview)
	}

	t.Run("deduplication: only 2 sessions for 3 history lines", func(t *testing.T) {
		if len(sessions) != 2 {
			t.Errorf("expected 2 sessions (deduplication), got %d", len(sessions))
		}
	})

	t.Run("Limit filter", func(t *testing.T) {
		limited, err := s.List(source.ListOptions{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(limited) != 1 {
			t.Errorf("expected 1 session with Limit=1, got %d", len(limited))
		}
	})

	t.Run("Since filter excludes old sessions", func(t *testing.T) {
		// Since = 1 nanosecond: all sessions should be excluded (they're old)
		filtered, err := s.List(source.ListOptions{Since: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(filtered) != 0 {
			t.Errorf("expected 0 sessions with very short Since, got %d", len(filtered))
		}
	})
}

// ---------------------------------------------------------------------------
// 3.7  Get() — messages populated, project set
// ---------------------------------------------------------------------------

func TestGet(t *testing.T) {
	home, _ := setupFakeHome(t)

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer func() { os.Setenv("HOME", origHome) }() //nolint:errcheck

	s := &codexSource{}

	t.Run("valid session ID returns session with messages", func(t *testing.T) {
		sess, err := s.Get(fixtureSessionID)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}
		if sess == nil {
			t.Fatal("expected session, got nil")
		}

		if sess.ID != fixtureSessionID {
			t.Errorf("sess.ID = %q, want %q", sess.ID, fixtureSessionID)
		}
		if sess.Tool != model.ToolCodex {
			t.Errorf("sess.Tool = %q, want %q", sess.Tool, model.ToolCodex)
		}
		if sess.Project != "/Users/testuser/prj/myproject" {
			t.Errorf("sess.Project = %q", sess.Project)
		}
		if len(sess.Messages) != 4 {
			t.Errorf("expected 4 messages, got %d", len(sess.Messages))
		}
	})

	t.Run("prefix match returns session", func(t *testing.T) {
		// Use first 8 chars as prefix
		prefix := fixtureSessionID[:8]
		sess, err := s.Get(prefix)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", prefix, err)
		}
		if sess == nil {
			t.Fatal("expected session, got nil")
		}
		if sess.ID != fixtureSessionID {
			t.Errorf("sess.ID = %q, want %q", sess.ID, fixtureSessionID)
		}
	})

	t.Run("unknown ID returns nil nil", func(t *testing.T) {
		sess, err := s.Get("00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if sess != nil {
			t.Errorf("expected nil session, got %+v", sess)
		}
	})
}

// ---------------------------------------------------------------------------
// 3.8  Search() — hit and miss
// ---------------------------------------------------------------------------

func TestSearch(t *testing.T) {
	home, _ := setupFakeHome(t)

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer func() { os.Setenv("HOME", origHome) }() //nolint:errcheck

	s := &codexSource{}

	t.Run("query matches content", func(t *testing.T) {
		results, err := s.Search("agents.md", source.ListOptions{})
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result, got 0")
		}
		// Should have a snippet
		if len(results[0].Matches) == 0 {
			t.Fatal("expected matches, got 0")
		}
		snippet := results[0].Matches[0].Snippet
		if !strings.Contains(strings.ToLower(snippet), "agents.md") {
			t.Errorf("snippet %q does not contain query", snippet)
		}
	})

	t.Run("query matches nothing returns empty", func(t *testing.T) {
		results, err := s.Search("zzznomatchzzz", source.ListOptions{})
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		results, err := s.Search("COMPARE", source.ListOptions{})
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected match for uppercase query, got 0")
		}
	})
}

// ---------------------------------------------------------------------------
// extractSessionIDFromPath
// ---------------------------------------------------------------------------

func TestExtractSessionIDFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "standard codex path",
			path: "/Users/foo/.codex/sessions/2026/02/09/rollout-20260209T100111-aabbccdd-1234-5678-9abc-def012345678.jsonl",
			want: "aabbccdd-1234-5678-9abc-def012345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionIDFromPath(tt.path)
			if got != tt.want {
				t.Errorf("extractSessionIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
