package team

import (
	"context"
	"errors"
	"fmt"

	"aranea-agents/pkg/loggateway"

	"aranea-agents/internal/biz"
)

var (
	errMediatorCoordNotSet = errors.New("mediator coordinator not set")
)

// TeamGraphCoordAccess is the narrow interface Runner needs from the coordinator side.
// It combines execution registration with HITL deferral and step watching,
// breaking the direct dependency on *TeamGraphRunCoordinator.
// Stability:evolving
type TeamGraphCoordAccess interface {
	RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error
	MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error
	DeferTeamRunSuccessIfHITL(ctx context.Context, graphExecID string, run *biz.TeamRun) (bool, error)
	StartGraphStepWatch(ctx context.Context, execID string) context.CancelFunc
}

// TeamRunMediator breaks the circular dependency between Runner and TeamGraphRunCoordinator.
// Runner depends on TeamGraphCoordAccess; Coordinator depends on Mediator's finisher methods.
// TeamRunMediator implements both and delegates to the concrete instances.
type TeamRunMediator struct {
	coord TeamGraphCoordAccess
	lg    loggateway.Logger

	// Finisher functions - set by Runner after construction via SetFinisherFunctions.
	persistGraphRunStepFn    func(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int)
	finalizeGraphTeamRunFn   func(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string)
	publishTeamStepStartedFn func(ctx context.Context, stepCtx *GraphRunStepContext, nodeID string)
}

// NewTeamRunMediator creates a mediator with no wiring; call SetCoordinator/SetFinisherFunctions after construction.
func NewTeamRunMediator(lg loggateway.Logger) *TeamRunMediator {
	return &TeamRunMediator{lg: lg}
}

// SetCoordinator wires the coordinator side (TeamGraphRunCoordinator implements TeamGraphCoordAccess).
// Startup order: SetCoordinator/SetFinisherFunctions must be called before RecoverSessions.
func (m *TeamRunMediator) SetCoordinator(c TeamGraphCoordAccess) {
	m.coord = c
}

// SetFinisherFunctions wires the finisher side using Runner method references.
// Startup order: SetFinisherFunctions/SetCoordinator must be called before RecoverSessions.
func (m *TeamRunMediator) SetFinisherFunctions(
	persistStep func(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int),
	finalizeRun func(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string),
	publishStepStarted func(ctx context.Context, stepCtx *GraphRunStepContext, nodeID string),
) {
	m.persistGraphRunStepFn = persistStep
	m.finalizeGraphTeamRunFn = finalizeRun
	m.publishTeamStepStartedFn = publishStepStarted
}

// --- TeamGraphCoordAccess delegation ---

func (m *TeamRunMediator) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	if m.coord == nil {
		m.lg.Warn("mediator coordinator not set, skipping RegisterTeamGraphExecution",
			loggateway.Str("exec_id", execID),
			loggateway.Str("team_id", teamID))
		return errMediatorCoordNotSet
	}
	return m.coord.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, ct)
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
		return false, fmt.Errorf("%w: DeferTeamRunSuccessIfHITL", errMediatorCoordNotSet)
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

// --- Finisher delegation (via function fields) ---

func (m *TeamRunMediator) PersistGraphRunStep(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int) {
	if m.persistGraphRunStepFn == nil {
		m.lg.Warn("mediator finisher not set, skipping PersistGraphRunStep",
			loggateway.Str("node_id", nodeID))
		return
	}
	m.persistGraphRunStepFn(ctx, stepCtx, nodeID, outputPreview, errMsg, skipped, toolCallCount)
}

func (m *TeamRunMediator) FinalizeGraphTeamRun(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string) {
	if m.finalizeGraphTeamRunFn == nil {
		m.lg.Warn("mediator finisher not set, skipping FinalizeGraphTeamRun",
			loggateway.Bool("failed", failed))
		return
	}
	m.finalizeGraphTeamRunFn(ctx, stepCtx, failed, errMsg)
}

func (m *TeamRunMediator) PublishTeamStepStarted(ctx context.Context, stepCtx *GraphRunStepContext, nodeID string) {
	if m.publishTeamStepStartedFn == nil {
		m.lg.Warn("mediator finisher not set, skipping PublishTeamStepStarted",
			loggateway.Str("node_id", nodeID))
		return
	}
	m.publishTeamStepStartedFn(ctx, stepCtx, nodeID)
}

// Compile-time interface assertions.
var (
	_ TeamGraphCoordAccess = (*TeamRunMediator)(nil)
	_ TeamGraphCoordAccess = (*TeamGraphRunCoordinator)(nil)
)
