package index

import (
	"github.com/psacc/omnisess/internal/model"
)

// SessionFromModel converts a unified model.Session (as returned by the
// Claude source `Get`) into the index-shaped Session. The fields on
// model.ToolCall — populated by the Claude parser — carry all the
// data the index needs; index does not import any source package directly.
//
// `providerName` is the OTel gen_ai.provider.name value ('anthropic' for Claude).
func SessionFromModel(s *model.Session, providerName string) *Session {
	if s == nil {
		return nil
	}
	out := &Session{
		ConversationID: s.ID,
		ProviderName:   providerName,
		RequestModel:   s.Model,
		ResponseModel:  s.Model,
		StartedAt:      s.StartedAt,
		UpdatedAt:      s.UpdatedAt,
	}
	for _, m := range s.Messages {
		if m.Role == model.RoleAssistant {
			out.TotalInputTokens += m.UsageInputTokens
			out.TotalOutputTokens += m.UsageOutputTokens
			out.TotalCacheCreateTokens += m.UsageCacheCreationInputTokens
			out.TotalCacheReadTokens += m.UsageCacheReadInputTokens
		}
		for _, tc := range m.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCallRow{
				ToolCallID:       tc.ID,
				ToolName:         tc.Name,
				ToolType:         ToolType(tc.Name),
				OperationName:    "execute_tool",
				IsError:          tc.IsError,
				Timestamp:        m.Timestamp,
				FilePath:         tc.FilePath,
				FileOp:           tc.FileOp,
				FileLinesAdded:   tc.FileLinesAdded,
				FileLinesRemoved: tc.FileLinesRemoved,
				FileContentSize:  tc.FileContentSize,
				ArgumentsJSON:    tc.Input,
				ResultJSON:       tc.Output,
			})
		}
	}
	return out
}
