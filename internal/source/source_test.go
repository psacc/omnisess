package source

import (
	"testing"
	"time"
)

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

func TestMatchesDate(t *testing.T) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	onDate := time.Date(2026, 4, 22, 0, 0, 0, 0, loc)

	tests := []struct {
		name   string
		ts     time.Time
		onDate time.Time
		want   bool
	}{
		{
			name:   "zero onDate matches anything",
			ts:     time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
			onDate: time.Time{},
			want:   true,
		},
		{
			name:   "same day same zone",
			ts:     time.Date(2026, 4, 22, 14, 30, 0, 0, loc),
			onDate: onDate,
			want:   true,
		},
		{
			name:   "same calendar day, different zone, still matches in onDate's zone",
			ts:     time.Date(2026, 4, 22, 22, 0, 0, 0, time.UTC), // 15:00 PDT
			onDate: onDate,
			want:   true,
		},
		{
			name:   "previous day in onDate's zone",
			ts:     time.Date(2026, 4, 21, 14, 30, 0, 0, loc),
			onDate: onDate,
			want:   false,
		},
		{
			name:   "next day in onDate's zone",
			ts:     time.Date(2026, 4, 23, 14, 30, 0, 0, loc),
			onDate: onDate,
			want:   false,
		},
		{
			name:   "UTC instant that lands on previous day in onDate's zone",
			ts:     time.Date(2026, 4, 22, 5, 0, 0, 0, time.UTC), // 22:00 PDT on the 21st
			onDate: onDate,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesDate(tt.ts, tt.onDate)
			if got != tt.want {
				t.Errorf("MatchesDate(%v, %v) = %v, want %v", tt.ts, tt.onDate, got, tt.want)
			}
		})
	}
}
