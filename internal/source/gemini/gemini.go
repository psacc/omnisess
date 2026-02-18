package gemini

import (
	"github.com/psacconier/sessions/internal/model"
	"github.com/psacconier/sessions/internal/source"
)

func init() {
	source.Register(&geminiSource{})
}

type geminiSource struct{}

func (s *geminiSource) Name() model.Tool { return model.ToolGemini }

func (s *geminiSource) List(opts source.ListOptions) ([]model.Session, error) {
	// TODO: implement — parse gemini --list-sessions output
	return nil, nil
}

func (s *geminiSource) Get(sessionID string) (*model.Session, error) {
	return nil, nil
}

func (s *geminiSource) Search(query string, opts source.ListOptions) ([]model.SearchResult, error) {
	return nil, nil
}
