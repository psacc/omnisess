package codex

import (
	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/source"
)

func init() {
	source.Register(&codexSource{})
}

type codexSource struct{}

func (s *codexSource) Name() model.Tool { return model.ToolCodex }

func (s *codexSource) List(opts source.ListOptions) ([]model.Session, error) {
	// TODO: implement — parse ~/.codex/history.jsonl and session files
	return nil, nil
}

func (s *codexSource) Get(sessionID string) (*model.Session, error) {
	return nil, nil
}

func (s *codexSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}
