package biz

import "context"

// PluginProvider abstracts plugin access for Team Graph compilation.
type PluginProvider interface {
	// GetPlugins returns plugin configurations for the given team definition.
	GetPlugins(ctx context.Context, teamID string) ([]map[string]any, error)
}

// DefaultPluginProvider is the default implementation that returns no plugins.
type DefaultPluginProvider struct{}

func NewDefaultPluginProvider() *DefaultPluginProvider {
	return &DefaultPluginProvider{}
}

func (p *DefaultPluginProvider) GetPlugins(_ context.Context, _ string) ([]map[string]any, error) {
	return nil, nil
}
