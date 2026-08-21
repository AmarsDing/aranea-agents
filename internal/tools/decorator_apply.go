package tools

import (
	"context"
	"sync"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ApplyDecorators wraps all CallableTools in the AssembledToolsets with
// ToolDecorator. Standalone Tools are decorated in place; ToolSets are
// wrapped with decoratedToolSet so their tools are also decorated when
// retrieved via Tools(ctx).
//
// Non-callable tools (pure metadata) are passed through unchanged.
// The decorator applies per-call timeout, result budget truncation,
// argument normalization, and deterministic caching for ConcurrentSafe tools.
//
// DeferredManager note (2026-08-21 修正，此前注释误称延迟工具无装饰器):
// the assembly pipeline runs FinalizeDeferredTools (shard_merge.go) BEFORE
// ApplyDecorators (tool_assembly.go), so each DeferredToolSet is itself
// wrapped by decoratedToolSet. tool_load activation only flips the
// visibility filter — the framework still resolves the callable via
// Tools(ctx), which returns the ToolDecorator-wrapped DeferredCallableTool.
// Deferred tools therefore keep decorator protection (timeout / result
// budget / arg normalization / deterministic cache) after activation.
// The manager.RegisterTool references collected in FinalizeDeferredTools
// feed tool_load's schema display only; they are not an execution path.
func ApplyDecorators(ts *AssembledToolsets, cfg ToolDecoratorConfig) {
	if ts == nil {
		return
	}
	for i, t := range ts.Tools {
		if t == nil {
			continue
		}
		if ct, ok := t.(trpctool.CallableTool); ok {
			ts.Tools[i] = NewToolDecorator(ct, cfg)
		}
	}
	for i, set := range ts.ToolSets {
		if set == nil {
			continue
		}
		ts.ToolSets[i] = newDecoratedToolSet(set, cfg)
	}
}

// DefaultDecoratorConfig is the product-layer decorator used by LLM
// assembly, catalog online test, and graph tool nodes so they share one
// Call path (normalize + locks + budget).
func DefaultDecoratorConfig(lg loggateway.Logger) ToolDecoratorConfig {
	return ToolDecoratorConfig{
		Timeout:       0,
		ResultBudget:  DefaultResultBudget,
		EnableCache:   true,
		Logger:        lg,
		StreamTimeout: DefaultStreamTimeout,
		StreamBudget:  DefaultStreamBudget,
	}
}

// ApplyDefaultDecorators applies DefaultDecoratorConfig.
func ApplyDefaultDecorators(ts *AssembledToolsets, lg loggateway.Logger) {
	ApplyDecorators(ts, DefaultDecoratorConfig(lg))
}

// decoratedToolSet wraps a ToolSet so that tools returned by Tools(ctx)
// are wrapped with ToolDecorator. Decorator instances are reused across
// Tools() calls so ConcurrentSafe ToolSet cache (TTL) actually hits.
type decoratedToolSet struct {
	inner      trpctool.ToolSet
	cfg        ToolDecoratorConfig
	mu         sync.Mutex
	decorators map[string]trpctool.CallableTool
}

func newDecoratedToolSet(inner trpctool.ToolSet, cfg ToolDecoratorConfig) *decoratedToolSet {
	return &decoratedToolSet{inner: inner, cfg: cfg}
}

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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decorators == nil {
		s.decorators = make(map[string]trpctool.CallableTool, len(raw))
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		if t == nil {
			continue
		}
		ct, ok := t.(trpctool.CallableTool)
		if !ok {
			out[i] = t
			continue
		}
		name := ""
		if d := ct.Declaration(); d != nil {
			name = d.Name
		}
		if name == "" {
			out[i] = NewToolDecorator(ct, s.cfg)
			continue
		}
		if existing, ok := s.decorators[name]; ok {
			out[i] = existing
			continue
		}
		dec := NewToolDecorator(ct, s.cfg)
		s.decorators[name] = dec
		out[i] = dec
	}
	return out
}
