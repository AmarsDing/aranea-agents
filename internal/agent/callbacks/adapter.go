package callbacks

import (
	"context"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ToolRecorderCallback is a Callback that runs an AfterTool hook.
// It wraps an arbitrary AfterToolCallbackStructured function so that
// product-layer tooling (e.g. usage recording) participates in the Chain.
type ToolRecorderCallback struct {
	priority int
	fn       trpctool.AfterToolCallbackStructured
}

var _ AfterToolHook = (*ToolRecorderCallback)(nil)

// NewToolRecorderCallback creates a ToolRecorderCallback with a given priority and handler.
func NewToolRecorderCallback(priority int, fn trpctool.AfterToolCallbackStructured) *ToolRecorderCallback {
	return &ToolRecorderCallback{priority: priority, fn: fn}
}

func (t *ToolRecorderCallback) Point() CallbackPoint { return PointAfterTool }
func (t *ToolRecorderCallback) Priority() int        { return t.priority }

func (t *ToolRecorderCallback) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	return t.fn(ctx, args)
}

// BeforeAgentHookFunc wraps a plain function as a BeforeAgentHook.
type BeforeAgentHookFunc struct {
	priority int
	fn       trpcagent.BeforeAgentCallbackStructured
}

var _ BeforeAgentHook = (*BeforeAgentHookFunc)(nil)

// NewBeforeAgentHook creates a BeforeAgentHookFunc with a given priority and handler.
func NewBeforeAgentHook(priority int, fn trpcagent.BeforeAgentCallbackStructured) *BeforeAgentHookFunc {
	return &BeforeAgentHookFunc{priority: priority, fn: fn}
}

func (h *BeforeAgentHookFunc) Point() CallbackPoint { return PointBeforeAgent }
func (h *BeforeAgentHookFunc) Priority() int        { return h.priority }
func (h *BeforeAgentHookFunc) HandleBeforeAgent(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
	return h.fn(ctx, args)
}

// AfterAgentHookFunc wraps a plain function as an AfterAgentHook.
type AfterAgentHookFunc struct {
	priority int
	fn       trpcagent.AfterAgentCallbackStructured
}

var _ AfterAgentHook = (*AfterAgentHookFunc)(nil)

// NewAfterAgentHook creates an AfterAgentHookFunc with a given priority and handler.
func NewAfterAgentHook(priority int, fn trpcagent.AfterAgentCallbackStructured) *AfterAgentHookFunc {
	return &AfterAgentHookFunc{priority: priority, fn: fn}
}

func (h *AfterAgentHookFunc) Point() CallbackPoint { return PointAfterAgent }
func (h *AfterAgentHookFunc) Priority() int        { return h.priority }
func (h *AfterAgentHookFunc) HandleAfterAgent(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
	return h.fn(ctx, args)
}
