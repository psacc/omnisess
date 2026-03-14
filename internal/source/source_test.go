package source

import "testing"

func TestMatchesExclude(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		excludes []string
		want     bool
	}{
		{
			name:     "nil excludes",
			project:  "/Users/foo/bar",
			excludes: nil,
			want:     false,
		},
		{
			name:     "empty excludes",
			project:  "/Users/foo/bar",
			excludes: []string{},
			want:     false,
		},
		{
			name:     "no match",
			project:  "/Users/foo/bar",
			excludes: []string{"baz"},
			want:     false,
		},
		{
			name:     "exact match",
			project:  "CodexBar/ClaudeProbe",
			excludes: []string{"CodexBar/ClaudeProbe"},
			want:     true,
		},
		{
			name:     "substring match",
			project:  "/Applications/CodexBar.app/ClaudeProbe",
			excludes: []string{"CodexBar"},
			want:     true,
		},
		{
			name:     "multiple excludes first matches",
			project:  "/Users/foo/bar",
			excludes: []string{"foo", "baz"},
			want:     true,
		},
		{
			name:     "multiple excludes second matches",
			project:  "/Users/foo/bar",
			excludes: []string{"baz", "bar"},
			want:     true,
		},
		{
			name:     "empty project with excludes",
			project:  "",
			excludes: []string{"foo"},
			want:     false,
		},
		{
			name:     "empty string in excludes is ignored",
			project:  "/Users/foo/bar",
			excludes: []string{""},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesExclude(tt.project, tt.excludes)
			if got != tt.want {
				t.Errorf("MatchesExclude(%q, %v) = %v, want %v", tt.project, tt.excludes, got, tt.want)
			}
		})
	}
}
