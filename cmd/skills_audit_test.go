package cmd

import (
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	cases := map[string]time.Duration{
		"30d":  30 * 24 * time.Hour,
		"90d":  90 * 24 * time.Hour,
		"180d": 180 * 24 * time.Hour,
	}
	for in, want := range cases {
		got, err := parseWindow(in)
		if err != nil {
			t.Errorf("parseWindow(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseWindow(%q) = %v want %v", in, got, want)
		}
	}
	if _, err := parseWindow("60d"); err == nil {
		t.Error("expected error for unsupported window")
	}
}
