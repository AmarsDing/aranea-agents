package biz

import "context"

// NativeTurnEvent carries the minimal context for post-turn side effects (evaluation, analytics).
type NativeTurnEvent struct {
	AgentID         string
	AgentConfigJSON string
	AgentSettings   *AgentRuntimeSettings
	SessionID       string
	UserInput       string
	AssistantOutput string
}

// NativeTurnAfterHook runs asynchronously after a successful native agent chat turn.
// Implementations must not block the chat path or propagate errors to the caller.
type NativeTurnAfterHook interface {
	AfterNativeTurn(ctx context.Context, ev NativeTurnEvent)
}
