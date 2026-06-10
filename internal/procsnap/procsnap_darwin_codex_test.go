//go:build darwin

package procsnap

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const codexFixtureDay = "testdata/codex-sessions/2026/06/09"

func fixtureRollout(name string) string {
	return filepath.Join(codexFixtureDay, name)
}

// stubCodexDir points codexSessionsDirFn at the testdata fixture tree.
func stubCodexDir(t *testing.T) {
	t.Helper()
	orig := codexSessionsDirFn
	codexSessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "codex-sessions"), nil
	}
	t.Cleanup(func() { codexSessionsDirFn = orig })
}

func stubCodexLsof(t *testing.T, fn func(pids []int) ([]byte, error)) {
	t.Helper()
	orig := codexLsofFn
	codexLsofFn = fn
	t.Cleanup(func() { codexLsofFn = orig })
}

// codexProcs is a minimal proc table with two codex processes (a TUI and a
// Codex.app app-server), one unrelated process, and their shared ancestry.
func codexProcs() map[int]procInfo {
	return map[int]procInfo{
		1:    {PID: 1, PPID: 0, Command: "launchd", Args: "/sbin/launchd"},
		70:   {PID: 70, PPID: 1, Command: "zsh", Args: "-zsh"},
		7001: {PID: 7001, PPID: 70, Command: "codex", Args: "codex --yolo"},
		7002: {PID: 7002, PPID: 1, Command: "/Applications/Codex.app/Contents/Resources/codex", Args: "codex app-server"},
		7003: {PID: 7003, PPID: 70, Command: "codex", Args: "codex"},
		8000: {PID: 8000, PPID: 70, Command: "vim", Args: "vim notes.txt"},
	}
}

func TestCodexSessions_HappyPath(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func(pids []int) ([]byte, error) {
		if len(pids) != 3 || pids[0] != 7001 || pids[1] != 7002 || pids[2] != 7003 {
			t.Fatalf("unexpected candidate pids: %v", pids)
		}
		out := "p7001\n" +
			"fcwd\n" +
			"n/Users/me/prj/lsof-cwd\n" +
			"ftxt\n" +
			"n/opt/homebrew/bin/codex\n" +
			"f34\n" +
			"n" + fixtureRollout("rollout-2026-06-09T12-00-00-019eb000-0000-7000-8000-00000000a001.jsonl") + "\n" +
			"p7002\n" +
			"fcwd\n" +
			"n/Users/me/prj/server-cwd\n" +
			"f35\n" +
			"n" + fixtureRollout("rollout-2026-06-09T13-00-00-019eb000-0000-7000-8000-00000000b002.jsonl") + "\n" +
			"f36\n" +
			"n" + fixtureRollout("rollout-2026-06-09T14-00-00-019eb000-0000-7000-8000-00000000c003.jsonl") + "\n" +
			"f37\n" +
			"n" + filepath.Join(codexFixtureDay, "notes.jsonl") + "\n"
		return []byte(out), nil
	})

	got := codexSessions(codexProcs())
	if len(got) != 3 {
		t.Fatalf("expected 3 codex sessions, got %d: %+v", len(got), got)
	}

	// 7001: valid session_meta — meta values win over filename/lsof fallbacks.
	s := got[0]
	if s.Tool != ToolCodex || s.PID != 7001 {
		t.Errorf("session 0 identity: %+v", s)
	}
	if s.SessionID != "019eb000-0000-7000-8000-00000000a001" {
		t.Errorf("session 0 id = %q", s.SessionID)
	}
	if s.CWD != "/Users/me/prj/foo" {
		t.Errorf("session 0 cwd must come from session_meta, got %q", s.CWD)
	}
	if s.Version != "0.138.0" || s.Entrypoint != "codex-tui" {
		t.Errorf("session 0 version/entrypoint: %+v", s)
	}
	want := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	if !s.StartedAt.Equal(want) {
		t.Errorf("session 0 started = %v, want %v", s.StartedAt, want)
	}
	if len(s.Ancestors) != 2 || s.Ancestors[0].Command != "zsh" || s.Ancestors[1].Command != "launchd" {
		t.Errorf("session 0 ancestors: %+v", s.Ancestors)
	}

	// 7002 / rollout b002: malformed first line — filename + lsof fallbacks.
	s = got[1]
	if s.PID != 7002 || s.SessionID != "019eb000-0000-7000-8000-00000000b002" {
		t.Errorf("session 1 fallback id: %+v", s)
	}
	if s.CWD != "/Users/me/prj/server-cwd" {
		t.Errorf("session 1 cwd must fall back to lsof cwd, got %q", s.CWD)
	}
	wantLocal := time.Date(2026, 6, 9, 13, 0, 0, 0, time.Local)
	if !s.StartedAt.Equal(wantLocal) {
		t.Errorf("session 1 started = %v, want filename time %v", s.StartedAt, wantLocal)
	}
	if s.Version != "" || s.Entrypoint != "" {
		t.Errorf("session 1 must have no meta-derived fields: %+v", s)
	}

	// 7002 / rollout c003: valid meta without cwd/timestamp — partial fallbacks.
	s = got[2]
	if s.SessionID != "019eb000-0000-7000-8000-00000000c003" || s.Entrypoint != "codex-exec" {
		t.Errorf("session 2 meta fields: %+v", s)
	}
	if s.CWD != "/Users/me/prj/server-cwd" {
		t.Errorf("session 2 cwd must fall back to lsof cwd, got %q", s.CWD)
	}
	wantLocal = time.Date(2026, 6, 9, 14, 0, 0, 0, time.Local)
	if !s.StartedAt.Equal(wantLocal) {
		t.Errorf("session 2 started must keep filename time, got %v", s.StartedAt)
	}
}

func TestCodexSessions_NoCandidates(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) {
		t.Fatal("lsof must not run without codex candidates")
		return nil, nil
	})
	procs := map[int]procInfo{
		1:  {PID: 1, Command: "launchd"},
		70: {PID: 70, Command: "zsh"},
	}
	if got := codexSessions(procs); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCodexSessions_DirError(t *testing.T) {
	orig := codexSessionsDirFn
	codexSessionsDirFn = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { codexSessionsDirFn = orig })

	if got := codexSessions(codexProcs()); got != nil {
		t.Errorf("dir error must yield nil, got %+v", got)
	}
}

func TestCodexSessions_LsofErrorNoOutput(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) { return nil, errors.New("lsof: not found") })
	if got := codexSessions(codexProcs()); got != nil {
		t.Errorf("lsof failure must yield nil, got %+v", got)
	}
}

func TestCodexSessions_LsofEmptyOutputNoError(t *testing.T) {
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) { return nil, nil })
	if got := codexSessions(codexProcs()); got != nil {
		t.Errorf("empty lsof output must yield nil, got %+v", got)
	}
}

func TestCodexSessions_LsofErrorWithOutputIsUsed(t *testing.T) {
	// lsof can exit non-zero (e.g. a PID died mid-scan) while still emitting
	// usable records; the output must be parsed anyway.
	stubCodexDir(t)
	stubCodexLsof(t, func([]int) ([]byte, error) {
		out := "p7001\nfcwd\nn/Users/me/prj/x\nf34\nn" +
			fixtureRollout("rollout-2026-06-09T12-00-00-019eb000-0000-7000-8000-00000000a001.jsonl") + "\n"
		return []byte(out), errors.New("exit status 1")
	})
	got := codexSessions(codexProcs())
	if len(got) != 1 || got[0].SessionID != "019eb000-0000-7000-8000-00000000a001" {
		t.Fatalf("expected 1 session from partial lsof output, got %+v", got)
	}
}

func TestEnumerate_IncludesCodex(t *testing.T) {
	// End-to-end through Enumerate: one claude session (registry fixture)
	// plus one codex session (lsof fixture) in a single snapshot.
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "sessions"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	origKill := killFn
	killFn = func(pid int) error {
		if pid == 52333 {
			return nil
		}
		return errors.New("dead")
	}
	t.Cleanup(func() { killFn = origKill })

	origPS := psRunnerFn
	psRunnerFn = func() ([]byte, error) {
		return []byte(`    1     0 launchd          /sbin/launchd
 5674     1 zsh              -zsh
52333  5674 claude           /usr/local/bin/claude
 7001  5674 codex            codex --yolo
`), nil
	}
	t.Cleanup(func() { psRunnerFn = origPS })

	stubCodexDir(t)
	stubCodexLsof(t, func(pids []int) ([]byte, error) {
		out := "p7001\nfcwd\nn/Users/me/prj/x\nf34\nn" +
			fixtureRollout("rollout-2026-06-09T12-00-00-019eb000-0000-7000-8000-00000000a001.jsonl") + "\n"
		return []byte(out), nil
	})

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(snap.Sessions) != 2 {
		t.Fatalf("expected claude+codex sessions, got %d: %+v", len(snap.Sessions), snap.Sessions)
	}
	if snap.Sessions[0].Tool != ToolClaude || snap.Sessions[1].Tool != ToolCodex {
		t.Errorf("tool ordering: %+v", snap.Sessions)
	}
	if !snap.IsActive("019eb000-0000-7000-8000-00000000a001") {
		t.Error("IsActive must match the codex session ID")
	}
}

func TestParseCodexLsof(t *testing.T) {
	dir := filepath.Join("testdata", "codex-sessions")
	rollout := fixtureRollout("rollout-2026-06-09T12-00-00-019eb000-0000-7000-8000-00000000a001.jsonl")
	raw := strings.Join([]string{
		"",     // blank line skipped
		"pabc", // unparseable pid — resets current pid
		"fcwd", // fd record while pid invalid
		"n/skipped/because/pid/invalid",
		"p7001",
		"fcwd",
		"n/Users/me/prj/x",
		"ftxt",
		"n/opt/homebrew/bin/codex", // not under sessions dir
		"f34",
		"n" + rollout,
		"f35",
		"n" + filepath.Join(dir, "2026", "06", "09") + "/other.log", // wrong suffix
	}, "\n") + "\n"

	got := parseCodexLsof([]byte(raw), dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 pid, got %+v", got)
	}
	cp := got[7001]
	if cp.cwd != "/Users/me/prj/x" {
		t.Errorf("cwd = %q", cp.cwd)
	}
	if len(cp.rollouts) != 1 || cp.rollouts[0] != rollout {
		t.Errorf("rollouts = %+v", cp.rollouts)
	}
}

func TestParseRolloutFilename(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantID  string
		wantOK  bool
		zeroTS  bool
		localTS time.Time
	}{
		{
			name:    "valid",
			path:    "/x/rollout-2026-06-09T12-25-40-019eb000-0000-7000-8000-00000000f006.jsonl",
			wantID:  "019eb000-0000-7000-8000-00000000f006",
			wantOK:  true,
			localTS: time.Date(2026, 6, 9, 12, 25, 40, 0, time.Local),
		},
		{
			name:   "bad timestamp still yields id",
			path:   "/x/rollout-notatime-019eb000-0000-7000-8000-00000000f006.jsonl",
			wantID: "019eb000-0000-7000-8000-00000000f006",
			wantOK: true,
			zeroTS: true,
		},
		{name: "no prefix", path: "/x/notes.jsonl"},
		{name: "no suffix", path: "/x/rollout-2026-06-09T12-25-40-019eb000-0000-7000-8000-00000000f006.txt"},
		{name: "too short", path: "/x/rollout-abc.jsonl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ts, ok := parseRolloutFilename(tc.path)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
			if tc.wantOK {
				if tc.zeroTS != ts.IsZero() {
					t.Errorf("ts = %v, zero-ness want %v", ts, tc.zeroTS)
				}
				if !tc.zeroTS && !ts.Equal(tc.localTS) {
					t.Errorf("ts = %v, want %v", ts, tc.localTS)
				}
			}
		})
	}
}

func TestReadCodexSessionMeta_Errors(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"missing file", fixtureRollout("rollout-does-not-exist.jsonl")},
		{"empty file", fixtureRollout("rollout-2026-06-09T15-00-00-019eb000-0000-7000-8000-00000000d004.jsonl")},
		{"malformed json", fixtureRollout("rollout-2026-06-09T13-00-00-019eb000-0000-7000-8000-00000000b002.jsonl")},
		{"wrong type", fixtureRollout("rollout-2026-06-09T16-00-00-019eb000-0000-7000-8000-00000000e005.jsonl")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readCodexSessionMeta(tc.path); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestReadCodexSessionMeta_LineTooLong(t *testing.T) {
	orig := codexMetaMaxLine
	codexMetaMaxLine = 16 // force bufio.ErrTooLong on a normal fixture
	t.Cleanup(func() { codexMetaMaxLine = orig })

	path := fixtureRollout("rollout-2026-06-09T12-00-00-019eb000-0000-7000-8000-00000000a001.jsonl")
	if _, err := readCodexSessionMeta(path); err == nil {
		t.Error("expected scanner error for over-long first line")
	}
}

func TestCodexSessionsDirFnDefault(t *testing.T) {
	dir, err := codexSessionsDirFn()
	if err != nil {
		t.Fatalf("codexSessionsDirFn: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join(".codex", "sessions")) {
		t.Errorf("unexpected codex sessions dir: %q", dir)
	}
}

func TestCodexSessionsDirFnDefault_HomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := codexSessionsDirFn(); err == nil {
		t.Error("expected error when HOME is unset")
	}
}

func TestCodexLsofFnDefault(t *testing.T) {
	// Exercise the real lsof invocation against our own PID. Any macOS test
	// host has lsof; the output must contain our pid record.
	raw, err := codexLsofFn([]int{os.Getpid()})
	if err != nil && len(raw) == 0 {
		t.Fatalf("codexLsofFn: %v", err)
	}
	if !strings.Contains(string(raw), "p"+strconv.Itoa(os.Getpid())) {
		t.Errorf("expected own pid record in lsof output")
	}
}
