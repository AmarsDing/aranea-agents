package biz

import "context"

// KnowledgeProvider abstracts knowledge access for Team Graph compilation.
type KnowledgeProvider interface {
	// GetKnowledge returns knowledge data for the given team definition.
	GetKnowledge(ctx context.Context, teamID string) (map[string]any, error)
}

// DefaultKnowledgeProvider is the default implementation that returns empty knowledge.
type DefaultKnowledgeProvider struct{}

func NewDefaultKnowledgeProvider() *DefaultKnowledgeProvider {
	return &DefaultKnowledgeProvider{}
}

func (p *DefaultKnowledgeProvider) GetKnowledge(_ context.Context, _ string) (map[string]any, error) {
	return nil, nil
}
