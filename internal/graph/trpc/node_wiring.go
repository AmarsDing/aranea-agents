package graph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func normalizeNodeType(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}

func nodeOptions(n NodeDef, resolvedFallback trpcagent.Agent) []trpcgraph.Option {
	opts := []trpcgraph.Option{}
	if n.InterruptBefore {
		opts = append(opts, trpcgraph.WithInterruptBefore())
	}
	if n.InterruptAfter {
		opts = append(opts, trpcgraph.WithInterruptAfter())
	}
	if len(n.Destinations) > 0 {
		ends := make(map[string]string, len(n.Destinations))
		for _, d := range n.Destinations {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			ends[d] = d
		}
		if len(ends) > 0 {
			opts = append(opts, trpcgraph.WithEndsMap(ends))
		}
	}
	if n.RetryMaxAttempts > 0 {
		opts = append(opts, trpcgraph.WithRetryPolicy(trpcgraph.WithSimpleRetry(n.RetryMaxAttempts)))
	}
	if n.CacheEnabled {
		pol := trpcgraph.DefaultCachePolicy()
		if n.CacheTTLSeconds > 0 {
			pol = &trpcgraph.CachePolicy{
				KeyFunc: trpcgraph.DefaultCachePolicy().KeyFunc,
				TTL:     time.Duration(n.CacheTTLSeconds) * time.Second,
			}
		}
		opts = append(opts, trpcgraph.WithNodeCachePolicy(pol))
	}
	opts = append(opts, agentMapperOptions(n)...)
	opts = append(opts, failureRecoveryOptions(n, resolvedFallback)...)
	return opts
}

func agentMapperOptions(n NodeDef) []trpcgraph.Option {
	opts := []trpcgraph.Option{}
	if m := parseInputMapper(n.InputMapperJSON); m != nil {
		opts = append(opts, trpcgraph.WithSubgraphInputMapper(m))
	}
	if m := parseOutputMapper(n.OutputMapperJSON); m != nil {
		opts = append(opts, trpcgraph.WithSubgraphOutputMapper(m))
	}
	if n.IsolatedMessages {
		opts = append(opts, trpcgraph.WithSubgraphIsolatedMessages(true))
	}
	if n.InputFromLastResponse {
		opts = append(opts, trpcgraph.WithSubgraphInputFromLastResponse())
	}
	return opts
}

func wireNode(ctx context.Context, sg *trpcgraph.StateGraph, n NodeDef, deps *GraphNodeResolverSet, lg loggateway.Logger) ([]trpcagent.Agent, error) {
	var resolvedFallback trpcagent.Agent
	if fb := strings.TrimSpace(n.FallbackAgent); fb != "" && deps != nil && deps.Agents != nil {
		if fa, ferr := deps.Agents.ResolveAgent(ctx, fb); ferr == nil {
			resolvedFallback = fa
		}
	}
	opts := nodeOptions(n, resolvedFallback)
	switch normalizeNodeType(n.Type) {
	case biz.NodeTypeLLM:
		if deps == nil || deps.Models == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type llm requires GraphNodeResolverSet.Models", n.ID))
		}
		mdl, err := deps.Models.ResolveModel(ctx, n.ModelName)
		if err != nil {
			return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: node %q llm model: %v", n.ID, err))
		}
		toolMap := map[string]trpctool.Tool{}
		if deps.Tools != nil && len(n.ToolNames) > 0 {
			toolMap, err = deps.Tools.ResolveTools(ctx, n.ToolNames)
			if err != nil {
				return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: node %q llm tools: %v", n.ID, err))
			}
		}
		sg.AddLLMNode(n.ID, mdl, n.Instruction, toolMap, opts...)
		return nil, nil
	case biz.NodeTypeTool, biz.NodeTypeTools:
		if deps == nil || deps.Tools == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type tool requires GraphNodeResolverSet.Tools", n.ID))
		}
		toolMap, err := deps.Tools.ResolveTools(ctx, n.ToolNames)
		if err != nil {
			return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: node %q tools: %v", n.ID, err))
		}
		sg.AddToolsNode(n.ID, toolMap, opts...)
		return nil, nil
	case biz.NodeTypeAgent:
		ref := strings.TrimSpace(n.AgentName)
		if ref == "" {
			ref = strings.TrimSpace(n.ID)
		}
		if deps == nil || deps.Agents == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type agent requires GraphNodeResolverSet.Agents", n.ID))
		}
		sub, err := deps.Agents.ResolveAgent(ctx, ref)
		if err != nil {
			return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: node %q agent %q: %v", n.ID, ref, err))
		}
		extras := []trpcagent.Agent{sub}
		if fb := strings.TrimSpace(n.FallbackAgent); fb != "" {
			fallback, ferr := deps.Agents.ResolveAgent(ctx, fb)
			if ferr != nil {
				return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: node %q fallback agent %q: %v", n.ID, fb, ferr))
			}
			extras = append(extras, fallback)
		}
		sg.AddAgentNode(n.ID, opts...)
		return extras, nil
	case biz.NodeTypeFunction:
		if n.Func != nil {
			sg.AddNode(n.ID, n.Func, opts...)
			return nil, nil
		}
		if strings.TrimSpace(n.FuncRef) == biz.SkipNodeFuncRef {
			sg.AddNode(n.ID, SkipNodeFunc(n.ID), opts...)
			return nil, nil
		}
		// Try resolving via FunctionResolver if available.
		if deps != nil && deps.Functions != nil && strings.TrimSpace(n.FuncRef) != "" {
			ct, err := deps.Functions.ResolveFunction(ctx, n.FuncRef)
			if err == nil && ct != nil {
				sg.AddNode(n.ID, callableToolToNodeFunc(n.ID, ct), opts...)
				return nil, nil
			}
			// Degradation: FunctionResolver failed, fall back to skip node with warning.
			// The function will be treated as a no-op pass-through at runtime.
			if lg != nil {
				lg.Warn("FunctionResolver 降级：运行时函数解析失败，节点将作为 no-op 透传",
					loggateway.StepID("graph.function_resolver_degradation"),
					loggateway.Str("node_id", n.ID),
					loggateway.Str("func_ref", n.FuncRef),
					loggateway.Err(err))
			}
			sg.AddNode(n.ID, SkipNodeFunc(n.ID), opts...)
			return nil, nil
		}
		return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type function requires Func or %q FuncRef", n.ID, biz.SkipNodeFuncRef))
	case biz.NodeTypeRouter:
		sg.AddNode(n.ID, func(ctx context.Context, state trpcgraph.State) (any, error) {
			return state, nil
		}, opts...)
		return nil, nil
	case biz.NodeTypeTask, biz.NodeTypeReview:
		if !n.InterruptAfter {
			opts = append(opts, trpcgraph.WithInterruptAfter())
		}
		sg.AddNode(n.ID, func(ctx context.Context, state trpcgraph.State) (any, error) {
			return state, nil
		}, opts...)
		return nil, nil
	default:
		if n.Func == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q has no Func (type=%q FuncRef=%q)", n.ID, n.Type, n.FuncRef))
		}
		sg.AddNode(n.ID, n.Func, opts...)
		return nil, nil
	}
}

// callableToolToNodeFunc wraps a CallableTool as a NodeFunc for graph function nodes.
func callableToolToNodeFunc(id string, ct trpctool.CallableTool) trpcgraph.NodeFunc {
	return func(ctx context.Context, state trpcgraph.State) (any, error) {
		result, err := ct.Call(ctx, nil)
		if err != nil {
			return nil, err
		}
		_ = result
		return state, nil
	}
}
