package a2a

import (
	"context"
	"strings"
)

// AgentLookupAdapter adapts a broader agent reader to the AgentLookup interface.
// This keeps the a2a package free from importing the full Agent type.
type AgentLookupAdapter struct {
	getAgent func(ctx context.Context, id string) (displayName, workspace string, err error)
}

// NewAgentLookupAdapter creates an AgentLookup from a function.
func NewAgentLookupAdapter(fn func(ctx context.Context, id string) (displayName, workspace string, err error)) *AgentLookupAdapter {
	return &AgentLookupAdapter{getAgent: fn}
}

func (a *AgentLookupAdapter) GetAgentByID(ctx context.Context, id string) (AgentMeta, error) {
	if a == nil || a.getAgent == nil {
		return AgentMeta{}, nil
	}
	dn, ws, err := a.getAgent(ctx, id)
	if err != nil {
		return AgentMeta{}, err
	}
	return AgentMeta{
		DisplayName: strings.TrimSpace(dn),
		Workspace:   strings.TrimSpace(ws),
	}, nil
}

var _ AgentLookup = (*AgentLookupAdapter)(nil)
