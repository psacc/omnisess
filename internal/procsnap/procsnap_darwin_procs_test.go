//go:build darwin

package procsnap

import (
	"testing"
)

func TestParsePS_Basic(t *testing.T) {
	raw := []byte(`    1     0 launchd          /sbin/launchd
  100     1 loginwindow      /System/Library/CoreServices/loginwindow.app/Contents/MacOS/loginwindow console
 3012   100 iTerm2           /Applications/iTerm.app/Contents/MacOS/iTerm2
 5674  3012 zsh              -zsh
52333  5674 claude           /usr/local/bin/claude
`)
	got, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(got))
	}
	p := got[52333]
	if p.PID != 52333 || p.PPID != 5674 || p.Command != "claude" {
		t.Errorf("wrong claude row: %+v", p)
	}
	if got[1].Command != "launchd" || got[1].PPID != 0 {
		t.Errorf("wrong launchd row: %+v", got[1])
	}
}

func TestParsePS_Empty(t *testing.T) {
	got, err := parsePS(nil)
	if err != nil {
		t.Fatalf("empty input must not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty input must yield empty map, got %d", len(got))
	}
}

func TestParsePS_SkipsMalformed(t *testing.T) {
	raw := []byte(`   1     0 launchd          /sbin/launchd
malformed line no fields
   5     1 zsh              -zsh
`)
	got, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 valid rows, got %d", len(got))
	}
}

func TestParsePS_CommandWithoutArgs(t *testing.T) {
	raw := []byte("  42     1 kernel_task\n")
	got, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS: %v", err)
	}
	p, ok := got[42]
	if !ok {
		t.Fatal("row 42 missing")
	}
	if p.Command != "kernel_task" || p.Args != "" {
		t.Errorf("comm-only row wrong: %+v", p)
	}
}

func TestParsePS_ScannerError(t *testing.T) {
	// A single line exceeding the 1MB scanner buffer limit causes sc.Err() to
	// return bufio.ErrTooLong, exercising the error-return path.
	line := make([]byte, 1024*1024+1)
	for i := range line {
		line[i] = 'x'
	}
	line[len(line)-1] = '\n'
	_, err := parsePS(line)
	if err == nil {
		t.Fatal("expected error for oversized line, got nil")
	}
}

func TestParsePS_SkipsMalformedVariants(t *testing.T) {
	// Each case exercises a distinct silent-skip branch inside parsePS.
	cases := []struct {
		name string
		line string
	}{
		// trimmed == "": blank line (spaces only) is silently skipped.
		{"blank_line", "   \n"},
		// pidEnd <= 0: line has no space after trimming (single token).
		{"no_space_at_all", "12345\n"},
		// PID Atoi fails: non-numeric PID field.
		{"non_numeric_pid", "abc  1 zsh -zsh\n"},
		// ppidEnd <= 0: PPID field has no trailing space (only one token left after PID).
		{"ppid_no_space", "  1 2\n"},
		// PPID Atoi fails: non-numeric PPID field.
		{"non_numeric_ppid", "  1 abc zsh -zsh\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePS([]byte(tc.line))
			if err != nil {
				t.Fatalf("parsePS: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("expected 0 rows (malformed skipped), got %d: %v", len(got), got)
			}
		})
	}
}

func TestWalkAncestors(t *testing.T) {
	procs := map[int]procInfo{
		1:     {PID: 1, PPID: 0, Command: "launchd"},
		100:   {PID: 100, PPID: 1, Command: "loginwindow"},
		3012:  {PID: 3012, PPID: 100, Command: "iTerm2"},
		5674:  {PID: 5674, PPID: 3012, Command: "zsh"},
		52333: {PID: 52333, PPID: 5674, Command: "claude"},
	}
	got := walkAncestors(52333, procs)
	if len(got) != 4 {
		t.Fatalf("expected 4 ancestors, got %d: %+v", len(got), got)
	}
	// Order: leaf-to-root (immediate parent first).
	wantPIDs := []int{5674, 3012, 100, 1}
	for i, want := range wantPIDs {
		if got[i].PID != want {
			t.Errorf("ancestor[%d].PID = %d, want %d", i, got[i].PID, want)
		}
	}
}

func TestWalkAncestors_Orphan(t *testing.T) {
	// parent PPID 9999 does not exist — chain stops.
	procs := map[int]procInfo{
		52333: {PID: 52333, PPID: 9999, Command: "claude"},
	}
	got := walkAncestors(52333, procs)
	if len(got) != 0 {
		t.Errorf("orphan must return empty chain, got %+v", got)
	}
}

func TestWalkAncestors_SelfReference(t *testing.T) {
	// Guard against pathological cycles.
	procs := map[int]procInfo{
		42: {PID: 42, PPID: 42, Command: "broken"},
	}
	got := walkAncestors(42, procs)
	if len(got) >= 100 {
		t.Errorf("self-reference must be bounded, got %d", len(got))
	}
}

func TestWalkAncestors_StartPIDMissing(t *testing.T) {
	procs := map[int]procInfo{1: {PID: 1, PPID: 0, Command: "launchd"}}
	got := walkAncestors(999, procs)
	if len(got) != 0 {
		t.Errorf("missing start pid must return empty, got %+v", got)
	}
}

func TestWalkAncestors_RootWithPPIDZeroNotPID1(t *testing.T) {
	// A chain that terminates at a process with PPID==0 that is NOT PID 1.
	// Exercises the `if ppid == 0 { return out }` guard.
	procs := map[int]procInfo{
		2:   {PID: 2, PPID: 0, Command: "kernel_task"},
		100: {PID: 100, PPID: 2, Command: "child"},
	}
	got := walkAncestors(100, procs)
	if len(got) != 1 || got[0].PID != 2 {
		t.Errorf("expected [kernel_task(2)], got %+v", got)
	}
}

func TestWalkAncestors_DepthExhausted(t *testing.T) {
	// Build a chain of maxAncestorDepth+2 processes so the loop limit fires.
	procs := make(map[int]procInfo, maxAncestorDepth+2)
	// PID 0 = imaginary root (PPID 0 → loop exits via ppid==0 OR depth limit).
	// To force depth exhaustion: use a long cycle-free chain where the root
	// has a non-zero PPID that's also missing from the map — but first ensure
	// we hit the depth cap. Chain: startPID → maxAncestorDepth+1 ancestors,
	// all connected; the final ancestor's PPID points somewhere not in map.
	base := 1000
	depth := maxAncestorDepth + 1
	for i := 0; i <= depth; i++ {
		pid := base + i
		ppid := base + i + 1 // all point to next; the last points outside map
		procs[pid] = procInfo{PID: pid, PPID: ppid, Command: "proc"}
	}
	got := walkAncestors(base, procs)
	// Must be bounded by maxAncestorDepth (loop runs depth times).
	if len(got) > maxAncestorDepth {
		t.Errorf("depth not bounded: got %d ancestors", len(got))
	}
}
