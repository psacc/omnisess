//go:build darwin

package procsnap

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

func TestParseTmuxPanes(t *testing.T) {
	raw := []byte("" +
		"111 alpha\n" +
		"222 my session with spaces\n" +
		"  333   beta  \n" + // leading/trailing space trimmed; name keeps inner run
		"\n" + // blank line skipped
		"notanumber gamma\n" + // non-numeric pid skipped
		"444\n" + // no separator (pid only) skipped
		"555 \n") // empty name skipped

	got := parseTmuxPanes(raw)

	want := map[int]string{
		111: "alpha",
		222: "my session with spaces",
		333: "beta", // "  beta  " -> line trimmed to "333   beta", cut on first space -> "  beta" -> trimmed? see below
	}
	// Note: line "  333   beta  " is TrimSpace'd to "333   beta", Cut on first
	// space yields name "  beta" (extra leading spaces preserved by Cut). Assert
	// the actual contract rather than a tidied guess.
	want[333] = "  beta"

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for pid, name := range want {
		if got[pid] != name {
			t.Errorf("pid %d = %q, want %q", pid, got[pid], name)
		}
	}
}

func TestResolveTmuxSession(t *testing.T) {
	panes := map[int]string{
		100: "pane-of-own-pid",
		200: "pane-of-ancestor",
	}

	tests := []struct {
		name      string
		pid       int
		ancestors []Ancestor
		want      string
	}{
		{
			name: "own pid matches (obi-fleet: exec claude directly)",
			pid:  100,
			want: "pane-of-own-pid",
		},
		{
			name: "ancestor pid matches (agent-deck: intermediate bash -c)",
			pid:  999,
			ancestors: []Ancestor{
				{PID: 200, Command: "bash", Args: "bash -c claude"},
			},
			want: "pane-of-ancestor",
		},
		{
			name: "own pid wins over ancestor",
			pid:  100,
			ancestors: []Ancestor{
				{PID: 200},
			},
			want: "pane-of-own-pid",
		},
		{
			// The regression this feature exists to prevent: a shared tmux
			// server ancestor carrying `-s obi-hestia` must NOT leak its boot
			// name onto a pane that has no PID match.
			name: "shared-server -s arg never leaks; no pid match -> empty",
			pid:  777,
			ancestors: []Ancestor{
				{PID: 5, Command: "tmux", Args: "tmux new-session -s obi-hestia"},
				{PID: 1, Command: "launchd"},
			},
			want: "",
		},
		{
			name: "empty pane map -> empty",
			pid:  100,
			want: "", // panes overridden below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := panes
			if tt.name == "empty pane map -> empty" {
				p = map[int]string{}
			}
			if got := resolveTmuxSession(tt.pid, tt.ancestors, p); got != tt.want {
				t.Errorf("resolveTmuxSession = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTmuxPanes_ErrorReturnsEmpty(t *testing.T) {
	orig := tmuxRunnerFn
	tmuxRunnerFn = func() ([]byte, error) { return nil, errors.New("tmux not running") }
	t.Cleanup(func() { tmuxRunnerFn = orig })

	if got := tmuxPanes(); len(got) != 0 {
		t.Errorf("expected empty map on error, got %#v", got)
	}
}

func TestTmuxPanes_Success(t *testing.T) {
	orig := tmuxRunnerFn
	tmuxRunnerFn = func() ([]byte, error) { return []byte("42 work\n"), nil }
	t.Cleanup(func() { tmuxRunnerFn = orig })

	got := tmuxPanes()
	if got[42] != "work" {
		t.Errorf("panes[42] = %q, want %q", got[42], "work")
	}
}

// TestEnumerate_ResolvesTmuxSession is the end-to-end guard: the live session's
// PID matches a pane, so TmuxSession is that pane's real name — and the shared
// tmux server ancestor's `-s obi-hestia` arg does not leak onto it.
func TestEnumerate_ResolvesTmuxSession(t *testing.T) {
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
		return syscall.ESRCH
	}
	t.Cleanup(func() { killFn = origKill })

	origPS := psRunnerFn
	psRunnerFn = func() ([]byte, error) {
		return []byte(`    1     0 launchd          /sbin/launchd
    5     1 tmux             tmux new-session -s obi-hestia
 5674     5 zsh              -zsh
52333  5674 claude           /usr/local/bin/claude
`), nil
	}
	t.Cleanup(func() { psRunnerFn = origPS })

	origTmux := tmuxRunnerFn
	tmuxRunnerFn = func() ([]byte, error) {
		// The pane's own pid is the claude session pid (obi-fleet style).
		return []byte("52333 feature-chore-bug-context-review\n"), nil
	}
	t.Cleanup(func() { tmuxRunnerFn = origTmux })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(snap.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(snap.Sessions))
	}
	if got := snap.Sessions[0].TmuxSession; got != "feature-chore-bug-context-review" {
		t.Errorf("TmuxSession = %q, want the pane name (never leaking -s obi-hestia)", got)
	}
}

func TestTmuxRunnerFnDefault(t *testing.T) {
	// Exercise the real tmuxRunnerFn literal for coverage. tmux may be absent
	// or not running on the test host, in which case it errors — that is a
	// valid outcome (tmuxPanes swallows it). We only require it not to panic.
	_, _ = tmuxRunnerFn()
}
