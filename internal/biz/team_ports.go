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

// TeamTurnRunnerPort is the biz-level port for the team turn runner.
// TeamService depends on this instead of *team.Runner.
//
// Stability:evolving
type TeamTurnRunnerPort interface {
	// RunTurnFromInput executes one user turn for a team session.
	RunTurnFromInput(ctx context.Context, sess Session, input TurnInput) (ChatMessage, ChatMessage, error)
}

// RunRegistryPort is the biz-level port for the runtime run registry.
// TeamService depends on this instead of *rt.RunRegistry.
//
// Stability:evolving
type RunRegistryPort interface {
	// Cancel cancels the active run for the given session.
	Cancel(sessionID string) (bool, string)

	// GetStatus returns the current status entry for the given session.
	GetStatus(sessionID string) (RunStatusEntry, bool)
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
type TeamRunObserver interface {
	// OnTeamRunStarted is called when a team run begins.
	OnTeamRunStarted(ctx context.Context, run TeamRun)

	// OnTeamRunStepFinished is called when a single step completes.
	OnTeamRunStepFinished(ctx context.Context, step TeamRunStep)

	// OnTeamRunFinished is called when a team run completes (success or failure).
	OnTeamRunFinished(ctx context.Context, run TeamRun)
}

// ---------------------------------------------------------------------------
// Team Turn Lifecycle Hooks
// ---------------------------------------------------------------------------
// These interfaces allow the TurnExecutor to delegate Team-specific behavior
// without knowing the concrete team runtime implementation.

// TeamBuildRunner constructs the runtime for a single team turn.
// This is the "build" hook in the TurnExecutor lifecycle for team sessions.
type TeamBuildRunner interface {
	// BuildTeamRunner constructs a callable team runner for the given session.
	BuildTeamRunner(ctx context.Context, session Session, input TurnInput) (TeamRunnerHandle, error)
}

// TeamRunnerHandle is the biz-level handle for a built team runner.
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
