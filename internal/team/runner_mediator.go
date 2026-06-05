package team

import (
	"context"

	"aranea-agents/internal/biz"
)

// TeamGraphCoordAccess is the narrow interface Runner needs from the coordinator side.
// It combines execution registration with HITL deferral and step watching,
// breaking the direct dependency on *TeamGraphRunCoordinator.
type TeamGraphCoordAccess interface {
	TeamGraphExecutionRegistry
	DeferTeamRunSuccessIfHITL(ctx context.Context, graphExecID string, run *biz.TeamRun) (bool, error)
	StartGraphStepWatch(ctx context.Context, execID string) context.CancelFunc
}

// TeamRunMediator breaks the circular dependency between Runner and TeamGraphRunCoordinator.
// Runner depends on TeamGraphCoordAccess; Coordinator depends on TeamGraphRunFinisher.
// TeamRunMediator implements both and delegates to the concrete instances.
type TeamRunMediator struct {
	coord    TeamGraphCoordAccess
	finisher TeamGraphRunFinisher
}

// NewTeamRunMediator creates a mediator with no wiring; call SetCoordinator/SetFinisher after construction.
func NewTeamRunMediator() *TeamRunMediator {
	return &TeamRunMediator{}
}

// SetCoordinator wires the coordinator side (TeamGraphRunCoordinator implements TeamGraphCoordAccess).
// Startup order: SetCoordinator/SetFinisher must be called before RecoverSessions.
func (m *TeamRunMediator) SetCoordinator(c TeamGraphCoordAccess) {
	m.coord = c
}

// SetFinisher wires the finisher side (Runner implements TeamGraphRunFinisher).
// Startup order: SetFinisher/SetCoordinator must be called before RecoverSessions.
func (m *TeamRunMediator) SetFinisher(f TeamGraphRunFinisher) {
	m.finisher = f
}

// --- TeamGraphCoordAccess delegation ---

func (m *TeamRunMediator) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	if m.coord == nil {
		return nil
	}
	return m.coord.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct)
}

func (m *TeamRunMediator) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	if m.coord == nil {
		return nil
	}
	return m.coord.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}

func (m *TeamRunMediator) DeferTeamRunSuccessIfHITL(ctx context.Context, graphExecID string, run *biz.TeamRun) (bool, error) {
	if m.coord == nil {
		return false, nil
	}
	return m.coord.DeferTeamRunSuccessIfHITL(ctx, graphExecID, run)
}

func (m *TeamRunMediator) StartGraphStepWatch(ctx context.Context, execID string) context.CancelFunc {
	if m.coord == nil {
		return func() {}
	}
	return m.coord.StartGraphStepWatch(ctx, execID)
}

// --- TeamGraphRunFinisher delegation ---

func (m *TeamRunMediator) PersistGraphRunStep(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int) {
	if m.finisher == nil {
		return
	}
	m.finisher.PersistGraphRunStep(ctx, stepCtx, nodeID, outputPreview, errMsg, skipped, toolCallCount)
}

func (m *TeamRunMediator) FinalizeGraphTeamRun(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string) {
	if m.finisher == nil {
		return
	}
	m.finisher.FinalizeGraphTeamRun(ctx, stepCtx, failed, errMsg)
}

// Compile-time interface assertions.
var (
	_ TeamGraphCoordAccess  = (*TeamRunMediator)(nil)
	_ TeamGraphRunFinisher  = (*TeamRunMediator)(nil)
	_ TeamGraphCoordAccess  = (*TeamGraphRunCoordinator)(nil)
)
