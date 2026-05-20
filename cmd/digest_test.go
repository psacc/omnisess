package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

func TestClampDigestConcurrency(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{"below floor (zero)", 0, 1},
		{"below floor (negative)", -3, 1},
		{"at cap", digestFetchConcurrencyCap, digestFetchConcurrencyCap},
		{"above cap", digestFetchConcurrencyCap + 5, digestFetchConcurrencyCap},
		{"mid range", 4, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampDigestConcurrency(tc.n); got != tc.want {
				t.Errorf("clampDigestConcurrency(%d) = %d, want %d", tc.n, got, tc.want)
			}
		})
	}
}

func TestDigestFetchConcurrency_BoundedByCap(t *testing.T) {
	if got := digestFetchConcurrency(); got < 1 {
		t.Errorf("digestFetchConcurrency() = %d, want >= 1", got)
	}
	if got := digestFetchConcurrency(); got > digestFetchConcurrencyCap {
		t.Errorf("digestFetchConcurrency() = %d, want <= %d", got, digestFetchConcurrencyCap)
	}
}

// TestWriteDigest_DeterministicAcrossRuns asserts that the parallel
// per-session Get fan-out inside writeDigest does not introduce ordering
// non-determinism in the rendered markdown. Same input → byte-identical output.
func TestWriteDigest_DeterministicAcrossRuns(t *testing.T) {
	sessions := []model.Session{
		{ID: "digest-sess-1", Tool: digestSrcName, StartedAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)},
		{ID: "digest-sess-2", Tool: digestSrcName, StartedAt: time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)},
	}
	srcMap := map[model.Tool]source.Source{digestSrcName: &digestSrc{}}

	var first strings.Builder
	writeDigest(&first, sessions, srcMap, "2026-05-10")
	want := first.String()

	for i := 0; i < 20; i++ {
		var got strings.Builder
		writeDigest(&got, sessions, srcMap, "2026-05-10")
		if got.String() != want {
			t.Fatalf("iteration %d: digest output diverged from first run\nwant:\n%s\ngot:\n%s",
				i, want, got.String())
		}
	}
}

// slowOKSrc returns nil from Get for sess-2 after a small delay, simulating
// a session whose JSONL is empty or unparseable. The deterministic-render
// loop must still produce sess-1's content in order.
type slowOKSrc struct{}

const slowOKSrcName = model.Tool("test-slow-ok-src")

func (s *slowOKSrc) Name() model.Tool { return slowOKSrcName }
func (s *slowOKSrc) List(_ source.ListOptions) ([]model.Session, error) {
	return []model.Session{
		{ID: "sok-sess-1", Tool: slowOKSrcName},
		{ID: "sok-sess-2", Tool: slowOKSrcName},
		{ID: "sok-sess-3", Tool: slowOKSrcName},
	}, nil
}
func (s *slowOKSrc) Get(id string) (*model.Session, error) {
	switch id {
	case "sok-sess-1":
		return &model.Session{
			ID:   "sok-sess-1",
			Tool: slowOKSrcName,
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "Question one?"},
			},
		}, nil
	case "sok-sess-2":
		// Simulate a failure for the middle session.
		return nil, errors.New("mock get failure")
	case "sok-sess-3":
		return &model.Session{
			ID:   "sok-sess-3",
			Tool: slowOKSrcName,
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "Question three?"},
			},
		}, nil
	}
	return nil, nil
}
func (s *slowOKSrc) Search(_ string, _ source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}

// TestWriteDigest_MidSessionGetErrorDoesNotAbort asserts that one session's
// Get failure (or nil result) does not prevent neighbouring sessions from
// rendering. This is the parallel analogue of the prior sequential
// "continue on err" branch in writeDigest.
func TestWriteDigest_MidSessionGetErrorDoesNotAbort(t *testing.T) {
	sessions := []model.Session{
		{ID: "sok-sess-1", Tool: slowOKSrcName},
		{ID: "sok-sess-2", Tool: slowOKSrcName}, // errors in Get
		{ID: "sok-sess-3", Tool: slowOKSrcName},
	}
	srcMap := map[model.Tool]source.Source{slowOKSrcName: &slowOKSrc{}}

	var buf strings.Builder
	writeDigest(&buf, sessions, srcMap, "2026-05-10")
	out := buf.String()

	if !strings.Contains(out, "Question one?") {
		t.Errorf("expected sess-1 content despite sess-2 error: %q", out)
	}
	if !strings.Contains(out, "Question three?") {
		t.Errorf("expected sess-3 content despite sess-2 error: %q", out)
	}
	// Exactly one separator (between sess-1 and sess-3 only — sess-2 skipped).
	if strings.Count(out, "\n---\n") != 1 {
		t.Errorf("expected exactly 1 separator between the two rendered sessions; got: %q", out)
	}
}
