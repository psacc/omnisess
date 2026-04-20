# 013 — Active Claude Process Tree Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-PID accurate active detection for Claude Code sessions plus a merged process-tree view via a new `omnisess ps` subcommand and a TUI lineage overlay.

**Spec:** [`../../design-docs/2026-04-20-active-claude-process-tree.md`](../../design-docs/2026-04-20-active-claude-process-tree.md)

**Architecture:** New `internal/procsnap/` package reads `~/.claude/sessions/<PID>.json`, verifies liveness via `kill(pid, 0)`, runs `ps -Ao pid=,ppid=,comm=,args=` once, walks ancestors up to `launchd` (PID 1). Two consumers: `cmd/ps.go` (new subcommand, text + `--json`) and `internal/tui/model.go` (accurate `Active` flag + `l` lineage overlay + 5s refresh tick).

**Tech Stack:** Go 1.25, Cobra, bubbletea, lipgloss, stdlib `os/exec`, `syscall`. Pure Go — no new `go.mod` dependencies.

---

## Pre-work

- [ ] **Step 0: Confirm branch.** Run `git status` and verify you are on `claude/sweet-mayer-e30e29` (or a sub-branch off it). Run `make check` to confirm a green baseline before touching anything.

---

## Track A — `internal/procsnap/` package (7 tasks)

Each task follows strict TDD. The package is claude-only: no other tool references leak in.

### Task A1: Package skeleton, types, ErrUnsupported, dispatch

**Files:**
- Create: `internal/procsnap/procsnap.go`
- Create: `internal/procsnap/procsnap_other.go`
- Create: `internal/procsnap/procsnap_test.go`

- [ ] **Step 1: Write the failing tests.**

Add to `internal/procsnap/procsnap_test.go`:

```go
package procsnap

import (
	"errors"
	"testing"
)

func TestSnapshot_IsActive_EmptySnapshot(t *testing.T) {
	s := Snapshot{}
	if s.IsActive("anything") {
		t.Error("empty snapshot must never report active")
	}
}

func TestSnapshot_IsActive_Match(t *testing.T) {
	s := Snapshot{Sessions: []Session{{SessionID: "abc"}, {SessionID: "def"}}}
	if !s.IsActive("abc") {
		t.Error("expected abc to be active")
	}
	if s.IsActive("xyz") {
		t.Error("xyz must not be active")
	}
}

func TestErrUnsupported_Defined(t *testing.T) {
	if ErrUnsupported == nil {
		t.Fatal("ErrUnsupported sentinel must be non-nil")
	}
	if !errors.Is(ErrUnsupported, ErrUnsupported) {
		t.Error("ErrUnsupported must match itself via errors.Is")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

Run: `go test ./internal/procsnap/...`
Expected: compile error — `Snapshot`, `Session`, `ErrUnsupported` undefined.

- [ ] **Step 3: Write `procsnap.go` with types and API.**

```go
// Package procsnap enumerates live Claude Code sessions by correlating
// ~/.claude/sessions/<PID>.json registry files with the running process
// table. macOS only; other platforms return ErrUnsupported.
package procsnap

import (
	"errors"
	"time"
)

// ErrUnsupported is returned by Enumerate on platforms that are not macOS.
var ErrUnsupported = errors.New("procsnap: unsupported platform")

// Session is one live Claude Code process with its ancestor chain.
type Session struct {
	PID        int
	SessionID  string
	Name       string    // from /rename, empty if unset
	CWD        string
	StartedAt  time.Time
	Entrypoint string    // "cli" | "claude-desktop" | other
	Version    string
	Ancestors  []Ancestor // index 0 = immediate parent, last = root
}

// Ancestor is one step up the process tree above a claude session.
type Ancestor struct {
	PID     int
	Command string // comm (short executable name)
	Args    string // trimmed full args line
}

// Snapshot is a point-in-time view of all live Claude sessions.
type Snapshot struct {
	Sessions []Session
	Built    time.Time
}

// IsActive reports whether the given Claude session ID is backed by a
// live process in this snapshot.
func (s Snapshot) IsActive(sessionID string) bool {
	for i := range s.Sessions {
		if s.Sessions[i].SessionID == sessionID {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Write `procsnap_other.go` stub.**

```go
//go:build !darwin

package procsnap

// Enumerate returns ErrUnsupported on non-macOS platforms. Callers fall back
// to their previous behavior.
func Enumerate() (Snapshot, error) {
	return Snapshot{}, ErrUnsupported
}
```

- [ ] **Step 5: Run tests to verify they pass.**

Run: `go test ./internal/procsnap/... -v`
Expected: 3 tests pass. Note the `TestErrUnsupported_Is` only asserts the non-match case (documents that we intentionally do not wrap — it is a sentinel, compared with `==`).

- [ ] **Step 6: Commit.**

```bash
git add internal/procsnap/
git commit -m "feat(procsnap): package skeleton with types and ErrUnsupported"
```

---

### Task A2: Darwin — session file scanner

**Files:**
- Create: `internal/procsnap/procsnap_darwin.go`
- Modify: `internal/procsnap/procsnap_test.go`
- Create: `internal/procsnap/testdata/sessions/52333.json`
- Create: `internal/procsnap/testdata/sessions/9999.json`
- Create: `internal/procsnap/testdata/sessions/garbage.json`

- [ ] **Step 1: Create testdata fixtures.**

`internal/procsnap/testdata/sessions/52333.json`:
```json
{"pid":52333,"sessionId":"eaa9deaf-6c97-4233-9629-9c425ebf4457","cwd":"/Users/me/prj/foo","startedAt":1776680734830,"version":"2.1.114","kind":"interactive","entrypoint":"cli","name":"refactor auth"}
```

`internal/procsnap/testdata/sessions/9999.json`:
```json
{"pid":9999,"sessionId":"dead-session","cwd":"/Users/me/gone","startedAt":1776000000000,"kind":"interactive","entrypoint":"claude-desktop"}
```

`internal/procsnap/testdata/sessions/garbage.json`:
```
not valid json {{{
```

- [ ] **Step 2: Write the failing test.**

Add to `internal/procsnap/procsnap_test.go`:

```go
import (
	"path/filepath"
	"time"
)

func TestScanSessionDir_HappyPath(t *testing.T) {
	entries, err := scanSessionDir(filepath.Join("testdata", "sessions"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries (garbage.json skipped), got %d", len(entries))
	}
	byPID := map[int]sessionFile{}
	for _, e := range entries {
		byPID[e.PID] = e
	}
	got := byPID[52333]
	if got.SessionID != "eaa9deaf-6c97-4233-9629-9c425ebf4457" {
		t.Errorf("wrong sessionId: %q", got.SessionID)
	}
	if got.Name != "refactor auth" {
		t.Errorf("name not parsed: %q", got.Name)
	}
	if got.Entrypoint != "cli" {
		t.Errorf("entrypoint: %q", got.Entrypoint)
	}
	wantStart := time.UnixMilli(1776680734830)
	if !got.StartedAt.Equal(wantStart) {
		t.Errorf("startedAt = %v, want %v", got.StartedAt, wantStart)
	}

	desk := byPID[9999]
	if desk.Entrypoint != "claude-desktop" {
		t.Errorf("desktop entrypoint: %q", desk.Entrypoint)
	}
	if desk.Name != "" {
		t.Errorf("desktop name should be empty, got %q", desk.Name)
	}
}

func TestScanSessionDir_Missing(t *testing.T) {
	entries, err := scanSessionDir(filepath.Join("testdata", "nonexistent"))
	if err != nil {
		t.Fatalf("missing dir must not error, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("missing dir must yield empty slice, got %d", len(entries))
	}
}
```

- [ ] **Step 3: Run the test to verify it fails.**

Run: `go test ./internal/procsnap/... -run TestScanSessionDir`
Expected: compile error — `scanSessionDir` and `sessionFile` undefined.

- [ ] **Step 4: Implement `procsnap_darwin.go` scanner.**

```go
//go:build darwin

package procsnap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// sessionFile is the parsed shape of ~/.claude/sessions/<PID>.json.
type sessionFile struct {
	PID        int
	SessionID  string
	Name       string
	CWD        string
	StartedAt  time.Time
	Version    string
	Entrypoint string
}

// rawSessionFile mirrors the JSON keys emitted by Claude Code.
type rawSessionFile struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"` // unix millis
	Version    string `json:"version"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
}

// scanSessionDir reads every *.json file in dir and returns the parsed
// entries. Missing directories return an empty slice and nil error.
// Unreadable or malformed files are logged to stderr and skipped.
func scanSessionDir(dir string) ([]sessionFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("procsnap: reading %s: %w", dir, err)
	}

	out := make([]sessionFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "procsnap: reading %s: %v\n", path, err)
			continue
		}
		var raw rawSessionFile
		if err := json.Unmarshal(data, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "procsnap: parsing %s: %v\n", path, err)
			continue
		}
		if raw.PID == 0 || raw.SessionID == "" {
			continue
		}
		out = append(out, sessionFile{
			PID:        raw.PID,
			SessionID:  raw.SessionID,
			Name:       raw.Name,
			CWD:        raw.CWD,
			StartedAt:  time.UnixMilli(raw.StartedAt),
			Version:    raw.Version,
			Entrypoint: raw.Entrypoint,
		})
	}
	return out, nil
}
```

- [ ] **Step 5: Run the test to verify it passes.**

Run: `go test ./internal/procsnap/... -run TestScanSessionDir -v`
Expected: both tests pass. Stderr will show a "parsing ... garbage.json" warning — that is intentional.

- [ ] **Step 6: Commit.**

```bash
git add internal/procsnap/procsnap_darwin.go internal/procsnap/testdata/ internal/procsnap/procsnap_test.go
git commit -m "feat(procsnap): scan ~/.claude/sessions/<PID>.json files"
```

---

### Task A3: Liveness check with injectable `killFn`

**Files:**
- Modify: `internal/procsnap/procsnap_darwin.go`
- Modify: `internal/procsnap/procsnap_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/procsnap/procsnap_test.go`:

```go
func TestFilterAlive(t *testing.T) {
	orig := killFn
	killFn = func(pid int) error {
		if pid == 52333 {
			return nil // alive
		}
		return syscall.ESRCH // dead
	}
	t.Cleanup(func() { killFn = orig })

	in := []sessionFile{
		{PID: 52333, SessionID: "a"},
		{PID: 9999, SessionID: "b"},
		{PID: 42, SessionID: "c"},
	}
	got := filterAlive(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 alive, got %d", len(got))
	}
	if got[0].PID != 52333 {
		t.Errorf("wrong alive PID: %d", got[0].PID)
	}
}
```

Add the import `"syscall"` at the top of the test file if not already present.

- [ ] **Step 2: Run the test to verify it fails.**

Run: `go test ./internal/procsnap/... -run TestFilterAlive`
Expected: compile error — `killFn` and `filterAlive` undefined.

- [ ] **Step 3: Add `filterAlive` and `killFn` to `procsnap_darwin.go`.**

Append to `internal/procsnap/procsnap_darwin.go`:

```go
import "syscall"  // add to the existing import block

// killFn is injectable for tests. syscall.Kill(pid, 0) is the portable
// liveness probe: returns nil if the process exists and we can signal it.
var killFn = func(pid int) error { return syscall.Kill(pid, 0) }

// filterAlive returns only entries whose PID is currently a live process.
func filterAlive(in []sessionFile) []sessionFile {
	out := make([]sessionFile, 0, len(in))
	for _, e := range in {
		if err := killFn(e.PID); err == nil {
			out = append(out, e)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes.**

Run: `go test ./internal/procsnap/... -run TestFilterAlive -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/procsnap/
git commit -m "feat(procsnap): drop stale PID registry entries via kill(0)"
```

---

### Task A4: Parse `ps` output

**Files:**
- Modify: `internal/procsnap/procsnap_darwin.go`
- Modify: `internal/procsnap/procsnap_test.go`

- [ ] **Step 1: Write the failing tests.**

Add to `internal/procsnap/procsnap_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail.**

Run: `go test ./internal/procsnap/... -run TestParsePS`
Expected: compile error — `parsePS` and `procInfo` undefined.

- [ ] **Step 3: Implement `parsePS`.**

Append to `internal/procsnap/procsnap_darwin.go`:

```go
import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)  // merge into existing imports

// procInfo is one row from `ps -Ao pid=,ppid=,comm=,args=`.
type procInfo struct {
	PID     int
	PPID    int
	Command string
	Args    string
}

// parsePS parses the output of `ps -Ao pid=,ppid=,comm=,args=` into a
// pid-indexed map. Malformed lines are silently skipped (ps output is
// whitespace-delimited and fixed-position on macOS, so a malformed line
// is almost always a truncation or synthetic environment).
func parsePS(raw []byte) (map[int]procInfo, error) {
	out := make(map[int]procInfo)
	if len(raw) == 0 {
		return out, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		// Parse PID.
		pidEnd := strings.IndexByte(trimmed, ' ')
		if pidEnd <= 0 {
			continue
		}
		pid, err := strconv.Atoi(trimmed[:pidEnd])
		if err != nil {
			continue
		}
		rest := strings.TrimLeft(trimmed[pidEnd:], " ")
		// Parse PPID.
		ppidEnd := strings.IndexByte(rest, ' ')
		if ppidEnd <= 0 {
			continue
		}
		ppid, err := strconv.Atoi(rest[:ppidEnd])
		if err != nil {
			continue
		}
		rest = strings.TrimLeft(rest[ppidEnd:], " ")
		// Parse comm (next whitespace-delimited field).
		commEnd := strings.IndexByte(rest, ' ')
		if commEnd <= 0 {
			// No args column — comm is the whole remainder.
			out[pid] = procInfo{PID: pid, PPID: ppid, Command: rest}
			continue
		}
		command := rest[:commEnd]
		args := strings.TrimLeft(rest[commEnd:], " ")
		out[pid] = procInfo{PID: pid, PPID: ppid, Command: command, Args: args}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass.**

Run: `go test ./internal/procsnap/... -run TestParsePS -v`
Expected: all three pass.

- [ ] **Step 5: Commit.**

```bash
git add internal/procsnap/
git commit -m "feat(procsnap): parse ps output into pid-indexed map"
```

---

### Task A5: Ancestor walker

**Files:**
- Modify: `internal/procsnap/procsnap_darwin.go`
- Modify: `internal/procsnap/procsnap_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/procsnap/procsnap_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail.**

Run: `go test ./internal/procsnap/... -run TestWalkAncestors`
Expected: compile error — `walkAncestors` undefined.

- [ ] **Step 3: Implement `walkAncestors`.**

Append to `internal/procsnap/procsnap_darwin.go`:

```go
// maxAncestorDepth bounds the walk against pathological PPID cycles.
// A real Unix process tree is never deeper than a few dozen levels.
const maxAncestorDepth = 64

// walkAncestors returns the chain of processes above startPID, ordered
// leaf-to-root. Stops at PID 1, an orphan (PPID missing from procs),
// or a detected cycle.
func walkAncestors(startPID int, procs map[int]procInfo) []Ancestor {
	var out []Ancestor
	self, ok := procs[startPID]
	if !ok {
		return out
	}
	seen := map[int]bool{startPID: true}
	ppid := self.PPID
	for depth := 0; depth < maxAncestorDepth; depth++ {
		if ppid == 0 {
			return out
		}
		if seen[ppid] {
			return out
		}
		seen[ppid] = true
		parent, ok := procs[ppid]
		if !ok {
			return out
		}
		out = append(out, Ancestor{
			PID:     parent.PID,
			Command: parent.Command,
			Args:    parent.Args,
		})
		if parent.PID == 1 {
			return out
		}
		ppid = parent.PPID
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass.**

Run: `go test ./internal/procsnap/... -run TestWalkAncestors -v`
Expected: all three pass.

- [ ] **Step 5: Commit.**

```bash
git add internal/procsnap/
git commit -m "feat(procsnap): walk ancestor chain up to launchd"
```

---

### Task A6: `Enumerate` wiring with injectable `psRunnerFn` and `sessionsDirFn`

**Files:**
- Modify: `internal/procsnap/procsnap_darwin.go`
- Modify: `internal/procsnap/procsnap_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/procsnap/procsnap_test.go`:

```go
func TestEnumerate_HappyPath(t *testing.T) {
	// Two PID files in testdata; only 52333 will be alive.
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
 5674     1 zsh              -zsh
52333  5674 claude           /usr/local/bin/claude
`), nil
	}
	t.Cleanup(func() { psRunnerFn = origPS })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(snap.Sessions) != 1 {
		t.Fatalf("expected 1 live session, got %d", len(snap.Sessions))
	}
	s := snap.Sessions[0]
	if s.PID != 52333 {
		t.Errorf("pid = %d", s.PID)
	}
	if s.Name != "refactor auth" {
		t.Errorf("name = %q", s.Name)
	}
	if len(s.Ancestors) != 2 {
		t.Fatalf("expected 2 ancestors, got %d: %+v", len(s.Ancestors), s.Ancestors)
	}
	if s.Ancestors[0].Command != "zsh" || s.Ancestors[1].Command != "launchd" {
		t.Errorf("ancestors: %+v", s.Ancestors)
	}
	if snap.Built.IsZero() {
		t.Error("Built must be set")
	}
	if !snap.IsActive("eaa9deaf-6c97-4233-9629-9c425ebf4457") {
		t.Error("IsActive must match live session ID")
	}
}

func TestEnumerate_PSFails(t *testing.T) {
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "sessions"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	origKill := killFn
	killFn = func(pid int) error { return nil }
	t.Cleanup(func() { killFn = origKill })

	origPS := psRunnerFn
	psRunnerFn = func() ([]byte, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { psRunnerFn = origPS })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("ps failure must not propagate, got %v", err)
	}
	if len(snap.Sessions) == 0 {
		t.Error("sessions must still be returned (without ancestors) when ps fails")
	}
	if len(snap.Sessions[0].Ancestors) != 0 {
		t.Error("ancestors must be empty when ps failed")
	}
}

func TestEnumerate_NoSessionsDir(t *testing.T) {
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return filepath.Join("testdata", "nonexistent"), nil
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	snap, err := Enumerate()
	if err != nil {
		t.Fatalf("missing dir must not error: %v", err)
	}
	if len(snap.Sessions) != 0 {
		t.Errorf("missing dir must yield empty snapshot, got %d sessions", len(snap.Sessions))
	}
}

func TestEnumerate_HomeDirError(t *testing.T) {
	origDir := sessionsDirFn
	sessionsDirFn = func() (string, error) {
		return "", errors.New("no home")
	}
	t.Cleanup(func() { sessionsDirFn = origDir })

	_, err := Enumerate()
	if err == nil {
		t.Error("expected error when home dir cannot be resolved")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

Run: `go test ./internal/procsnap/... -run TestEnumerate`
Expected: compile error — `psRunnerFn`, `sessionsDirFn`, `Enumerate` (darwin version) undefined.

- [ ] **Step 3: Implement the wiring.**

Append to `internal/procsnap/procsnap_darwin.go`:

```go
import "os/exec"  // add to existing imports

// psRunnerFn is injectable. Real impl runs `ps -Ao pid=,ppid=,comm=,args=`
// which emits one row per process with no header.
var psRunnerFn = func() ([]byte, error) {
	cmd := exec.Command("ps", "-Ao", "pid=,ppid=,comm=,args=")
	return cmd.Output()
}

// sessionsDirFn is injectable. Real impl resolves ~/.claude/sessions/.
var sessionsDirFn = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "sessions"), nil
}

// Enumerate scans the Claude session registry, drops stale PID files,
// walks ancestor chains, and returns a snapshot. On macOS it never
// returns ErrUnsupported.
func Enumerate() (Snapshot, error) {
	dir, err := sessionsDirFn()
	if err != nil {
		return Snapshot{}, fmt.Errorf("procsnap: resolving sessions dir: %w", err)
	}

	files, err := scanSessionDir(dir)
	if err != nil {
		return Snapshot{}, err
	}
	alive := filterAlive(files)

	var procs map[int]procInfo
	raw, psErr := psRunnerFn()
	if psErr != nil {
		fmt.Fprintf(os.Stderr, "procsnap: ps failed: %v\n", psErr)
		procs = map[int]procInfo{}
	} else {
		procs, _ = parsePS(raw)
	}

	sessions := make([]Session, 0, len(alive))
	for _, f := range alive {
		sessions = append(sessions, Session{
			PID:        f.PID,
			SessionID:  f.SessionID,
			Name:       f.Name,
			CWD:        f.CWD,
			StartedAt:  f.StartedAt,
			Entrypoint: f.Entrypoint,
			Version:    f.Version,
			Ancestors:  walkAncestors(f.PID, procs),
		})
	}
	return Snapshot{Sessions: sessions, Built: time.Now()}, nil
}
```

- [ ] **Step 4: Run all procsnap tests to verify they pass.**

Run: `go test ./internal/procsnap/... -v`
Expected: every test in the package passes.

- [ ] **Step 5: Verify 100% coverage for the package.**

Run: `make cover-pkg PKG=./internal/procsnap/...`
Expected: final line shows `total: (statements) 100.0%`. If below 100%, identify the uncovered lines with `go tool cover -html=coverage.out` and add a table-driven case until coverage is 100%.

- [ ] **Step 6: Commit.**

```bash
git add internal/procsnap/
git commit -m "feat(procsnap): Enumerate wires scan, liveness, ps parse, ancestor walk"
```

---

### Task A7: Smoke test against real `~/.claude/sessions/`

**Files:** None (manual verification)

- [ ] **Step 1: Build the binary.**

Run: `go build -o /tmp/omnisess-a7 .`
Expected: exit 0, binary produced. (We have no CLI surface yet; the smoke uses a one-off test binary in Task B2.)

- [ ] **Step 2: Write an ad-hoc smoke test.**

Create a temporary file `/tmp/procsnap_smoke.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/psacc/omnisess/internal/procsnap"
)

func main() {
	snap, err := procsnap.Enumerate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(snap)
}
```

Run: `go run /tmp/procsnap_smoke.go` from the repo root.
Expected: prints at least one claude session if you have one running (including this one). `Ancestors` should be populated and terminate at `launchd` (PID 1).

- [ ] **Step 3: Delete the smoke file.**

Run: `rm /tmp/procsnap_smoke.go`
This is a one-off. Do not commit.

---

## Track B — `omnisess ps` subcommand (3 tasks)

### Task B1: Command skeleton

**Files:**
- Create: `cmd/ps.go`
- Create: `cmd/ps_test.go`

- [ ] **Step 1: Write the failing test.**

Create `cmd/ps_test.go`:

```go
package cmd

import (
	"bytes"
	"testing"

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
```

- [ ] **Step 2: Run the test to verify it fails.**

Run: `go test ./cmd/... -run TestRunPS_Unsupported`
Expected: compile error — `runPSWith` undefined.

- [ ] **Step 3: Create `cmd/ps.go`.**

```go
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/psacc/omnisess/internal/procsnap"
)

var flagPSTree bool // reserved for future; default output is the tree

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show active Claude sessions as a process tree",
	Long:  "List live Claude Code sessions (CLI + Claude Desktop agent mode), grouped by shared ancestor chain up to launchd.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPSWith(os.Stdout, procsnap.Enumerate, flagJSON)
	},
}

func init() {
	rootCmd.AddCommand(psCmd)
}

// enumerator matches procsnap.Enumerate's signature for dependency injection.
type enumerator func() (procsnap.Snapshot, error)

func runPSWith(out io.Writer, enum enumerator, asJSON bool) error {
	snap, err := enum()
	if err != nil {
		if errors.Is(err, procsnap.ErrUnsupported) {
			fmt.Fprintln(out, "omnisess ps: unsupported on this platform (macOS only)")
			return nil
		}
		return err
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	}

	if len(snap.Sessions) == 0 {
		fmt.Fprintln(out, "No live Claude sessions.")
		return nil
	}
	renderTree(out, snap)
	return nil
}

// renderTree is defined in Task B2.
func renderTree(out io.Writer, snap procsnap.Snapshot) { /* filled in Task B2 */ }
```

- [ ] **Step 4: Run the test to verify it passes.**

Run: `go test ./cmd/... -run TestRunPS_Unsupported -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add cmd/ps.go cmd/ps_test.go
git commit -m "feat(cmd): omnisess ps skeleton with ErrUnsupported handling"
```

---

### Task B2: Merged tree renderer

**Files:**
- Modify: `cmd/ps.go`
- Modify: `cmd/ps_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `cmd/ps_test.go`:

```go
import "time"

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

func TestRenderTree_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderTree(&buf, procsnap.Snapshot{})
	if buf.Len() != 0 {
		t.Errorf("empty snapshot must produce no output, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail.**

Run: `go test ./cmd/... -run TestRenderTree`
Expected: first test fails — tree contains two "launchd" / "iTerm2" (stub `renderTree` is a no-op).

- [ ] **Step 3: Implement `renderTree`.**

Replace the stub in `cmd/ps.go` with:

```go
import (
	"sort"
	"strings"
	"time"
)  // merge into existing imports

// treeNode is a transient representation used only during rendering.
type treeNode struct {
	label    string         // what to print on this line
	children []*treeNode    // ancestors-as-children going downward
	byPID    map[int]*treeNode
}

func newNode(label string) *treeNode {
	return &treeNode{label: label, byPID: map[int]*treeNode{}}
}

// renderTree prints the merged process tree to out.
func renderTree(out io.Writer, snap procsnap.Snapshot) {
	if len(snap.Sessions) == 0 {
		return
	}

	root := newNode("") // virtual super-root; its children are the true roots
	for _, s := range snap.Sessions {
		// Ancestors are leaf-to-root; flip so root is first.
		chain := make([]procsnap.Ancestor, len(s.Ancestors))
		for i, a := range s.Ancestors {
			chain[len(chain)-1-i] = a
		}

		parent := root
		for _, a := range chain {
			child, ok := parent.byPID[a.PID]
			if !ok {
				label := fmt.Sprintf("%s (%d)", a.Command, a.PID)
				child = newNode(label)
				parent.byPID[a.PID] = child
				parent.children = append(parent.children, child)
			}
			parent = child
		}
		leaf := newNode(leafLabel(s, snap.Built))
		parent.children = append(parent.children, leaf)
	}

	for _, top := range root.children {
		printNode(out, top, "", true)
	}
}

func leafLabel(s procsnap.Session, now time.Time) string {
	name := s.Name
	if name == "" {
		name = shortID(s.SessionID)
	}
	project := projectBase(s.CWD)
	age := formatAge(now.Sub(s.StartedAt))
	entry := s.Entrypoint
	if entry == "claude-desktop" {
		entry = "desktop"
	}
	return fmt.Sprintf("claude  %s  %s (%s)  %s  %s",
		name, project, shortID(s.SessionID), entry, age)
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func projectBase(cwd string) string {
	if cwd == "" {
		return "-"
	}
	idx := strings.LastIndex(cwd, "/")
	if idx < 0 || idx == len(cwd)-1 {
		return cwd
	}
	return cwd[idx+1:]
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func printNode(out io.Writer, n *treeNode, prefix string, isLast bool) {
	var connector string
	var nextPrefix string
	if prefix == "" && isLast {
		// Top-level node: no connector.
		fmt.Fprintln(out, n.label)
		nextPrefix = ""
	} else {
		if isLast {
			connector = "└─ "
			nextPrefix = prefix + "   "
		} else {
			connector = "├─ "
			nextPrefix = prefix + "│  "
		}
		fmt.Fprintf(out, "%s%s%s\n", prefix, connector, n.label)
	}

	// Stable ordering: named claude leaves first, then by label.
	sort.SliceStable(n.children, func(i, j int) bool {
		return n.children[i].label < n.children[j].label
	})
	for i, c := range n.children {
		last := i == len(n.children)-1
		var childPrefix string
		if prefix == "" && connector == "" {
			childPrefix = ""
		} else {
			childPrefix = nextPrefix
		}
		printNode(out, c, childPrefix, last)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass.**

Run: `go test ./cmd/... -run TestRenderTree -v`
Expected: both tests pass.

- [ ] **Step 5: Manual smoke test.**

Run: `go run . ps`
Expected: tree of your currently running claude sessions. Must include this session (omnisess working copy). If nothing prints but sessions exist, check that `~/.claude/sessions/<PID>.json` has a live PID via `ps -p <pid>`.

- [ ] **Step 6: Commit.**

```bash
git add cmd/ps.go cmd/ps_test.go
git commit -m "feat(cmd): omnisess ps renders merged ancestor tree"
```

---

### Task B3: `--json` flag

**Files:**
- Modify: `cmd/ps_test.go`

`--json` already exists on `rootCmd`. `runPSWith` already honors it. This task just adds a test.

- [ ] **Step 1: Write the failing test.**

Add to `cmd/ps_test.go`:

```go
import "encoding/json"

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
```

- [ ] **Step 2: Run the test to verify it passes.**

Run: `go test ./cmd/... -run TestRunPS_JSON -v`
Expected: PASS — this is a belt-and-suspenders test; the code path is already implemented.

- [ ] **Step 3: Commit.**

```bash
git add cmd/ps_test.go
git commit -m "test(cmd): cover omnisess ps --json output"
```

---

## Track C — TUI integration (4 tasks)

### Task C1: Pass snapshot into TUI and override claude `Active`

**Files:**
- Modify: `cmd/tui.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/tui/model_test.go`:

```go
import (
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/procsnap"
)

func TestApplySnapshot_OverridesClaudeActive(t *testing.T) {
	sessions := []model.Session{
		{ID: "live-claude", Tool: model.ToolClaude, Active: false, UpdatedAt: time.Now()},
		{ID: "dead-claude", Tool: model.ToolClaude, Active: true, UpdatedAt: time.Now()},
		{ID: "any-cursor", Tool: model.ToolCursor, Active: true, UpdatedAt: time.Now()},
	}
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{{SessionID: "live-claude"}},
	}
	got := applySnapshot(sessions, snap)
	if !got[0].Active {
		t.Errorf("live claude must become Active=true")
	}
	if got[1].Active {
		t.Errorf("claude not in snapshot must become Active=false")
	}
	if !got[2].Active {
		t.Errorf("cursor Active must be untouched (was true)")
	}
}

func TestApplySnapshot_UnsupportedIsNoop(t *testing.T) {
	sessions := []model.Session{
		{ID: "x", Tool: model.ToolClaude, Active: true, UpdatedAt: time.Now()},
	}
	got := applySnapshot(sessions, procsnap.Snapshot{})
	// Empty snapshot overrides: claude session becomes inactive.
	// This matches "we believe the snapshot when we have it"; callers that
	// receive ErrUnsupported must not call applySnapshot.
	if got[0].Active {
		t.Error("empty snapshot must mark claude sessions inactive")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails.**

Run: `go test ./internal/tui/... -run TestApplySnapshot`
Expected: compile error — `applySnapshot` undefined.

- [ ] **Step 3: Add `applySnapshot` to `internal/tui/model.go`.**

Append (below existing exported helpers):

```go
import "github.com/psacc/omnisess/internal/procsnap"  // add to the existing import block

// applySnapshot overrides the Active flag for every claude session
// based on the snapshot. Non-claude sessions are untouched. Caller must
// only invoke this when Enumerate returned nil error (never on
// ErrUnsupported).
func applySnapshot(sessions []model.Session, snap procsnap.Snapshot) []model.Session {
	live := make(map[string]bool, len(snap.Sessions))
	for _, s := range snap.Sessions {
		live[s.SessionID] = true
	}
	for i := range sessions {
		if sessions[i].Tool == model.ToolClaude {
			sessions[i].Active = live[sessions[i].ID]
		}
	}
	return sessions
}
```

- [ ] **Step 4: Export `applySnapshot` as `ApplySnapshot`.**

Rename the function in `internal/tui/model.go` and update the test references. Callers from `cmd/tui.go` will use the exported name.

- [ ] **Step 5: Wire in `cmd/tui.go`.**

Add imports:

```go
import "errors"                                                   // if not already present
import "github.com/psacc/omnisess/internal/procsnap"              // new
```

In `runTUI`, after `all = all[:opts.Limit]` and the empty-check, but before `toolModes := buildToolModes()`, insert:

```go
// Enrich Claude Active flags from the live process snapshot. On
// unsupported platforms or snapshot errors we keep the existing
// (mtime-based) flags. snapOK controls whether we also attach the
// snapshot to the TUI model for the lineage overlay (Task C3) and
// the refresh tick (Task C4).
var (
	snap     procsnap.Snapshot
	snapOK   bool
)
if s, err := procsnap.Enumerate(); err == nil {
	snap = s
	snapOK = true
	all = tui.ApplySnapshot(all, snap)
} else if !errors.Is(err, procsnap.ErrUnsupported) {
	fmt.Fprintf(os.Stderr, "warning: procsnap: %v\n", err)
}
```

(The `snap` / `snapOK` variables are unused after this task — they will be consumed in C3 and C4. That's acceptable because we keep the Go compiler happy by referring to `snap` in the `_ = snap` line below; remove the throwaway when C3 lands.)

Add one line immediately below the block above:

```go
_ = snap
_ = snapOK
```

- [ ] **Step 6: Run tests.**

Run: `go test ./internal/tui/... ./cmd/... -v`
Expected: all green.

- [ ] **Step 7: Commit.**

```bash
git add internal/tui/model.go internal/tui/model_test.go cmd/tui.go
git commit -m "feat(tui): override claude Active flag from procsnap snapshot"
```

---

### Task C2: Name cascade — prefer `/rename` name from snapshot

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/tui/model_test.go`:

```go
func TestApplySnapshot_PopulatesRenameTitle(t *testing.T) {
	sessions := []model.Session{
		{ID: "aaa", Tool: model.ToolClaude, Title: "", UpdatedAt: time.Now()},
		{ID: "bbb", Tool: model.ToolClaude, Title: "existing preview", UpdatedAt: time.Now()},
	}
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{
			{SessionID: "aaa", Name: "pair with alice"},
			{SessionID: "bbb", Name: ""}, // no /rename
		},
	}
	got := ApplySnapshot(sessions, snap)
	if got[0].Title != "pair with alice" {
		t.Errorf("Title = %q, want %q", got[0].Title, "pair with alice")
	}
	if got[1].Title != "existing preview" {
		t.Errorf("empty rename must not overwrite existing Title, got %q", got[1].Title)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails.**

Run: `go test ./internal/tui/... -run TestApplySnapshot_PopulatesRenameTitle`
Expected: FAIL — `got[0].Title` is empty.

- [ ] **Step 3: Extend `ApplySnapshot`.**

Replace the body of `ApplySnapshot` with:

```go
func ApplySnapshot(sessions []model.Session, snap procsnap.Snapshot) []model.Session {
	bySessionID := make(map[string]procsnap.Session, len(snap.Sessions))
	for _, s := range snap.Sessions {
		bySessionID[s.SessionID] = s
	}
	for i := range sessions {
		if sessions[i].Tool != model.ToolClaude {
			continue
		}
		live, ok := bySessionID[sessions[i].ID]
		sessions[i].Active = ok
		if ok && live.Name != "" {
			sessions[i].Title = live.Name
		}
	}
	return sessions
}
```

- [ ] **Step 4: Run the test to verify it passes.**

Run: `go test ./internal/tui/... -v`
Expected: all green.

- [ ] **Step 5: Commit.**

```bash
git add internal/tui/
git commit -m "feat(tui): prefer /rename name from snapshot as session Title"
```

---

### Task C3: Lineage overlay on `l` key

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`
- Modify: `cmd/tui.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/tui/model_test.go`:

```go
import tea "github.com/charmbracelet/bubbletea"

func TestModel_LineageOverlay_ToggleAndDismiss(t *testing.T) {
	sessions := []model.Session{
		{ID: "aaa", Tool: model.ToolClaude, UpdatedAt: time.Now(), Active: true},
	}
	snap := procsnap.Snapshot{
		Sessions: []procsnap.Session{{
			SessionID: "aaa",
			PID:       1234,
			Ancestors: []procsnap.Ancestor{
				{PID: 100, Command: "zsh"},
				{PID: 1, Command: "launchd"},
			},
		}},
	}
	m := New(sessions, nil)
	m.SetSnapshot(snap)

	// Press 'l' — overlay becomes visible.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	view := m2.View()
	if !bytesContains(view, "Lineage") || !bytesContains(view, "zsh") || !bytesContains(view, "launchd") {
		t.Errorf("expected lineage overlay with ancestors, got:\n%s", view)
	}

	// Press Esc — overlay dismissed.
	m3, _ := m2.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	view2 := m3.View()
	if bytesContains(view2, "Lineage") {
		t.Errorf("overlay must dismiss on Esc, still shown:\n%s", view2)
	}
}

func bytesContains(s string, sub string) bool { return strings.Contains(s, sub) }
```

(Make sure `strings` and `tea` imports are at the top of the test file.)

- [ ] **Step 2: Run the test to verify it fails.**

Run: `go test ./internal/tui/... -run TestModel_LineageOverlay`
Expected: compile error — `SetSnapshot` undefined; `l`/`esc` handlers missing.

- [ ] **Step 3: Extend `Model` with lineage state and handlers.**

In `internal/tui/model.go`:

```go
// Add fields to Model struct:
type Model struct {
	// ... existing fields ...
	snapshot       procsnap.Snapshot
	showingLineage bool
}

// SetSnapshot attaches a snapshot used by the lineage overlay and active flag.
func (m *Model) SetSnapshot(snap procsnap.Snapshot) {
	m.snapshot = snap
}
```

In the `Update` method, **replace** the existing line

```go
case "q", "esc", "ctrl+c":
```

with these three cases (same block, ordered this way):

```go
case "l":
	m.showingLineage = true
	m.message = ""
	return m, nil
case "esc":
	if m.showingLineage {
		m.showingLineage = false
		return m, nil
	}
	m.quitting = true
	return m, tea.Quit
case "q", "ctrl+c":
	m.quitting = true
	return m, tea.Quit
```

The body of the original `q/esc/ctrl+c` branch already sets `m.quitting = true; return m, tea.Quit` — keep that body under `case "q", "ctrl+c":` and delete the line that was there before (originally shared between all three).

At the end of `View()`, before the final `return`, append:

```go
if m.showingLineage && len(m.sessions) > 0 {
	b.WriteString(m.renderLineage())
}
```

Add the `renderLineage` method:

```go
func (m Model) renderLineage() string {
	sess := m.sessions[m.cursor]
	if sess.Tool != model.ToolClaude {
		return styleFooter.Render("Lineage unavailable: not a Claude session.\n")
	}
	var live *procsnap.Session
	for i := range m.snapshot.Sessions {
		if m.snapshot.Sessions[i].SessionID == sess.ID {
			live = &m.snapshot.Sessions[i]
			break
		}
	}
	if live == nil {
		return styleFooter.Render("Lineage: session has no live process.\n")
	}
	var sb strings.Builder
	sb.WriteString(styleHeader.Render("Lineage"))
	sb.WriteByte('\n')
	sb.WriteString(fmt.Sprintf("  claude (%d)\n", live.PID))
	for _, a := range live.Ancestors {
		sb.WriteString(fmt.Sprintf("  └─ %s (%d)\n", a.Command, a.PID))
	}
	sb.WriteString(styleFooter.Render("(esc to dismiss)\n"))
	return sb.String()
}
```

In `cmd/tui.go`, delete the throwaway `_ = snap / _ = snapOK` lines from Task C1, and replace the `m := tui.New(all, toolModes)` line with:

```go
m := tui.New(all, toolModes)
if snapOK {
	m.SetSnapshot(snap)
}
```

The TUI now has the snapshot it needs to render the lineage overlay. Only one `procsnap.Enumerate()` call happens at startup; C4 adds the refresh tick.

- [ ] **Step 4: Run the tests to verify they pass.**

Run: `go test ./internal/tui/... ./cmd/... -v`
Expected: all green.

- [ ] **Step 5: Commit.**

```bash
git add internal/tui/model.go internal/tui/model_test.go cmd/tui.go
git commit -m "feat(tui): lineage overlay on l key, dismiss on esc"
```

---

### Task C4: 5-second refresh tick

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing test.**

Add to `internal/tui/model_test.go`:

```go
func TestModel_SnapshotTick_UpdatesSnapshot(t *testing.T) {
	sessions := []model.Session{
		{ID: "aaa", Tool: model.ToolClaude, UpdatedAt: time.Now()},
	}
	m := New(sessions, nil)

	// Inject a fake enumerator.
	calls := 0
	m.SetEnumerator(func() (procsnap.Snapshot, error) {
		calls++
		return procsnap.Snapshot{Sessions: []procsnap.Session{{SessionID: "aaa"}}}, nil
	})

	// Simulate the tick message delivery.
	m2, _ := m.Update(snapshotTickMsg{})
	mm := m2.(Model)
	if calls != 1 {
		t.Errorf("expected enumerator to be called once, got %d", calls)
	}
	if !mm.snapshot.IsActive("aaa") {
		t.Error("snapshot must have been stored")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails.**

Run: `go test ./internal/tui/... -run TestModel_SnapshotTick`
Expected: compile error — `SetEnumerator` and `snapshotTickMsg` undefined.

- [ ] **Step 3: Add tick plumbing.**

Append to `internal/tui/model.go`:

```go
// snapshotTickMsg fires every 5 seconds to refresh the process snapshot.
type snapshotTickMsg struct{}

const snapshotTickInterval = 5 * time.Second

// SetEnumerator injects a procsnap enumerator. Callers provide this
// before launching the program so the TUI can refresh.
func (m *Model) SetEnumerator(e func() (procsnap.Snapshot, error)) {
	m.enumerate = e
}
```

Add `enumerate func() (procsnap.Snapshot, error)` to the `Model` struct.

Modify `Init`:

```go
func (m Model) Init() tea.Cmd {
	return tea.Tick(snapshotTickInterval, func(time.Time) tea.Msg {
		return snapshotTickMsg{}
	})
}
```

In `Update`, add a case before the `tea.KeyMsg` case:

```go
case snapshotTickMsg:
	if m.enumerate != nil {
		if snap, err := m.enumerate(); err == nil {
			m.snapshot = snap
			m.sessions = ApplySnapshot(m.sessions, snap)
		}
	}
	return m, tea.Tick(snapshotTickInterval, func(time.Time) tea.Msg {
		return snapshotTickMsg{}
	})
```

In `cmd/tui.go`, attach the enumerator only when the initial snapshot succeeded. If `Enumerate` returned `ErrUnsupported` once, it will keep returning it — no point scheduling refresh ticks. Insert inside the existing `if snapOK` block:

```go
if snapOK {
	m.SetSnapshot(snap)
	m.SetEnumerator(procsnap.Enumerate)
}
```

When `SetEnumerator` is never called, the `Init` tick fires once, `m.enumerate == nil` means the tick case is a no-op but still reschedules. That is harmless but wasteful; guard the reschedule:

In the `Update` case for `snapshotTickMsg`, wrap the reschedule:

```go
case snapshotTickMsg:
	if m.enumerate == nil {
		return m, nil
	}
	if snap, err := m.enumerate(); err == nil {
		m.snapshot = snap
		m.sessions = ApplySnapshot(m.sessions, snap)
	}
	return m, tea.Tick(snapshotTickInterval, func(time.Time) tea.Msg {
		return snapshotTickMsg{}
	})
```

And in `Init` only schedule the first tick if we ever expect to have an enumerator — safest: always schedule (Init is called before `SetEnumerator`; checking there would require ordering guarantees we don't want). The `Update` guard is sufficient.

- [ ] **Step 4: Run the tests to verify they pass.**

Run: `go test ./internal/tui/... -v`
Expected: all green.

- [ ] **Step 5: Manual smoke test.**

Run: `go run . tui` then watch a claude row for ~6 seconds. The `*` Active marker should remain stable (or flip if you quit a claude session in another terminal).

- [ ] **Step 6: Commit.**

```bash
git add internal/tui/model.go internal/tui/model_test.go cmd/tui.go
git commit -m "feat(tui): 5s snapshot refresh tick"
```

---

## Track D — Docs and cleanup (2 tasks)

### Task D1: ARCHITECTURE.md codemap update

**Files:**
- Modify: `ARCHITECTURE.md`

- [ ] **Step 1: Add the `procsnap` package to the codemap.**

Locate the existing codemap block in `ARCHITECTURE.md`. Insert a new line in the appropriate section:

```
internal/procsnap/          Live-process correlation for Claude sessions (darwin-only)
```

Also add `omnisess ps` to any command listing.

- [ ] **Step 2: Verify the doc still renders.**

Run: `grep -c '^#' ARCHITECTURE.md` just to confirm the file wasn't corrupted.

- [ ] **Step 3: Commit.**

```bash
git add ARCHITECTURE.md
git commit -m "docs(architecture): document procsnap package and omnisess ps"
```

---

### Task D2: Exec plan completion + design doc cross-link

**Files:**
- Move: `docs/exec-plans/active/013-active-claude-process-tree.md` → `docs/exec-plans/completed/013-active-claude-process-tree.md`
- Modify: `docs/design-docs/2026-04-20-active-claude-process-tree.md` (status line)

- [ ] **Step 1: Flip the spec status.**

In `docs/design-docs/2026-04-20-active-claude-process-tree.md` change:

```
**Status**: Design — not yet implemented
```

to:

```
**Status**: Implemented
```

- [ ] **Step 2: Move this plan to completed.**

Run:
```bash
git mv docs/exec-plans/active/013-active-claude-process-tree.md docs/exec-plans/completed/013-active-claude-process-tree.md
```

- [ ] **Step 3: Run `make check` one final time.**

Run: `make check`
Expected: fmt + vet + lint + test all green, 100% coverage gate satisfied.

- [ ] **Step 4: Run `make smoke`.**

Run: `make smoke`
Expected: the binary prints help without errors.

- [ ] **Step 5: Manual end-to-end smoke.**

Run these one at a time:
- `./omnisess ps` — shows tree with at least this session.
- `./omnisess ps --json` — emits valid JSON.
- `./omnisess tui` — the `*` active marker reflects real claude liveness; pressing `l` shows lineage; `Esc` dismisses; `q` quits.

- [ ] **Step 6: Commit.**

```bash
git add docs/exec-plans/ docs/design-docs/
git commit -m "docs: mark 013 plan and process-tree spec as implemented"
```

---

## Self-review checklist

Before opening a PR:

- [ ] All tasks above have every checkbox ticked.
- [ ] `make check` passes with 100% per-package coverage (except exempt list).
- [ ] `make smoke` succeeds.
- [ ] The TUI shows accurate Active flags (verified by starting/killing a claude session).
- [ ] `omnisess ps` works on your machine; `omnisess ps --json` parses.
- [ ] On a Linux VM or WSL (if accessible), `omnisess ps` prints the "unsupported" message and exits 0; the TUI still runs with fallback detection. (If no Linux accessible: skip but note in PR description.)
- [ ] Design doc status updated to "Implemented".

## Notes for the implementer

- **Do not** modify `internal/detect/process.go` beyond what is explicitly required. The old heuristic remains the fallback for non-claude tools and for non-macOS platforms.
- **Do not** add a new `go.mod` dependency. Everything uses stdlib, existing bubbletea, and existing lipgloss.
- **Do not** mutate anything under `~/.claude/`. Read-only invariant.
- If plan `010-session-names` is merged before this plan starts, the TUI's `session.Title` fallback chain already includes a `Name()` method. This plan only adds `/rename` name as a new top-priority source; it does not replace `Name()`.
- Injection points (`psRunnerFn`, `killFn`, `sessionsDirFn`) are package-private. Tests manipulate them via `orig := X; X = fake; t.Cleanup(func(){ X = orig })`. This matches the existing pattern in `internal/detect/process.go`.
