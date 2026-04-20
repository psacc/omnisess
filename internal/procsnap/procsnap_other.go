//go:build !darwin

package procsnap

// Enumerate returns ErrUnsupported on non-macOS platforms. Callers fall back
// to their previous behavior.
func Enumerate() (Snapshot, error) {
	return Snapshot{}, ErrUnsupported
}
