package tools

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ApplyDecorators wraps all CallableTools in the AssembledToolsets with
// ToolDecorator. Standalone Tools are decorated in place; ToolSets are
// wrapped with decoratedToolSet so their tools are also decorated when
// retrieved via Tools(ctx).
//
// Non-callable tools (pure metadata) are passed through unchanged.
// The decorator applies per-call timeout, result budget truncation, and
// deterministic caching for ConcurrentSafe tools.
//
// Limitation (DeferredManager): This function only decorates tools that
// are present in AssembledToolsets at decoration time. Tools that are
// lazily loaded by the DeferredManager (internal/tools/deferred) after
// decoration are NOT wrapped with ToolDecorator. This is acceptable
// because deferred tools are typically MCP/agent tools whose execution
// path already has its own timeout/governance via the framework's
// runner-level controls. If a deferred tool needs decorator-level
// protection in the future, the DeferredToolManager should be extended
// to apply decorators when materializing tools, or the decoration should
// move to the tool retrieval boundary.
func ApplyDecorators(ts *AssembledToolsets, cfg ToolDecoratorConfig) {
	if ts == nil {
		return
	}
	// Decorate standalone tools in place.
	for i, t := range ts.Tools {
		if t == nil {
			continue
		}
		if ct, ok := t.(trpctool.CallableTool); ok {
			ts.Tools[i] = NewToolDecorator(ct, cfg)
		}
	}
	// Wrap toolsets so their tools are decorated on retrieval.
	for i, set := range ts.ToolSets {
		if set == nil {
			continue
		}
		ts.ToolSets[i] = &decoratedToolSet{inner: set, cfg: cfg}
	}
}

// decoratedToolSet wraps a ToolSet so that tools returned by Tools(ctx)
// are wrapped with ToolDecorator. This follows the same pattern as
// confirmingToolSet in the trpc sub-package.
//
// Tools are decorated fresh on each Tools(ctx) call (consistent with
// the framework's ToolSet contract). As a result, the deterministic
// cache is effective for standalone Tools but not for ToolSet-managed
// tools (each retrieval creates a new decorator instance). Timeout and
// result budget apply to both.
type decoratedToolSet struct {
	inner trpctool.ToolSet
	cfg   ToolDecoratorConfig
}

// Compile-time interface assertion.
var _ trpctool.ToolSet = (*decoratedToolSet)(nil)

func (s *decoratedToolSet) Name() string {
	if s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *decoratedToolSet) Close() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *decoratedToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		if t == nil {
			continue
		}
		if ct, ok := t.(trpctool.CallableTool); ok {
			out[i] = NewToolDecorator(ct, s.cfg)
		} else {
			out[i] = t
		}
	}
	return out
}
