package team

import (
	"context"
	"errors"
	"fmt"

	"aranea-agents/pkg/loggateway"

	"aranea-agents/internal/biz"
)

var (
	errMediatorCoordNotSet    = errors.New("mediator coordinator not set")
	errMediatorFinisherNotSet = errors.New("mediator finisher not set")
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
	lg       loggateway.Logger
}

// NewTeamRunMediator creates a mediator with no wiring; call SetCoordinator/SetFinisher after construction.
func NewTeamRunMediator(lg loggateway.Logger) *TeamRunMediator {
	return &TeamRunMediator{lg: lg}
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
		m.lg.Warn("mediator coordinator not set, skipping RegisterTeamGraphExecution",
			loggateway.Str("exec_id", execID),
			loggateway.Str("team_id", teamID))
		return errMediatorCoordNotSet
	}
	return m.coord.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct)
}

func (m *TeamRunMediator) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	if m.coord == nil {
		m.lg.Warn("mediator coordinator not set, skipping MarkTeamGraphInterrupt",
			loggateway.Str("exec_id", execID),
			loggateway.Str("node_id", nodeID))
		return errMediatorCoordNotSet
	}
	return m.coord.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}

func (m *TeamRunMediator) DeferTeamRunSuccessIfHITL(ctx context.Context, graphExecID string, run *biz.TeamRun) (bool, error) {
	if m.coord == nil {
		m.lg.Warn("mediator coordinator not set, skipping DeferTeamRunSuccessIfHITL",
			loggateway.Str("graph_exec_id", graphExecID))
		return false, fmt.Errorf("mediator coordinator not set")
	}
	return m.coord.DeferTeamRunSuccessIfHITL(ctx, graphExecID, run)
}

func (m *TeamRunMediator) StartGraphStepWatch(ctx context.Context, execID string) context.CancelFunc {
	if m.coord == nil {
		m.lg.Warn("mediator coordinator not set, skipping StartGraphStepWatch",
			loggateway.Str("exec_id", execID))
		return func() {}
	}
	return m.coord.StartGraphStepWatch(ctx, execID)
}

// --- TeamGraphRunFinisher delegation ---

func (m *TeamRunMediator) PersistGraphRunStep(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int) {
	if m.finisher == nil {
		m.lg.Warn("mediator finisher not set, skipping PersistGraphRunStep",
			loggateway.Str("node_id", nodeID))
		return
	}
	m.finisher.PersistGraphRunStep(ctx, stepCtx, nodeID, outputPreview, errMsg, skipped, toolCallCount)
}

func (m *TeamRunMediator) FinalizeGraphTeamRun(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string) {
	if m.finisher == nil {
		m.lg.Warn("mediator finisher not set, skipping FinalizeGraphTeamRun",
			loggateway.Bool("failed", failed))
		return
	}
	m.finisher.FinalizeGraphTeamRun(ctx, stepCtx, failed, errMsg)
}

// Compile-time interface assertions.
var (
	_ TeamGraphCoordAccess = (*TeamRunMediator)(nil)
	_ TeamGraphRunFinisher = (*TeamRunMediator)(nil)
	_ TeamGraphCoordAccess = (*TeamGraphRunCoordinator)(nil)
)
