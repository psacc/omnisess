package cmd

import (
	"os"
	"slices"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:  "standard Go duration - hours",
			input: "5h",
			want:  5 * time.Hour,
		},
		{
			name:  "standard Go duration - minutes",
			input: "30m",
			want:  30 * time.Minute,
		},
		{
			name:  "standard Go duration - mixed",
			input: "1h30m",
			want:  90 * time.Minute,
		},
		{
			name:  "days",
			input: "7d",
			want:  7 * 24 * time.Hour,
		},
		{
			name:  "one day",
			input: "1d",
			want:  24 * time.Hour,
		},
		{
			name:  "weeks",
			input: "2w",
			want:  14 * 24 * time.Hour,
		},
		{
			name:  "one week",
			input: "1w",
			want:  7 * 24 * time.Hour,
		},
		{
			name:    "invalid string",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "just letters",
			input:   "abc",
			wantErr: true,
		},
		{
			name:  "zero days",
			input: "0d",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDuration(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetListOptions_ExcludeProjects(t *testing.T) {
	// Save and restore global flag state.
	origExclude := flagExcludeProjects
	origSince := flagSince
	t.Cleanup(func() {
		flagExcludeProjects = origExclude
		flagSince = origSince
	})
	flagSince = ""

	t.Run("env var only", func(t *testing.T) {
		flagExcludeProjects = nil
		t.Setenv("OMNISESS_EXCLUDE_PROJECTS", "CodexBar,TestApp")
		opts := getListOptions()
		want := []string{"CodexBar", "TestApp"}
		if !slices.Equal(opts.ExcludeProjects, want) {
			t.Errorf("got %v, want %v", opts.ExcludeProjects, want)
		}
	})

	t.Run("flag only", func(t *testing.T) {
		flagExcludeProjects = []string{"FooProject"}
		os.Unsetenv("OMNISESS_EXCLUDE_PROJECTS")
		opts := getListOptions()
		want := []string{"FooProject"}
		if !slices.Equal(opts.ExcludeProjects, want) {
			t.Errorf("got %v, want %v", opts.ExcludeProjects, want)
		}
	})

	t.Run("flag + env var merged", func(t *testing.T) {
		flagExcludeProjects = []string{"FromFlag"}
		t.Setenv("OMNISESS_EXCLUDE_PROJECTS", "FromEnv")
		opts := getListOptions()
		want := []string{"FromFlag", "FromEnv"}
		if !slices.Equal(opts.ExcludeProjects, want) {
			t.Errorf("got %v, want %v", opts.ExcludeProjects, want)
		}
	})

	t.Run("neither set", func(t *testing.T) {
		flagExcludeProjects = nil
		os.Unsetenv("OMNISESS_EXCLUDE_PROJECTS")
		opts := getListOptions()
		if len(opts.ExcludeProjects) != 0 {
			t.Errorf("expected empty, got %v", opts.ExcludeProjects)
		}
	})
}
