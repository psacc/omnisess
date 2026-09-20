//go:build darwin

package procsnap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPSRunnerFnDefault(t *testing.T) {
	raw, err := psRunnerFn()
	if err != nil {
		t.Fatalf("psRunnerFn: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty ps output")
	}
	procs, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS on real ps output: %v", err)
	}
	if _, ok := procs[1]; !ok {
		t.Error("expected PID 1 (launchd) in real ps output")
	}
	self, ok := procs[os.Getpid()]
	if !ok {
		t.Fatal("expected current test process in real ps output")
	}
	if self.Command != filepath.Base(os.Args[0]) {
		t.Errorf("current process command = %q, want %q", self.Command, filepath.Base(os.Args[0]))
	}
}
