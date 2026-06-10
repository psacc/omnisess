package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/procsnap"
)

func TestRunPS_Unsupported(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, procsnap.ErrUnsupported
	}, false)
	if err != nil {
		t.Fatalf("unsupported must not error, got %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("unsupported")) {
		t.Errorf("expected friendly notice, got %q", buf.String())
	}
}

// TestRunPS_EnumError ensures non-ErrUnsupported errors propagate.
func TestRunPS_EnumError(t *testing.T) {
	var buf bytes.Buffer
	sentinel := errors.New("boom")
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, sentinel
	}, false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// TestRunPS_EmptySessions exercises the "no live sessions" text branch.
func TestRunPS_EmptySessions(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, nil
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("No live sessions.")) {
		t.Errorf("expected empty-sessions notice, got %q", buf.String())
	}
}

// TestRunPS_TreeHappyPath exercises the non-JSON branch with sessions present.
func TestRunPS_TreeHappyPath(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{
			Sessions: []procsnap.Session{{PID: 1234, SessionID: "abcdef1234"}},
		}, nil
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// renderTree is real now — it should print something containing the session's short id.
	if !bytes.Contains(buf.Bytes(), []byte("abcdef12")) {
		t.Errorf("expected session short id in output, got %q", buf.String())
	}
}

// TestRunPS_JSONEmpty exercises the JSON branch with an empty snapshot.
func TestRunPS_JSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, nil
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded procsnap.Snapshot
	if jerr := json.Unmarshal(buf.Bytes(), &decoded); jerr != nil {
		t.Fatalf("expected valid JSON, got %q (err %v)", buf.String(), jerr)
	}
}

// TestRunPS_JSONWithSessions exercises the JSON branch with a non-empty snapshot.
func TestRunPS_JSONWithSessions(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{
			Sessions: []procsnap.Session{{PID: 42, SessionID: "xyz"}},
		}, nil
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded procsnap.Snapshot
	if jerr := json.Unmarshal(buf.Bytes(), &decoded); jerr != nil {
		t.Fatalf("expected valid JSON, got %q (err %v)", buf.String(), jerr)
	}
	if len(decoded.Sessions) != 1 || decoded.Sessions[0].PID != 42 {
		t.Errorf("roundtripped snapshot mismatch: %+v", decoded)
	}
}

// TestPSCmd_RunE invokes the psCmd RunE closure via rootCmd so that the
// closure itself (and not just runPSWith) is covered. On darwin Enumerate
// may succeed with zero or more sessions; on other platforms it returns
// ErrUnsupported. Both paths are handled without error.
func TestPSCmd_RunE(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	rootCmd.SetArgs([]string{"ps"})
	defer rootCmd.SetArgs(nil)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd ps returned error: %v", err)
	}
}

func TestRenderTree_Merges_SharedAncestors(t *testing.T) {
	snap := procsnap.Snapshot{
		Built: time.Now(),
		Sessions: []procsnap.Session{
			{
				PID:        52333,
				SessionID:  "aaa11111-0000",
				Name:       "refactor auth",
				CWD:        "/Users/me/prj/foo",
				StartedAt:  time.Now().Add(-2 * time.Minute),
				Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 5674, Command: "zsh"},
					{PID: 3012, Command: "iTerm2"},
					{PID: 1, Command: "launchd"},
				},
			},
			{
				PID:        60001,
				SessionID:  "bbb22222-0000",
				CWD:        "/Users/me/prj/bar",
				StartedAt:  time.Now().Add(-5 * time.Minute),
				Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 5680, Command: "zsh"},
					{PID: 3012, Command: "iTerm2"},
					{PID: 1, Command: "launchd"},
				},
			},
		},
	}
	var buf bytes.Buffer
	renderTree(&buf, snap)
	got := buf.String()

	// The shared iTerm2 and launchd ancestors must appear once each.
	if bytes.Count(buf.Bytes(), []byte("iTerm2")) != 1 {
		t.Errorf("iTerm2 must appear once (merged):\n%s", got)
	}
	if bytes.Count(buf.Bytes(), []byte("launchd")) != 1 {
		t.Errorf("launchd must appear once (merged):\n%s", got)
	}
	// Both claude leaves must be present.
	if !bytes.Contains(buf.Bytes(), []byte("refactor auth")) {
		t.Errorf("named session missing:\n%s", got)
	}
	if !bytes.Contains(buf.Bytes(), []byte("bbb22222")) {
		t.Errorf("unnamed session short id missing:\n%s", got)
	}
	// Entrypoint annotation.
	if !bytes.Contains(buf.Bytes(), []byte("cli")) {
		t.Errorf("entrypoint missing:\n%s", got)
	}
}

// TestRenderTree_MultipleTopLevelRoots covers the sort of root.children (the
// comparator is only invoked when there are at least two top-level roots).
// Two sessions with disjoint ancestor chains produce two distinct roots.
func TestRenderTree_MultipleTopLevelRoots(t *testing.T) {
	snap := procsnap.Snapshot{
		Built: time.Now(),
		Sessions: []procsnap.Session{
			// Root 1: top ancestor "zzz"
			{
				PID: 200, SessionID: "aaaaaaaa", Name: "sess-a",
				CWD: "/x/a", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 2, Command: "zzz"},
				},
			},
			// Root 2: top ancestor "aaa". Alphabetically before zzz, so sort
			// must reorder the top-level roots.
			{
				PID: 201, SessionID: "bbbbbbbb", Name: "sess-b",
				CWD: "/x/b", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 1, Command: "aaa"},
				},
			},
		},
	}
	var buf bytes.Buffer
	renderTree(&buf, snap)
	out := buf.String()

	// Two independent roots, "aaa" first after sorting.
	aaaIdx := strings.Index(out, "aaa (1)")
	zzzIdx := strings.Index(out, "zzz (2)")
	if aaaIdx < 0 || zzzIdx < 0 {
		t.Fatalf("expected both roots in output:\n%s", out)
	}
	if aaaIdx >= zzzIdx {
		t.Errorf("expected 'aaa (1)' sorted before 'zzz (2)':\n%s", out)
	}
}

func TestRenderTree_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderTree(&buf, procsnap.Snapshot{})
	if buf.Len() != 0 {
		t.Errorf("empty snapshot must produce no output, got %q", buf.String())
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 30 * time.Second, "30s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatAge(tc.d); got != tc.want {
				t.Errorf("formatAge(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestProjectBase(t *testing.T) {
	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{"empty", "", "-"},
		{"regular", "/Users/me/prj/foo", "foo"},
		{"trailing-slash", "/Users/me/prj/foo/", "/Users/me/prj/foo/"},
		{"no-slash", "foo", "foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectBase(tc.cwd); got != tc.want {
				t.Errorf("projectBase(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}

func TestShortID(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want string
	}{
		{"long", "abcdef1234567890", "abcdef12"},
		{"exact-8", "abcdef12", "abcdef12"},
		{"short", "abc", "abc"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortID(tc.id); got != tc.want {
				t.Errorf("shortID(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

func TestLeafLabel(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		session procsnap.Session
		want    []string // substrings that must appear
	}{
		{
			name: "named-cli",
			session: procsnap.Session{
				Tool:       procsnap.ToolClaude,
				SessionID:  "abcdef1234",
				Name:       "refactor auth",
				CWD:        "/Users/me/prj/foo",
				StartedAt:  now.Add(-30 * time.Second),
				Entrypoint: "cli",
			},
			want: []string{"claude", "refactor auth", "foo", "abcdef12", "cli", "30s"},
		},
		{
			name: "unnamed-desktop",
			session: procsnap.Session{
				Tool:       procsnap.ToolClaude,
				SessionID:  "xyz99999000",
				CWD:        "/Users/me/prj/bar",
				StartedAt:  now.Add(-2 * time.Hour),
				Entrypoint: "claude-desktop",
			},
			want: []string{"claude", "xyz99999", "bar", "desktop", "2h"},
		},
		{
			name: "codex-tui",
			session: procsnap.Session{
				Tool:       procsnap.ToolCodex,
				SessionID:  "019e0000-aaaa-7000-8000-000000000000",
				CWD:        "/Users/me/prj/baz",
				StartedAt:  now.Add(-3 * time.Minute),
				Entrypoint: "codex-tui",
			},
			want: []string{"codex", "019e0000", "baz", "codex-tui", "3m"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := leafLabel(tc.session, now)
			for _, s := range tc.want {
				if !strings.Contains(got, s) {
					t.Errorf("leafLabel: expected %q to contain %q", got, s)
				}
			}
		})
	}
}

func TestRunPS_JSON(t *testing.T) {
	var buf bytes.Buffer
	enum := func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{
			Built: time.Now(),
			Sessions: []procsnap.Session{{
				PID:       1234,
				SessionID: "abc",
				CWD:       "/tmp/x",
			}},
		}, nil
	}
	if err := runPSWith(&buf, enum, true); err != nil {
		t.Fatalf("runPSWith: %v", err)
	}
	var out procsnap.Snapshot
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, buf.String())
	}
	if len(out.Sessions) != 1 || out.Sessions[0].SessionID != "abc" {
		t.Errorf("roundtrip failed: %+v", out)
	}
}

// TestRenderTree_LastChildOfRootKeepsConnector locks down the invariant that
// the last child of a top-level root must render with "└─" and be indented
// under the root, NOT stranded as if it were another top-level root.
//
// Bug this protects against: printNode had a special case "prefix=="" &&
// isLast" that dropped the connector AND passed an empty prefix to children,
// which cascaded — every last-child of every root-level node would re-enter
// the "I am a top-level" branch and get printed without indentation.
func TestRenderTree_LastChildOfRootKeepsConnector(t *testing.T) {
	snap := procsnap.Snapshot{
		Built: time.Now(),
		Sessions: []procsnap.Session{
			// One claude under iTerm2 → launchd.
			{
				Tool: procsnap.ToolClaude,
				PID:  100, SessionID: "aaaaaaaa", Name: "sess-a",
				CWD: "/x/a", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 50, Command: "iTerm2"},
					{PID: 1, Command: "launchd"},
				},
			},
			// One claude under tmux → launchd. tmux sorts AFTER iTerm2,
			// so it is the LAST child of launchd — the bug trigger.
			{
				Tool: procsnap.ToolClaude,
				PID:  101, SessionID: "bbbbbbbb", Name: "sess-b",
				CWD: "/x/b", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 60, Command: "tmux"},
					{PID: 1, Command: "launchd"},
				},
			},
		},
	}
	var buf bytes.Buffer
	renderTree(&buf, snap)
	out := buf.String()

	// Middle child of launchd: ├─ iTerm2 (50)
	if !strings.Contains(out, "├─ iTerm2 (50)") {
		t.Errorf("expected '├─ iTerm2 (50)' (middle-child of launchd):\n%s", out)
	}
	// Last child of launchd: MUST be └─ tmux (60), not a bare "tmux (60)".
	if !strings.Contains(out, "└─ tmux (60)") {
		t.Errorf("expected '└─ tmux (60)' (last-child of launchd) with connector:\n%s", out)
	}
	// And the leaf claude under tmux must be indented under tmux, not stranded.
	// With correct rendering, the sess-b line is prefixed with "   └─ " (3 spaces
	// under tmux's blank col, then connector).
	if !strings.Contains(out, "   └─ claude  sess-b") {
		t.Errorf("expected sess-b to be indented under tmux with '   └─ ':\n%s", out)
	}
}

// TestRenderTree_DeepBranches exercises the middle-child (├─) and last-child
// (└─) connector branches of printNode by arranging siblings beneath a shared
// non-top ancestor (so prefix != "" on recursion).
func TestRenderTree_DeepBranches(t *testing.T) {
	snap := procsnap.Snapshot{
		Built: time.Now(),
		Sessions: []procsnap.Session{
			// Three claude leaves share grandparent launchd -> iTerm2 -> (zsh-a, zsh-b, zsh-c).
			// Under iTerm2, the three zsh children produce ├─ / ├─ / └─.
			// Each zsh has a single claude child -> also └─ at deeper level.
			{
				PID: 101, SessionID: "aaa", Name: "sess-a",
				CWD: "/x/a", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 10, Command: "zsh-a"},
					{PID: 3012, Command: "iTerm2"},
					{PID: 1, Command: "launchd"},
				},
			},
			{
				PID: 102, SessionID: "bbb", Name: "sess-b",
				CWD: "/x/b", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 11, Command: "zsh-b"},
					{PID: 3012, Command: "iTerm2"},
					{PID: 1, Command: "launchd"},
				},
			},
			{
				PID: 103, SessionID: "ccc", Name: "sess-c",
				CWD: "/x/c", StartedAt: time.Now().Add(-1 * time.Minute), Entrypoint: "cli",
				Ancestors: []procsnap.Ancestor{
					{PID: 12, Command: "zsh-c"},
					{PID: 3012, Command: "iTerm2"},
					{PID: 1, Command: "launchd"},
				},
			},
		},
	}
	var buf bytes.Buffer
	renderTree(&buf, snap)
	out := buf.String()
	if !strings.Contains(out, "├─") {
		t.Errorf("expected middle-child connector ├─ in output:\n%s", out)
	}
	if !strings.Contains(out, "└─") {
		t.Errorf("expected last-child connector └─ in output:\n%s", out)
	}
}
