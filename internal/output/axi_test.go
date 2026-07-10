package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
)

// decodeEnvelope renders sessions in axi mode and unmarshals the result.
func decodeEnvelope(t *testing.T, sessions []model.Session) axiEnvelope {
	t.Helper()
	var buf bytes.Buffer
	renderAxiSessions(&buf, sessions)

	// Compact: exactly one line (single trailing newline, no indentation).
	out := buf.String()
	if n := strings.Count(strings.TrimRight(out, "\n"), "\n"); n != 0 {
		t.Fatalf("axi output is not compact: %d embedded newlines\n%s", n, out)
	}

	var env axiEnvelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal axi envelope: %v\noutput: %s", err, out)
	}
	return env
}

func TestAxiStatus(t *testing.T) {
	tests := []struct {
		name string
		sess model.Session
		want string
	}{
		{"live with registry status", model.Session{Active: true, Status: "busy"}, "busy"},
		{"live no status", model.Session{Active: true}, "active"},
		{"not live", model.Session{Active: false, Status: "busy"}, "inactive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := axiStatus(tt.sess); got != tt.want {
				t.Errorf("axiStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderAxiSessions_Envelope(t *testing.T) {
	updated := time.Date(2026, 7, 10, 18, 38, 0, 0, time.Local)
	sessions := []model.Session{
		{
			ID:        "b700c242-5202-4ca2-a4ad-0650c44c0449",
			Tool:      model.ToolClaude,
			Project:   "/Users/x/prj/psacc/firstmate",
			Active:    true,
			Status:    "busy",
			UpdatedAt: updated,
		},
		{
			ID:        "4f7c9002-aaaa-bbbb-cccc-ddddeeeeffff",
			Tool:      model.ToolCodex,
			Project:   "/Users/x/prj/ops-central",
			Active:    true, // live, no registry status → "active"
			UpdatedAt: updated,
		},
		{
			ID:        "deadbeef-0000-1111-2222-333344445555",
			Tool:      model.ToolClaude,
			Project:   "/Users/x/prj/psacc/omnisess",
			Active:    false,
			UpdatedAt: updated,
		},
	}

	env := decodeEnvelope(t, sessions)

	// Minimal schema: exactly 5 fields, full ID preserved, ShortProject applied.
	if len(env.Sessions) != 3 {
		t.Fatalf("got %d sessions, want 3", len(env.Sessions))
	}
	first := env.Sessions[0]
	if first.ID != "b700c242-5202-4ca2-a4ad-0650c44c0449" {
		t.Errorf("ID = %q, want full session ID (not shortened)", first.ID)
	}
	if first.Project != "psacc/firstmate" {
		t.Errorf("Project = %q, want ShortProject %q", first.Project, "psacc/firstmate")
	}
	if first.Status != "busy" {
		t.Errorf("Status = %q, want busy", first.Status)
	}
	if want := updated.Format("2006-01-02T15:04"); first.Updated != want {
		t.Errorf("Updated = %q, want %q", first.Updated, want)
	}

	// Aggregate (axi #4).
	s := env.Summary
	if s.Total != 3 || s.Active != 2 {
		t.Errorf("summary total/active = %d/%d, want 3/2", s.Total, s.Active)
	}
	if s.Status["busy"] != 1 || s.Status["active"] != 1 || s.Status["inactive"] != 1 {
		t.Errorf("status breakdown = %v, want busy:1 active:1 inactive:1", s.Status)
	}
	if s.Tools["claude"] != 2 || s.Tools["codex"] != 1 {
		t.Errorf("tools breakdown = %v, want claude:2 codex:1", s.Tools)
	}

	// Next-step hint (axi #9) uses the FULL qualified ID so `show` resolves.
	if len(env.Next) == 0 || env.Next[0] != "omnisess show claude:b700c242-5202-4ca2-a4ad-0650c44c0449" {
		t.Errorf("next[0] = %v, want fully-qualified show hint", env.Next)
	}
}

func TestRenderAxiSessions_EmptyState(t *testing.T) {
	env := decodeEnvelope(t, nil)

	// Definitive empty state (axi #5): explicit count=0, never ambiguous.
	if env.Summary.Total != 0 || env.Summary.Active != 0 {
		t.Errorf("empty summary = %+v, want total/active 0", env.Summary)
	}
	if env.Sessions == nil {
		// Encoded as [] and decoded back to a non-nil empty slice.
		t.Errorf("sessions decoded to nil, want empty slice")
	}
	if len(env.Sessions) != 0 {
		t.Errorf("sessions = %v, want empty", env.Sessions)
	}
	if len(env.Next) == 0 || !strings.Contains(env.Next[0], "--since") {
		t.Errorf("empty next = %v, want a widen-the-window hint", env.Next)
	}

	// Maps serialize as {} not null: verify in the raw bytes.
	var buf bytes.Buffer
	renderAxiSessions(&buf, nil)
	raw := buf.String()
	if strings.Contains(raw, `"status":null`) || strings.Contains(raw, `"tools":null`) {
		t.Errorf("empty aggregate maps serialized as null, want {}: %s", raw)
	}
}

func TestRenderAxiSessions_NoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	renderAxiSessions(&buf, []model.Session{{ID: "x", Tool: model.ToolClaude}})
	out := buf.String()
	// The search hint contains "<term>": it must render as literal "<term>",
	// not the escaped "<term>" Go's json encoder emits by default.
	if strings.Contains(out, "\\u003c") {
		t.Errorf("output HTML-escaped '<'; want literal '<term>': %s", out)
	}
	if !strings.Contains(out, "<term>") {
		t.Errorf("output missing literal '<term>': %s", out)
	}
}
