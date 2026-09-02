package biz

import (
	"context"
	"time"
)

// TeamTurnRuntime is the biz-level port for executing a team turn.
// Consumers (ChatOrchestrator, Channel) depend on this instead of
// reaching into team.Runner internals.
//
// Implementations live in internal/team. Wire binding in internal/service.
// Stability:evolving
type TeamTurnRuntime interface {
	// ExecuteTurn runs a single team turn and returns the result.
	ExecuteTurn(ctx context.Context, input TeamTurnInput) (TeamTurnResult, error)

	// CancelTurn cancels an in-flight team turn.
	CancelTurn(ctx context.Context, sessionID string) bool
}

// TeamTurnInput is the transport-neutral input for a team turn.
type TeamTurnInput struct {
	SessionID string
	TeamID    string
	Content   string
	AgentKey  string
}

// TeamTurnResult is the return type for TeamTurnRuntime.ExecuteTurn.
type TeamTurnResult struct {
	Outcome      TurnOutcome
	UserMsg      ChatMessage
	AssistantMsg ChatMessage
	TeamRunID    string
}

// ---------------------------------------------------------------------------
// Service-layer dependency ports
// ---------------------------------------------------------------------------
// These interfaces break the dependency inversion violation where
// internal/service imported concrete types from internal/team and
// internal/runtime. Service layer must depend only on biz interfaces.

// AwaitReplyFunc is the biz-level signature for the await-hook callback.
// It mirrors tools/serviceawaitreply.ReplyFunc without importing that package.
type AwaitReplyFunc func(ctx context.Context) (reply string, err error)

// AwaitHookProvider creates an AwaitReplyFunc for a given session/run pair.
type AwaitHookProvider func(runCtx context.Context, sessionID, runID string) AwaitReplyFunc

// TeamMediatorPort is the biz-level port for the team run mediator.
// It abstracts the circular-dependency-breaking mediator between Runner
// and TeamGraphRunCoordinator, without importing internal/team types.
//
// Stability:evolving
type TeamMediatorPort interface {
	// SetFinisher wires the finisher that persists graph run steps and
	// finalizes team runs. The finisher is typically *team.Runner.
	SetFinisher(finisher TeamGraphRunFinisherPort)

	// SetCoordinator wires the coordinator side so the mediator can forward
	// RegisterTeamGraphExecution / MarkTeamGraphInterrupt / HITL deferral and
	// step-watch calls. Construction order per team/runner.go:
	// Runner → Mediator → Coordinator → Mediator.SetCoordinator.
	SetCoordinator(coord TeamGraphCoordPort)
}

// TeamGraphRunFinisherPort is the biz-level port for the graph run finisher.
// It abstracts the step persistence and team run finalization logic that
// team.Runner provides to the coordinator via the mediator.
//
// Stability:evolving
type TeamGraphRunFinisherPort interface {
	// SetMediator wires the mediator that breaks the circular dependency
	// between Runner and TeamGraphRunCoordinator.
	SetMediator(mediator TeamMediatorPort)

	// SetAwaitHookProvider wires the await-hook callback factory.
	SetAwaitHookProvider(fn AwaitHookProvider)

	// SetDeliverableGate wires the real-deliverable gate that vetoes
	// run-success finalization for DAG teams with no real deliverable
	// (2026-07-28 修复3). Backed by SpiritTeamController.HasRealDeliverable.
	SetDeliverableGate(fn TeamDeliverableGateFunc)

	// SetUpstreamDeliverableSeed wires the cross-team deliverable seed
	// resolver (2026-08-08 问题3c): at DAG downstream team turn start, the
	// runner injects the seed into the graph initial state's "deliverable"
	// field, so members read upstream topics directly via get_deliverable
	// instead of failing on an isolated per-execution state. Backed by
	// SpiritTeamUsecase.UpstreamDeliverableSeed. Nil keeps legacy behavior
	// (no seed; members fall back to read_upstream_deliverable / digests).
	SetUpstreamDeliverableSeed(fn TeamUpstreamSeedFunc)

	// SetQualityGate wires the deliverable CONTENT quality gate consulted
	// after the binary gate passes (G3/ADR-G 2026-08-14): verdict
	// pass/revise/fail with a bounded revision loop (maxQualityRevisions).
	// Backed by SpiritTeamUsecase.EvaluateDeliverableQuality. Nil keeps
	// binary-only behavior.
	SetQualityGate(fn TeamQualityGateFunc)

	// SetRevisionEnqueuer wires the revision followup channel that delivers
	// judge feedback to the team session (P2-3 ChatEnqueueKindFollowup
	// roadbed: consumed as a new turn after the current turn ends). Nil
	// degrades revise verdicts to fail-open pass.
	SetRevisionEnqueuer(fn TeamRevisionEnqueuerFunc)

	// SetNoProgressEnqueuer wires the no-progress correction-note channel
	// (79-runtime-governance R5): the auditor's nudge is enqueued as a
	// followup so members read it in the next turn's history. Nil degrades
	// to log-only (counting/cancel still work).
	SetNoProgressEnqueuer(fn TeamRevisionEnqueuerFunc)
}

// TeamDeliverableGateFunc reports whether the team produced a real
// deliverable via set_deliverable. Mirrors SpiritTeamController.HasRealDeliverable.
type TeamDeliverableGateFunc func(ctx context.Context, team Team) (bool, error)

// TeamUpstreamSeedFunc resolves the upstream deliverable seed for a DAG
// downstream team (business topics only; summary/cognition/ack excluded).
// Returns (nil, nil) when the team has no dependencies or no completed
// upstream deliverables. Mirrors SpiritTeamUsecase.UpstreamDeliverableSeed.
type TeamUpstreamSeedFunc func(ctx context.Context, team Team) (map[string]any, error)

// TeamQualityGateFunc evaluates deliverable content quality for a DAG team
// (G3/ADR-G). Mirrors SpiritTeamUsecase.EvaluateDeliverableQuality.
type TeamQualityGateFunc func(ctx context.Context, team Team) (QualityGateResult, error)

// TeamRevisionEnqueuerFunc enqueues a quality-gate revision followup to the
// team session (P2-3 followup semantics: consumed as a new turn after the
// current turn ends).
type TeamRevisionEnqueuerFunc func(ctx context.Context, sessionID, content string) error

// StaleRunHandlerFunc is invoked when the coordinator's reconciler
// force-fails a lost running team run (P0 终态一致性). The service layer
// forwards the failure to PlanExecutor.NotifyTeamCompletion so a blocked DAG
// step unblocks (cascade-fails) instead of waiting forever.
type StaleRunHandlerFunc func(teamID, teamRunID, reason string)

// TeamGraphCoordPort is the biz-level port for the team graph run coordinator.
// It abstracts the coordinator that manages graph execution steps.
//
// Stability:evolving
type TeamGraphCoordPort interface {
	// SetFinisher wires the finisher (typically the mediator) to the coordinator.
	SetFinisher(finisher TeamMediatorPort)

	// SetStaleRunHandler wires the callback invoked when ReconcileStaleRuns
	// reaps a lost running team run.
	SetStaleRunHandler(h StaleRunHandlerFunc)

	// RecoverSessions recovers in-flight graph executions after restart.
	RecoverSessions(ctx context.Context)
}

// TeamRunStartupResumeMarker reports whether a running team run was already
// crash-resumed from its graph checkpoint during this process's startup
// reconcile (83-长时运行韧性). Implemented by the team graph coordinator;
// consumed by TeamUsecase.RecoverOrphanedRunningTeamsEx to skip killing runs
// that are alive again. The marker set lives only for the startup window.
// Stability:evolving
type TeamRunStartupResumeMarker interface {
	WasStartupResumed(runID string) bool
}

// GraphExecFinalizer converges a graph_executions row to a terminal state
// when biz-layer orphan recovery kills a team run (83-长时运行韧性 §4.2):
// without it the graph_executions row would stay running forever (team 层
// finalize 不经手判死路径)。Implemented by *GraphUsecase
// (FinalizeTeamGraphExecution); optional — nil skips convergence (单测/
// 离线工具)。
// Stability:evolving
type GraphExecFinalizer interface {
	FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error
}

// TeamTurnRunnerPort is the biz-level port for the team turn runner.
// TeamService depends on this instead of *team.Runner.
//
// Stability:evolving
type TeamTurnRunnerPort interface {
	// RunTurnFromInput executes one user turn for a team session.
	RunTurnFromInput(ctx context.Context, sess Session, input TurnInput) (ChatMessage, ChatMessage, error)
}

// TeamRunnerWirePort combines TeamTurnRunnerPort with runtime wiring methods.
// ChatOrchestrator depends on this instead of *team.Runner, breaking the
// direct dependency on the concrete team.Runner type.
//
// Stability:evolving
type TeamRunnerWirePort interface {
	TeamTurnRunnerPort
	TeamGraphRunFinisherPort
}

// RunRegistryPort is the biz-level port for the runtime run registry.
// TeamService depends on this instead of *rt.RunRegistry.
//
// Stability:evolving
type RunRegistryPort interface {
	// Cancel cancels the active run for the given session.
	Cancel(sessionID, reason string) (bool, string)

	// GetStatus returns the current status entry for the given session.
	GetStatus(sessionID string) (RunStatusEntry, bool)

	// EnqueueUserMessage enqueues a user message into the active run's pending
	// queue. Returns (accepted=false, err=nil) if no active run exists or the
	// runner does not support queued messages.
	// Stability:evolving
	EnqueueUserMessage(sessionID, content string) (bool, error)
}

// RunStatusEntry is the biz-level representation of a runtime run status.
// It mirrors rt.RunStatusEntry without importing internal/runtime.
type RunStatusEntry struct {
	RunID     string
	Status    string
	ErrMsg    string
	UpdatedAt time.Time
}

// TeamRunObserver is the biz-level port for observing team run lifecycle events.
// Implementations receive notifications when team runs start, step, and finish.
//
// Implementations live in internal/service (WS projection, metrics).
// Wire binding in internal/service.
// Stability:evolving
type TeamRunObserver interface {
	// OnTeamRunStarted is called when a team run begins.
	OnTeamRunStarted(ctx context.Context, run TeamRunRecord)

	// OnTeamRunStepFinished is called when a single step completes.
	OnTeamRunStepFinished(ctx context.Context, step TeamRunStep)

	// OnTeamRunFinished is called when a team run completes (success or failure).
	OnTeamRunFinished(ctx context.Context, run TeamRunRecord)
}

// ---------------------------------------------------------------------------
// Team Turn Lifecycle Hooks
// ---------------------------------------------------------------------------
// These interfaces allow the TurnExecutor to delegate Team-specific behavior
// without knowing the concrete team runtime implementation.

// TeamBuildRunner constructs the runtime for a single team turn.
// This is the "build" hook in the TurnExecutor lifecycle for team sessions.
// Stability:evolving
type TeamBuildRunner interface {
	// BuildTeamRunner constructs a callable team runner for the given session.
	BuildTeamRunner(ctx context.Context, session Session, input TurnInput) (TeamRunnerHandle, error)
}

// TeamRunnerHandle is the biz-level handle for a built team runner.
// Stability:evolving
type TeamRunnerHandle interface {
	// Run executes the team turn and returns the result.
	Run(ctx context.Context) (TeamTurnOutput, error)
	// Close releases any resources held by the runner.
	Close()
}

// TeamTurnOutput carries the result of a single team turn execution.
type TeamTurnOutput struct {
	ContentMarkdown string
	TokenIn         int
	TokenOut        int
	TeamRunID       string
}

// TeamPersistTurnRecord persists the turn result for a team session.
// This is the "persist" hook in the TurnExecutor lifecycle for team sessions.
type TeamPersistTurnRecord interface {
	PersistTeamTurn(ctx context.Context, sessionID, teamID string, userMsg, assistantMsg ChatMessage, tokenIn, tokenOut int) error
}

// TeamProjectRuntimeEvent emits runtime events (envelopes) for a team turn.
// This is the "project events" hook in the TurnExecutor lifecycle for team sessions.
type TeamProjectRuntimeEvent interface {
	ProjectTeamEvents(ctx context.Context, sessionID, runID string, outcome TurnOutcome, assistantMsg ChatMessage) error
}

// TeamRunStatusTransitioner is the biz-level port for transitioning team run
// status through the state machine. The internal/team runtime layer must use
// this instead of setting run.Status directly, ensuring all transitions are
// validated and timestamped consistently.
//
// Stability:evolving
type TeamRunStatusTransitioner interface {
	// TransitionRunStatus validates and applies a team run status transition.
	// Returns the updated TeamRun or an error if the transition is invalid.
	TransitionRunStatus(ctx context.Context, runID string, newStatus string) (TeamRunRecord, error)
}
