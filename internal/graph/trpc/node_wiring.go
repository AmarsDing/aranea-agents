package graph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func normalizeNodeType(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}

func nodeOptions(n NodeDef, policy *biz.TeamFailurePolicy) []trpcgraph.Option {
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
	opts = append(opts, failureRecoveryOptions(n)...)
	if policy != nil && policy.CircuitBreaker != nil {
		opts = append(opts, circuitBreakerOptions(n, policy.CircuitBreaker)...)
	}
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

func wireNode(ctx context.Context, sg *trpcgraph.StateGraph, n NodeDef, deps *BuildDeps, policy *biz.TeamFailurePolicy) ([]trpcagent.Agent, error) {
	opts := nodeOptions(n, policy)
	switch normalizeNodeType(n.Type) {
	case "llm":
		if deps == nil || deps.Models == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type llm requires BuildDeps.Models", n.ID))
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
	case "tool", "tools":
		if deps == nil || deps.Tools == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type tool requires BuildDeps.Tools", n.ID))
		}
		toolMap, err := deps.Tools.ResolveTools(ctx, n.ToolNames)
		if err != nil {
			return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: node %q tools: %v", n.ID, err))
		}
		sg.AddToolsNode(n.ID, toolMap, opts...)
		return nil, nil
	case "agent":
		ref := strings.TrimSpace(n.AgentName)
		if ref == "" {
			ref = strings.TrimSpace(n.ID)
		}
		if deps == nil || deps.Agents == nil {
			return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type agent requires BuildDeps.Agents", n.ID))
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
	case "function":
		if n.Func != nil {
			sg.AddNode(n.ID, n.Func, opts...)
			return nil, nil
		}
		if strings.TrimSpace(n.FuncRef) == biz.SkipNodeFuncRef {
			sg.AddNode(n.ID, SkipNodeFunc(n.ID), opts...)
			return nil, nil
		}
		return nil, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph: node %q type function requires Func or %q FuncRef", n.ID, biz.SkipNodeFuncRef))
	case "router":
		sg.AddNode(n.ID, func(ctx context.Context, state trpcgraph.State) (any, error) {
			return state, nil
		}, opts...)
		return nil, nil
	case "task", "review":
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
