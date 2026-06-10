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
	other := errors.New("unrelated")
	if errors.Is(ErrUnsupported, other) {
		t.Error("ErrUnsupported must not match an unrelated error; callers rely on sentinel equality")
	}
}

func TestSnapshot_Lookup(t *testing.T) {
	s := Snapshot{Sessions: []Session{{SessionID: "abc", Status: "busy"}, {SessionID: "def"}}}
	live, ok := s.Lookup("abc")
	if !ok || live.Status != "busy" {
		t.Errorf("Lookup(abc) = (%+v, %v), want status busy", live, ok)
	}
	if _, ok := s.Lookup("nope"); ok {
		t.Error("Lookup(nope) must not match")
	}
}

// TestCached_MemoizesSnapshotAndError verifies the once-semantics of the
// shared snapshot: one enumeration per process lifetime, with both the
// snapshot and the error memoized. enumerateFn is injected so the test is
// deterministic on every platform and never touches the real process table.
func TestCached_MemoizesSnapshotAndError(t *testing.T) {
	orig := enumerateFn
	t.Cleanup(func() {
		enumerateFn = orig
		resetCache()
	})

	calls := 0
	enumerateFn = func() (Snapshot, error) {
		calls++
		return Snapshot{Sessions: []Session{{Tool: ToolClaude, SessionID: "abc", Status: "idle"}}}, nil
	}
	resetCache()

	first, err := Cached()
	if err != nil || len(first.Sessions) != 1 {
		t.Fatalf("Cached() = (%d sessions, %v), want 1 session, nil", len(first.Sessions), err)
	}
	second, err := Cached()
	if err != nil || len(second.Sessions) != 1 {
		t.Fatalf("second Cached() = (%d sessions, %v)", len(second.Sessions), err)
	}
	if calls != 1 {
		t.Errorf("enumerateFn ran %d times, want 1 (memoized)", calls)
	}

	// Errors are memoized too: a failed enumeration is not retried within
	// the same run.
	enumerateFn = func() (Snapshot, error) {
		calls++
		return Snapshot{}, errors.New("ps exploded")
	}
	resetCache()
	if _, err := Cached(); err == nil {
		t.Fatal("expected memoized error")
	}
	if _, err := Cached(); err == nil {
		t.Fatal("expected memoized error on second call")
	}
	if calls != 2 {
		t.Errorf("enumerateFn ran %d times total, want 2", calls)
	}
}
