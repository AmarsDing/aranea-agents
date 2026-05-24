package adapter

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CatalogFunctionResolver resolves catalog tools as CallableTool for graph
// function nodes. Function nodes are lightweight callables that don't need
// the full Agent lifecycle.
type CatalogFunctionResolver struct {
	Tools *biz.ToolUsecase
}

var _ graphtrpc.FunctionResolver = (*CatalogFunctionResolver)(nil)

func NewCatalogFunctionResolver(tools *biz.ToolUsecase) *CatalogFunctionResolver {
	return &CatalogFunctionResolver{Tools: tools}
}

func (r *CatalogFunctionResolver) ResolveFunction(ctx context.Context, funcRef string) (trpctool.CallableTool, error) {
	if r == nil || r.Tools == nil {
		return nil, fmt.Errorf("graph: tool catalog not configured for function resolution")
	}
	key := strings.TrimSpace(funcRef)
	if key == "" {
		return nil, fmt.Errorf("graph: function func_ref is required")
	}
	row, err := r.Tools.GetTool(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("graph: function %q: %w", key, err)
	}
	callable, _, err := callableFromBizTool(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("graph: function %q: %w", key, err)
	}
	ct, ok := callable.(trpctool.CallableTool)
	if !ok {
		return nil, fmt.Errorf("graph: function %q: tool is not callable", key)
	}
	return ct, nil
}
