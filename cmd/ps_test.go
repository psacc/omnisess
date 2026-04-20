package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/psacc/omnisess/internal/procsnap"
)

func TestRunPS_Unsupported(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, procsnap.ErrUnsupported
	}, false)
	if err != nil {
		t.Fatalf("unsupported must not error, got %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("unsupported")) {
		t.Errorf("expected friendly notice, got %q", buf.String())
	}
}

// TestRunPS_EnumError ensures non-ErrUnsupported errors propagate.
func TestRunPS_EnumError(t *testing.T) {
	var buf bytes.Buffer
	sentinel := errors.New("boom")
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, sentinel
	}, false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// TestRunPS_EmptySessions exercises the "no live sessions" text branch.
func TestRunPS_EmptySessions(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, nil
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("No live Claude sessions.")) {
		t.Errorf("expected empty-sessions notice, got %q", buf.String())
	}
}

// TestRunPS_TreeHappyPath exercises the non-JSON branch with sessions present
// (calls the B2 renderTree stub, which is a no-op for B1).
func TestRunPS_TreeHappyPath(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{
			Sessions: []procsnap.Session{{PID: 1234, SessionID: "abc"}},
		}, nil
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// renderTree is a stub in B1, so buf is expected to be empty.
}

// TestRunPS_JSONEmpty exercises the JSON branch with an empty snapshot.
func TestRunPS_JSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{}, nil
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded procsnap.Snapshot
	if jerr := json.Unmarshal(buf.Bytes(), &decoded); jerr != nil {
		t.Fatalf("expected valid JSON, got %q (err %v)", buf.String(), jerr)
	}
}

// TestRunPS_JSONWithSessions exercises the JSON branch with a non-empty snapshot.
func TestRunPS_JSONWithSessions(t *testing.T) {
	var buf bytes.Buffer
	err := runPSWith(&buf, func() (procsnap.Snapshot, error) {
		return procsnap.Snapshot{
			Sessions: []procsnap.Session{{PID: 42, SessionID: "xyz"}},
		}, nil
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded procsnap.Snapshot
	if jerr := json.Unmarshal(buf.Bytes(), &decoded); jerr != nil {
		t.Fatalf("expected valid JSON, got %q (err %v)", buf.String(), jerr)
	}
	if len(decoded.Sessions) != 1 || decoded.Sessions[0].PID != 42 {
		t.Errorf("roundtripped snapshot mismatch: %+v", decoded)
	}
}

// TestPSCmd_RunE invokes the psCmd RunE closure via rootCmd so that the
// closure itself (and not just runPSWith) is covered. On darwin Enumerate
// may succeed with zero or more sessions; on other platforms it returns
// ErrUnsupported. Both paths are handled without error.
func TestPSCmd_RunE(t *testing.T) {
	silenceOutput(t)
	resetFlags()
	rootCmd.SetArgs([]string{"ps"})
	defer rootCmd.SetArgs(nil)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd ps returned error: %v", err)
	}
}
