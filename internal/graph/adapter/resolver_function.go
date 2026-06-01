package adapter

import (
	"context"
	"fmt"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CatalogFunctionResolver resolves catalog tools as CallableTool for graph
// function nodes. Function nodes are lightweight callables that don't need
// the full Agent lifecycle.
type CatalogFunctionResolver struct {
	Tools *biz.ToolUsecase
	lg    loggateway.Logger
}

var _ graphtrpc.FunctionResolver = (*CatalogFunctionResolver)(nil)

func NewCatalogFunctionResolver(tools *biz.ToolUsecase, lg loggateway.Logger) *CatalogFunctionResolver {
	return &CatalogFunctionResolver{Tools: tools, lg: lg}
}

func (r *CatalogFunctionResolver) ResolveFunction(ctx context.Context, funcRef string) (trpctool.CallableTool, error) {
	if r == nil || r.Tools == nil {
		return nil, kerrors.InternalServer("GRAPH", "graph: tool catalog not configured for function resolution")
	}
	key := strings.TrimSpace(funcRef)
	if key == "" {
		return nil, kerrors.BadRequest("GRAPH", "graph: function func_ref is required")
	}
	row, err := r.Tools.GetTool(ctx, key)
	if err != nil {
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: function %q: %v", key, err))
	}
	callable, _, err := callableFromBizTool(ctx, row, r.lg)
	if err != nil {
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: function %q: %v", key, err))
	}
	ct, ok := callable.(trpctool.CallableTool)
	if !ok {
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph: function %q: tool is not callable", key))
	}
	return ct, nil
}
