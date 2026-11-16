package opencode

import (
	"testing"

	"github.com/psacc/omnisess/internal/model"
)

func TestParseV2Message_User(t *testing.T) {
	msg := parseV2Message("user", `{"time":{"created":1700000001000},"text":"hello"}`)
	if msg == nil {
		t.Fatal("expected message, got nil")
	}
	if msg.Role != model.RoleUser || msg.Content != "hello" {
		t.Errorf("msg = %+v", msg)
	}
}

func TestParseV2Message_Skips(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		data string
	}{
		{"system row", "system", `{"text":"notice"}`},
		{"empty user", "user", `{"text":""}`},
		{"malformed user", "user", `not-json`},
		{"malformed assistant", "assistant", `not-json`},
		{"assistant without renderable content", "assistant", `{"content":[{"type":"reasoning","text":"x"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if msg := parseV2Message(tt.typ, tt.data); msg != nil {
				t.Errorf("expected nil, got %+v", msg)
			}
		})
	}
}

func TestParseV2Message_AssistantWithoutText(t *testing.T) {
	msg := parseV2Message("assistant", `{"content":[{"type":"tool","id":"c1","name":"read","state":{"status":"completed","input":{"a":1},"output":"ok"}}]}`)
	if msg == nil {
		t.Fatal("expected message, got nil")
	}
	if msg.Content != "" {
		t.Errorf("Content = %q, want empty", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Output != "ok" {
		t.Errorf("Output = %q", msg.ToolCalls[0].Output)
	}
}

func TestToolOutput_Empty(t *testing.T) {
	if got := toolOutput(v2ContentBlock{}); got != "" {
		t.Errorf("toolOutput() = %q, want empty", got)
	}
}

func TestParseLegacyMessage(t *testing.T) {
	tests := []struct {
		name string
		data string
		part []legacyPart
		want *model.Message
	}{
		{
			name: "user text",
			data: `{"role":"user","time":{"created":1700000001000}}`,
			part: []legacyPart{{Type: "text", Text: "hi"}},
			want: &model.Message{Role: model.RoleUser, Content: "hi"},
		},
		{
			name: "unknown role",
			data: `{"role":"system"}`,
			part: []legacyPart{{Type: "text", Text: "ignored"}},
			want: nil,
		},
		{
			name: "malformed payload",
			data: `not-json`,
			want: nil,
		},
		{
			name: "no renderable parts",
			data: `{"role":"assistant"}`,
			part: []legacyPart{{Type: "reasoning", Text: "x"}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLegacyMessage(tt.data, tt.part)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("expected nil, got %+v", got)
			case tt.want != nil && got == nil:
				t.Errorf("expected %+v, got nil", tt.want)
			case tt.want != nil && (got.Role != tt.want.Role || got.Content != tt.want.Content):
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestExtractSnippet(t *testing.T) {
	tests := []struct {
		name    string
		content string
		idx     int
		match   int
		target  int
		want    string
	}{
		{"short content unchanged", "hello", 0, 2, 200, "hello"},
		{"centred match", "0123456789012345678901234567890123456789", 20, 5, 10, "...890123456..."},
		{"match near start", "0123456789012345678901234567890123456789", 1, 1, 10, "012345678..."},
		{"match near end", "0123456789012345678901234567890123456789", 37, 1, 10, "...123456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSnippet(tt.content, tt.idx, tt.match, tt.target); got != tt.want {
				t.Errorf("extractSnippet() = %q, want %q", got, tt.want)
			}
		})
	}
}
