//go:build !darwin

package procsnap

import (
	"errors"
	"testing"
)

// TestEnumerate_NonDarwin guarantees Enumerate returns ErrUnsupported on
// non-macOS platforms. Covers procsnap_other.go (only compiled on !darwin).
func TestEnumerate_NonDarwin(t *testing.T) {
	snap, err := Enumerate()
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("non-darwin Enumerate must return ErrUnsupported, got %v", err)
	}
	if len(snap.Sessions) != 0 {
		t.Errorf("non-darwin Enumerate must return empty snapshot, got %d sessions", len(snap.Sessions))
	}
}
