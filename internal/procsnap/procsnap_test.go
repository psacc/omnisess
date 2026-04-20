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
