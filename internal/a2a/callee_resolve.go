package a2a

import (
	"context"
	"errors"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	InvokeTargetLocal  = "local"
	InvokeTargetRemote = "remote"
)

// InvokeTarget describes how a callee should be invoked.
type InvokeTarget struct {
	Kind   string
	Local  biz.A2AAgentCard
	Remote biz.A2ARemoteAgent
}

// ResolveInvokeTarget picks local catalog agent or remote registry entry.
func ResolveInvokeTarget(ctx context.Context, uc *biz.A2AUsecase, calleeAgentID string) (InvokeTarget, error) {
	if uc == nil {
		return InvokeTarget{}, kerrors.InternalServer("A2A", "a2a usecase not configured")
	}
	card, err := uc.GetAgentCard(ctx, calleeAgentID)
	if err == nil {
		if card.Enabled {
			return InvokeTarget{Kind: InvokeTargetLocal, Local: card}, nil
		}
		return InvokeTarget{}, kerrors.Forbidden("A2A", "agent is not A2A-enabled")
	}
	if !errors.Is(err, biz.ErrNotFound) {
		return InvokeTarget{}, err
	}
	remote, err := uc.GetRemoteAgent(ctx, calleeAgentID)
	if err != nil {
		if errors.Is(err, biz.ErrNotFound) {
			return InvokeTarget{}, kerrors.NotFound("A2A", "callee agent not found or disabled")
		}
		return InvokeTarget{}, err
	}
	if !remote.Enabled {
		return InvokeTarget{}, kerrors.Forbidden("A2A", "remote agent is disabled")
	}
	return InvokeTarget{Kind: InvokeTargetRemote, Remote: remote}, nil
}
