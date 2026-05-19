package graph

import (
	"context"
	"fmt"
	"strings"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func normalizeNodeType(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}

func nodeOptions(n NodeDef) []trpcgraph.Option {
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
	return opts
}

func wireNode(ctx context.Context, sg *trpcgraph.StateGraph, n NodeDef, deps *BuildDeps) error {
	opts := nodeOptions(n)
	switch normalizeNodeType(n.Type) {
	case "llm":
		if deps == nil || deps.Models == nil {
			return fmt.Errorf("graph: node %q type llm requires BuildDeps.Models", n.ID)
		}
		mdl, err := deps.Models.ResolveModel(ctx, n.ModelName)
		if err != nil {
			return fmt.Errorf("graph: node %q llm model: %w", n.ID, err)
		}
		toolMap := map[string]trpctool.Tool{}
		if deps.Tools != nil && len(n.ToolNames) > 0 {
			toolMap, err = deps.Tools.ResolveTools(ctx, n.ToolNames)
			if err != nil {
				return fmt.Errorf("graph: node %q llm tools: %w", n.ID, err)
			}
		}
		sg.AddLLMNode(n.ID, mdl, n.Instruction, toolMap, opts...)
		return nil
	case "tool", "tools":
		if deps == nil || deps.Tools == nil {
			return fmt.Errorf("graph: node %q type tool requires BuildDeps.Tools", n.ID)
		}
		toolMap, err := deps.Tools.ResolveTools(ctx, n.ToolNames)
		if err != nil {
			return fmt.Errorf("graph: node %q tools: %w", n.ID, err)
		}
		sg.AddToolsNode(n.ID, toolMap, opts...)
		return nil
	default:
		if n.Func == nil {
			return fmt.Errorf("graph: node %q has no Func (type=%q FuncRef=%q)", n.ID, n.Type, n.FuncRef)
		}
		sg.AddNode(n.ID, n.Func, opts...)
		return nil
	}
}
