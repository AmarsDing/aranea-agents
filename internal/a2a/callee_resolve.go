package a2a

import (
	"context"
	"errors"

	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

const (
	InvokeTargetLocal  = "local"
	InvokeTargetRemote = "remote"
)

// InvokeTarget describes how a callee should be invoked.
type InvokeTarget struct {
	Kind   string
	Local  a2abiz.AgentCard
	Remote a2abiz.RemoteAgent
}

// ResolveInvokeTarget picks local catalog agent or remote registry entry.
func ResolveInvokeTarget(ctx context.Context, uc *a2abiz.Usecase, calleeAgentID string) (InvokeTarget, error) {
	if uc == nil {
		return InvokeTarget{}, apierror.Internal(apierror.DomainA2A, "a2a usecase not configured")
	}
	card, err := uc.GetAgentCard(ctx, calleeAgentID)
	if err == nil {
		if card.Enabled {
			return InvokeTarget{Kind: InvokeTargetLocal, Local: card}, nil
		}
		return InvokeTarget{}, apierror.Forbidden(apierror.DomainA2A, "agent is not A2A-enabled")
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return InvokeTarget{}, err
	}
	remote, err := uc.GetRemoteAgent(ctx, calleeAgentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return InvokeTarget{}, apierror.NotFound(apierror.DomainA2A, "callee agent not found or disabled")
		}
		return InvokeTarget{}, err
	}
	if !remote.Enabled {
		return InvokeTarget{}, apierror.Forbidden(apierror.DomainA2A, "remote agent is disabled")
	}
	return InvokeTarget{Kind: InvokeTargetRemote, Remote: remote}, nil
}
