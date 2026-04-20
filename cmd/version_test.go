package cmd

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveVersionFromInfo(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{
			name: "ok false",
			ok:   false,
			want: "(devel)",
		},
		{
			name: "nil info",
			info: nil,
			ok:   true,
			want: "(devel)",
		},
		{
			name: "empty version",
			info: &debug.BuildInfo{Main: debug.Module{Version: ""}},
			ok:   true,
			want: "(devel)",
		},
		{
			name: "literal (devel)",
			info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			ok:   true,
			want: "(devel)",
		},
		{
			name: "real semver",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v0.4.1"}},
			ok:   true,
			want: "v0.4.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersionFromInfo(tt.info, tt.ok); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	// Runs via debug.ReadBuildInfo(), which in tests returns "(devel)" —
	// assert it's non-empty so the wrapper is exercised.
	if got := resolveVersion(); got == "" {
		t.Error("resolveVersion returned empty string")
	}
}

func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.Run(versionCmd, nil)
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Error("version command produced no output")
	}
}
