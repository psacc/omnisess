package cmd

import (
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
