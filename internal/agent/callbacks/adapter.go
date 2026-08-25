package callbacks

import (
	"context"
	"reflect"
	"runtime"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
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

// BeforeModelHookFunc wraps a plain function as a BeforeModelHook.
type BeforeModelHookFunc struct {
	priority int
	layer    SystemLayer
	name     string
	fn       trpcmodel.BeforeModelCallbackStructured
}

var _ BeforeModelHook = (*BeforeModelHookFunc)(nil)
var _ LayeredCallback = (*BeforeModelHookFunc)(nil)

// NewBeforeModelHook creates a BeforeModelHookFunc with a given priority, layer, and handler.
// The hook name is auto-derived from the handler's enclosing function
// (e.g. "agent.newMemoryInjectBeforeHook.func1") for the C3 execution-order
// golden (79-runtime-governance 附录 A.1) and debug logging; it carries no
// behavioral effect.
func NewBeforeModelHook(priority int, layer SystemLayer, fn trpcmodel.BeforeModelCallbackStructured) *BeforeModelHookFunc {
	return &BeforeModelHookFunc{priority: priority, layer: layer, name: deriveHookName(fn), fn: fn}
}

// deriveHookName returns the fully-qualified name of fn's enclosing function,
// stripping the module path prefix to the last two path segments
// ("<pkg>.<func>[.funcN]"). Returns "" for nil handlers.
func deriveHookName(fn trpcmodel.BeforeModelCallbackStructured) string {
	if fn == nil {
		return ""
	}
	pc := reflect.ValueOf(fn).Pointer()
	f := runtime.FuncForPC(pc)
	if f == nil {
		return ""
	}
	name := f.Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func (h *BeforeModelHookFunc) Point() CallbackPoint { return PointBeforeModel }
func (h *BeforeModelHookFunc) Priority() int        { return h.priority }
func (h *BeforeModelHookFunc) Layer() SystemLayer   { return h.layer }

// Name returns the auto-derived handler identity (see NewBeforeModelHook).
func (h *BeforeModelHookFunc) Name() string { return h.name }

func (h *BeforeModelHookFunc) HandleBeforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	return h.fn(ctx, args)
}

// AfterModelHookFunc wraps a plain function as an AfterModelHook.
type AfterModelHookFunc struct {
	priority int
	fn       trpcmodel.AfterModelCallbackStructured
}

var _ AfterModelHook = (*AfterModelHookFunc)(nil)

// NewAfterModelHook creates an AfterModelHookFunc with a given priority and handler.
func NewAfterModelHook(priority int, fn trpcmodel.AfterModelCallbackStructured) *AfterModelHookFunc {
	return &AfterModelHookFunc{priority: priority, fn: fn}
}

func (h *AfterModelHookFunc) Point() CallbackPoint { return PointAfterModel }
func (h *AfterModelHookFunc) Priority() int        { return h.priority }
func (h *AfterModelHookFunc) HandleAfterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
	return h.fn(ctx, args)
}

// BeforeToolHookFunc wraps a plain function as a BeforeToolHook.
type BeforeToolHookFunc struct {
	priority int
	fn       trpctool.BeforeToolCallbackStructured
}

var _ BeforeToolHook = (*BeforeToolHookFunc)(nil)

// NewBeforeToolHook creates a BeforeToolHookFunc with a given priority and handler.
func NewBeforeToolHook(priority int, fn trpctool.BeforeToolCallbackStructured) *BeforeToolHookFunc {
	return &BeforeToolHookFunc{priority: priority, fn: fn}
}

func (h *BeforeToolHookFunc) Point() CallbackPoint { return PointBeforeTool }
func (h *BeforeToolHookFunc) Priority() int        { return h.priority }
func (h *BeforeToolHookFunc) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	return h.fn(ctx, args)
}

// AfterToolHookFunc wraps a plain function as an AfterToolHook.
type AfterToolHookFunc struct {
	priority int
	fn       trpctool.AfterToolCallbackStructured
}

var _ AfterToolHook = (*AfterToolHookFunc)(nil)

// NewAfterToolHook creates an AfterToolHookFunc with a given priority and handler.
func NewAfterToolHook(priority int, fn trpctool.AfterToolCallbackStructured) *AfterToolHookFunc {
	return &AfterToolHookFunc{priority: priority, fn: fn}
}

func (h *AfterToolHookFunc) Point() CallbackPoint { return PointAfterTool }
func (h *AfterToolHookFunc) Priority() int        { return h.priority }
func (h *AfterToolHookFunc) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	return h.fn(ctx, args)
}
