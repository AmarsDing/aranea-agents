package biz

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Team→Agent Dependency Ports
// ---------------------------------------------------------------------------
// These interfaces allow the team package to consume agent runtime capabilities
// without directly importing internal/agent. The ChatOrchestrator or a dedicated
// adapter implements these and injects them into the team Runner.

// TeamTurnRunnerFactory builds and runs a team turn via the agent runtime.
// This is the primary port through which the team package executes turns,
// eliminating the direct dependency on agent.RunTRPCUserTurn and related helpers.
// Stability:evolving
type TeamTurnRunnerFactory interface {
	// NewTurnRunner creates a turn runner from the compiled root agent and config.
	NewTurnRunner(ctx context.Context, input TeamTurnRunnerInput) (TeamTurnRunnerHandle, error)
}

// TeamTurnRunnerInput carries all inputs needed to create a team turn runner.
type TeamTurnRunnerInput struct {
	SessionID  string
	RunID      string
	UserID     string
	Content    string
	DialogMode string
	Provider   string
	Model      string
	TeamID     string
	AgentID    string
	AgentKey   string
	EntryPoint TurnEntryPoint
	Timeout    time.Duration
}

// TeamTurnRunnerHandle manages the lifecycle of a team turn execution.
// Stability:evolving
type TeamTurnRunnerHandle interface {
	// Run executes the turn and returns the stream result.
	Run(ctx context.Context) (TeamStreamResult, error)
	// Close releases runner resources.
	Close()
}

// TeamStreamResult carries the output of a team turn stream execution,
// separate from the biz-level TeamTurnResult which includes messages.
type TeamStreamResult struct {
	ContentMarkdown string
	ReasoningText   string
	PromptTok       int
	CompletionTok   int
	HasError        bool
	LastError       string
	HasContent      bool
}

// TeamAgentHelper provides utility functions from the agent package that
// the team runner needs, without requiring a direct import.
// Stability:evolving
type TeamAgentHelper interface {
	// RFC3339Now returns the current time in RFC3339 format.
	RFC3339Now() string

	// UserIDFromCtx extracts the user ID from the context.
	UserIDFromCtx(ctx context.Context) string

	// UserOptionsJSON builds the user options JSON for a team turn.
	UserOptionsJSON(agent Agent, dialogMode, provider, model string, contextRatio float64, anchor *TeamMemberAnchorRef) (string, error)

	// AssistantOptionsJSON builds the assistant options JSON.
	AssistantOptionsJSON(agent Agent, anchor *TeamMemberAnchorRef) (string, error)

	// MergeReasoningIntoAssistantOptionsJSON merges reasoning text into assistant options.
	MergeReasoningIntoAssistantOptionsJSON(optsJSON, reasoning string) (string, error)

	// DisplayMarkdownFromStream extracts display markdown from a turn result.
	// The second return value indicates whether the display text is a reasoning fallback.
	DisplayMarkdownFromStream(result TeamStreamResult) (string, bool)

	// EstimateTokensIfMissing estimates token counts when not provided by the model.
	EstimateTokensIfMissing(promptTok, completionTok int, input, output string) (int, int)

	// ResolveRalphLoopTurn resolves Ralph Loop configuration for an agent.
	ResolveRalphLoopTurn(settingsJSON string) RalphLoopResult
}

// TeamMemberAnchorRef is the biz-level representation of a team member anchor,
// used for options JSON construction without importing internal/agent.
type TeamMemberAnchorRef struct {
	AgentID string
	Name    string
	Role    string
	TeamID  string // 新增：标识消息属于哪个团队
}

// RalphLoopResult carries the resolved Ralph Loop configuration.
type RalphLoopResult struct {
	Skip    bool
	Config  string
	SkipErr error
}
