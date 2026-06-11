package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
	tooltrpc "aranea-agents/internal/tools/trpc"
)

// teamRunnerWireAdapter adapts *team.Runner to biz.TeamRunnerWirePort.
// This breaks the direct dependency of service layer on the concrete team.Runner type.
type teamRunnerWireAdapter struct {
	inner *team.Runner
}

// ProvideTeamRunnerWirePort wraps a concrete *team.Runner as a biz.TeamRunnerWirePort.
func ProvideTeamRunnerWirePort(r *team.Runner) biz.TeamRunnerWirePort {
	if r == nil {
		return nil
	}
	return &teamRunnerWireAdapter{inner: r}
}

func (a *teamRunnerWireAdapter) RunTurnFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	return a.inner.RunTurnFromInput(ctx, sess, input)
}

func (a *teamRunnerWireAdapter) SetMediator(mediator biz.TeamMediatorPort) {
	if a.inner == nil || mediator == nil {
		return
	}
	// The mediator port is backed by *team.TeamRunMediator; extract it.
	if tm, ok := mediator.(*teamMediatorAdapter); ok {
		a.inner.SetMediator(tm.inner)
	}
}

func (a *teamRunnerWireAdapter) SetAwaitHookProvider(fn biz.AwaitHookProvider) {
	if a.inner == nil || fn == nil {
		return
	}
	// Bridge biz.AwaitHookProvider to the team.Runner's expected signature.
	a.inner.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc {
		bizFn := fn(runCtx, sessionID, runID)
		return func(ctx context.Context) (string, error) {
			return bizFn(ctx)
		}
	})
}

// teamMediatorAdapter adapts *team.TeamRunMediator to biz.TeamMediatorPort.
type teamMediatorAdapter struct {
	inner *team.TeamRunMediator
}

// ProvideTeamMediatorPort wraps a concrete *team.TeamRunMediator as a biz.TeamMediatorPort.
func ProvideTeamMediatorPort(m *team.TeamRunMediator) biz.TeamMediatorPort {
	if m == nil {
		return nil
	}
	return &teamMediatorAdapter{inner: m}
}

func (a *teamMediatorAdapter) SetFinisher(finisher biz.TeamGraphRunFinisherPort) {
	if a.inner == nil || finisher == nil {
		return
	}
	// The finisher port is backed by *teamRunnerWireAdapter; extract the inner Runner.
	if ra, ok := finisher.(*teamRunnerWireAdapter); ok {
		a.inner.SetFinisher(ra.inner)
	}
}

// teamGraphCoordAdapter adapts *team.TeamGraphRunCoordinator to biz.TeamGraphCoordPort.
type teamGraphCoordAdapter struct {
	inner *team.TeamGraphRunCoordinator
}

// ProvideTeamGraphCoordPort wraps a concrete *team.TeamGraphRunCoordinator as a biz.TeamGraphCoordPort.
func ProvideTeamGraphCoordPort(c *team.TeamGraphRunCoordinator) biz.TeamGraphCoordPort {
	if c == nil {
		return nil
	}
	return &teamGraphCoordAdapter{inner: c}
}

func (a *teamGraphCoordAdapter) SetFinisher(finisher biz.TeamMediatorPort) {
	if a.inner == nil || finisher == nil {
		return
	}
	if ma, ok := finisher.(*teamMediatorAdapter); ok {
		a.inner.SetFinisher(ma.inner)
	}
}

func (a *teamGraphCoordAdapter) RecoverSessions(ctx context.Context) {
	a.inner.RecoverSessions(ctx)
}
