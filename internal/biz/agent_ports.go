package biz

import "context"

// AgentRuntimeBuilder is the biz-level port for building an Agent runtime.
// Consumers (ChatOrchestrator, Team, Graph adapter) depend on this instead
// of reaching into internal/agent internals.
//
// Implementations live in internal/agent. Wire binding in internal/service.
type AgentRuntimeBuilder interface {
	// BuildAgent constructs a runtime agent from a biz Agent model.
	BuildAgent(ctx context.Context, agent Agent) (AgentRuntime, error)
}

// AgentRuntime is the biz-level handle for a built agent runtime.
// The concrete type is framework-specific; this interface exposes
// only what biz-level consumers need.
type AgentRuntime interface {
	// AgentID returns the agent's unique identifier.
	AgentID() string
}

// ToolsetAssembler is the biz-level port for assembling tools for an agent turn.
// Consumers (ChatOrchestrator, Team, Graph adapter) depend on this instead
// of reaching into internal/tools internals.
//
// Implementations live in internal/tools. Wire binding in internal/service.
type ToolsetAssembler interface {
	// AssembleForAgent builds the toolset for a given agent configuration.
	AssembleForAgent(ctx context.Context, agent Agent, overrides ToolOverrides) (ToolsetResult, error)
}

// ToolOverrides carries per-turn tool configuration overrides.
type ToolOverrides struct {
	EnabledTools []string
	MCPOverrides []MCPServerOverride
}

// MCPServerOverride is a per-turn MCP server configuration override.
type MCPServerOverride struct {
	ServerKey string
	Config    map[string]any
}

// ToolsetResult is the biz-level result of toolset assembly.
type ToolsetResult struct {
	ToolCount    int
	ToolSetCount int
}

// ModelResolverPort is the biz-level port for resolving LLM models.
// Consumers (ChatOrchestrator, Graph adapter) depend on this instead
// of reaching into internal/provider internals.
//
// Implementations live in internal/provider. Wire binding in internal/service.
type ModelResolverPort interface {
	// ResolveModel resolves a provider/model pair into a biz ModelInfo.
	ResolveModel(ctx context.Context, providerName, modelName string) (ModelInfo, error)
}

// ModelInfo is the biz-level model metadata.
type ModelInfo struct {
	Provider    string
	Model       string
	DisplayName string
	MaxTokens   int
}

// ---------------------------------------------------------------------------
// Agent Turn Lifecycle Hooks
// ---------------------------------------------------------------------------
// These interfaces allow the TurnExecutor to delegate Agent-specific behavior
// without knowing the concrete agent runtime implementation. The ChatOrchestrator
// or a dedicated AgentTurnHook implementation will satisfy these.

// AgentBuildRunner constructs the trpc-agent-go runner for a single agent turn.
// This is the "build" hook in the TurnExecutor lifecycle.
type AgentBuildRunner interface {
	// BuildRunner constructs a callable agent runner from the given agent and session.
	BuildRunner(ctx context.Context, agent Agent, sessionID string, input TurnInput) (AgentRunnerHandle, error)
}

// AgentRunnerHandle is the biz-level handle for a built agent runner.
// It abstracts the trpc-agent-go Runner so biz doesn't import it.
type AgentRunnerHandle interface {
	// Run executes the agent turn and returns the assistant message content.
	Run(ctx context.Context) (AgentTurnOutput, error)
	// Close releases any resources held by the runner.
	Close()
}

// AgentTurnOutput carries the result of a single agent turn execution.
type AgentTurnOutput struct {
	ContentMarkdown string
	TokenIn         int
	TokenOut        int
	ToolUseEvents   []ToolUseEventRef
}

// ToolUseEventRef is a lightweight reference to a tool use event produced during a turn.
type ToolUseEventRef struct {
	ID     string
	Name   string
	Status string
}

// AgentPersistTurnRecord persists the turn result for an agent session.
// This is the "persist" hook in the TurnExecutor lifecycle.
type AgentPersistTurnRecord interface {
	PersistAgentTurn(ctx context.Context, sessionID string, userMsg, assistantMsg ChatMessage) error
}

// AgentProjectRuntimeEvent emits runtime events (envelopes) for an agent turn.
// This is the "project events" hook in the TurnExecutor lifecycle.
type AgentProjectRuntimeEvent interface {
	ProjectAgentEvents(ctx context.Context, sessionID, runID string, outcome TurnOutcome, assistantMsg ChatMessage) error
}
