// Package callbacks provides a priority-ordered chain of agent/model/tool callbacks
// that wrap the trpc-agent-go native callback types. It acts as the single
// registration hub for Aranea product-layer hooks, keeping the adapter
// code separate from the trpc-agent-go internals.
package callbacks

import (
	"context"
	"sort"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CallbackPoint identifies where in the agent lifecycle a callback fires.
type CallbackPoint int

const (
	PointBeforeAgent CallbackPoint = iota
	PointAfterAgent
	PointBeforeModel
	PointAfterModel
	PointBeforeTool
	PointAfterTool
	PointOnError
)

// Callback is a single entry in a Chain. Implement exactly one Handle* method
// for the point(s) you care about; unused methods may be left as no-ops.
type Callback interface {
	// Point declares which lifecycle hook this callback handles.
	Point() CallbackPoint

	// Priority controls ordering within a Chain. Lower value = earlier execution.
	// Callbacks with equal priority run in registration order.
	Priority() int
}

// BeforeAgentHook is implemented by callbacks that fire before the agent runs.
type BeforeAgentHook interface {
	Callback
	HandleBeforeAgent(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error)
}

// AfterAgentHook is implemented by callbacks that fire after the agent runs.
type AfterAgentHook interface {
	Callback
	HandleAfterAgent(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error)
}

// BeforeModelHook fires before the model is called.
type BeforeModelHook interface {
	Callback
	HandleBeforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
}

// AfterModelHook fires after the model has responded.
type AfterModelHook interface {
	Callback
	HandleAfterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error)
}

// BeforeToolHook fires before a tool is executed.
type BeforeToolHook interface {
	Callback
	HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
}

// AfterToolHook fires after a tool has executed.
type AfterToolHook interface {
	Callback
	HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error)
}

// PluginCallback marks a Chain entry that originated from a DB-backed plugin.
// Built-in plugins normally register via Runner WithPlugins; this interface is
// reserved for future Chain mirroring when double-invocation is ruled out.
type PluginCallback interface {
	Callback
	// PluginName returns the identifier of the originating plugin.
	PluginName() string
}

// Chain is a prioritised, immutable list of Callback entries.
// Build one with NewChain; use the Adapt* methods to convert to
// trpc-agent-go native callback slices.
type Chain struct {
	entries []Callback
}

// NewChain creates a Chain sorted by Priority() then registration order.
func NewChain(cbs ...Callback) *Chain {
	sorted := make([]Callback, len(cbs))
	copy(sorted, cbs)
	// stable sort preserves registration order for equal priorities.
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority() < sorted[j].Priority()
	})
	return &Chain{entries: sorted}
}

// Append returns a new Chain with additional callbacks appended (and re-sorted).
func (c *Chain) Append(cbs ...Callback) *Chain {
	all := make([]Callback, len(c.entries)+len(cbs))
	copy(all, c.entries)
	copy(all[len(c.entries):], cbs)
	return NewChain(all...)
}

// HasAgentHooks reports whether the chain contains agent lifecycle hooks.
func (c *Chain) HasAgentHooks() bool {
	if c == nil {
		return false
	}
	for _, cb := range c.entries {
		switch cb.(type) {
		case BeforeAgentHook, AfterAgentHook:
			return true
		}
	}
	return false
}

// HasModelHooks reports whether the chain contains model lifecycle hooks.
func (c *Chain) HasModelHooks() bool {
	if c == nil {
		return false
	}
	for _, cb := range c.entries {
		switch cb.(type) {
		case BeforeModelHook, AfterModelHook:
			return true
		}
	}
	return false
}

// HasToolHooks reports whether the chain contains tool lifecycle hooks.
func (c *Chain) HasToolHooks() bool {
	if c == nil {
		return false
	}
	for _, cb := range c.entries {
		switch cb.(type) {
		case BeforeToolHook, AfterToolHook:
			return true
		}
	}
	return false
}

// AdaptAgentCallbacks constructs a *trpcagent.Callbacks from every
// BeforeAgentHook and AfterAgentHook in the chain.
func (c *Chain) AdaptAgentCallbacks() *trpcagent.Callbacks {
	ac := trpcagent.NewCallbacks(trpcagent.WithContinueOnError(false))
	for _, cb := range c.entries {
		if h, ok := cb.(BeforeAgentHook); ok {
			h := h
			ac.RegisterBeforeAgent(trpcagent.BeforeAgentCallbackStructured(
				func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
					return h.HandleBeforeAgent(ctx, args)
				},
			))
		}
		if h, ok := cb.(AfterAgentHook); ok {
			h := h
			ac.RegisterAfterAgent(trpcagent.AfterAgentCallbackStructured(
				func(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
					return h.HandleAfterAgent(ctx, args)
				},
			))
		}
	}
	return ac
}

// AdaptModelCallbacks constructs a *trpcmodel.Callbacks from every
// BeforeModelHook and AfterModelHook in the chain.
func (c *Chain) AdaptModelCallbacks() *trpcmodel.Callbacks {
	mc := trpcmodel.NewCallbacks()
	for _, cb := range c.entries {
		if h, ok := cb.(BeforeModelHook); ok {
			h := h
			mc.RegisterBeforeModel(trpcmodel.BeforeModelCallbackStructured(
				func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
					return h.HandleBeforeModel(ctx, args)
				},
			))
		}
		if h, ok := cb.(AfterModelHook); ok {
			h := h
			mc.RegisterAfterModel(trpcmodel.AfterModelCallbackStructured(
				func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
					return h.HandleAfterModel(ctx, args)
				},
			))
		}
	}
	return mc
}

// AdaptToolCallbacks constructs a *trpctool.Callbacks from every
// BeforeToolHook and AfterToolHook in the chain.
func (c *Chain) AdaptToolCallbacks() *trpctool.Callbacks {
	tc := trpctool.NewCallbacks()
	for _, cb := range c.entries {
		if h, ok := cb.(BeforeToolHook); ok {
			h := h
			tc.RegisterBeforeTool(trpctool.BeforeToolCallbackStructured(
				func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
					return h.HandleBeforeTool(ctx, args)
				},
			))
		}
		if h, ok := cb.(AfterToolHook); ok {
			h := h
			tc.RegisterAfterTool(trpctool.AfterToolCallbackStructured(
				func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
					return h.HandleAfterTool(ctx, args)
				},
			))
		}
	}
	return tc
}
