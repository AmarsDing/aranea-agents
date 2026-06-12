package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/testexec"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CatalogToolResolver resolves catalog tools by key for graph tool nodes.
type CatalogToolResolver struct {
	Tools *biz.ToolUsecase
	lg    loggateway.Logger
}

var _ graphtrpc.ToolResolver = (*CatalogToolResolver)(nil)

func NewCatalogToolResolver(tools *biz.ToolUsecase, lg loggateway.Logger) *CatalogToolResolver {
	return &CatalogToolResolver{Tools: tools, lg: lg}
}

func (r *CatalogToolResolver) ResolveTools(ctx context.Context, toolNames []string) (map[string]trpctool.Tool, error) {
	if r == nil || r.Tools == nil {
		return nil, apierror.Internal(apierror.DomainGraph, "graph: tool catalog not configured")
	}
	out := make(map[string]trpctool.Tool)
	for _, name := range toolNames {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		row, err := r.Tools.GetTool(ctx, key)
		if err != nil {
			return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph: tool %q: %v", key, err))
		}
		callable, resolvedKey, err := callableFromBizTool(ctx, row, r.lg)
		if err != nil {
			return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph: tool %q: %v", key, err))
		}
		out[resolvedKey] = callable
	}
	if len(out) == 0 {
		return nil, apierror.BadRequest(apierror.DomainGraph, "graph: at least one tool name required for tool nodes")
	}
	return out, nil
}

func callableFromBizTool(ctx context.Context, row biz.Tool, lg loggateway.Logger) (trpctool.Tool, string, error) {
	merged := mergeToolConfigJSON(row.ConfigJSON, row.DefaultConfigJSON, lg)
	asm, ok, _ := testexec.AssemblyForCatalogKey(row.Key, merged, nil, lg)
	if !ok {
		spec, ok := openAPISpecFromBizTool(row, lg)
		if !ok {
			return nil, "", apierror.Internal(apierror.DomainGraph, fmt.Sprintf("unsupported catalog tool %q", row.Key))
		}
		asm = tools.AssemblyConfig{
			EnabledTools: []string{"openapi"},
			OpenAPISpecs: []tools.OpenAPISpecConfig{spec},
			Lg:           lg,
		}
	}
	ts, err := tools.Assemble(ctx, asm)
	if err != nil {
		return nil, "", err
	}
	names := catalogToolNames(row.Key)
	for _, want := range names {
		if t, ok := matchCallable(ts.Tools, want); ok {
			return t, want, nil
		}
		for _, set := range ts.ToolSets {
			if set == nil {
				continue
			}
			list := set.Tools(ctx)
			if t, ok := matchCallable(list, want); ok {
				return t, want, nil
			}
		}
	}
	return nil, "", apierror.NotFound(apierror.DomainGraph, fmt.Sprintf("callable tool %q not found after assembly", row.Key))
}

func mergeToolConfigJSON(configJSON, defaultJSON string, lg loggateway.Logger) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(defaultJSON) != "" {
		if err := json.Unmarshal([]byte(defaultJSON), &out); err != nil {
			lg.Warn("default tool config json unmarshal failed", loggateway.Err(err))
		}
	}
	if strings.TrimSpace(configJSON) != "" {
		var overlay map[string]any
		if err := json.Unmarshal([]byte(configJSON), &overlay); err != nil {
			lg.Warn("tool config json unmarshal failed, using defaults", loggateway.Err(err))
		} else {
			for k, v := range overlay {
				out[k] = v
			}
		}
	}
	return out
}

func openAPISpecFromBizTool(row biz.Tool, lg loggateway.Logger) (tools.OpenAPISpecConfig, bool) {
	var meta map[string]any
	if strings.TrimSpace(row.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &meta); err != nil {
			lg.Warn("tool metadata json unmarshal failed", loggateway.Err(err), loggateway.Str("tool_key", row.Key))
		}
	}
	if spec, ok := meta["openapi_spec"].(map[string]any); ok {
		b, _ := json.Marshal(spec)
		return tools.OpenAPISpecConfig{Name: row.Key, SpecData: b}, true
	}
	return tools.OpenAPISpecConfig{}, false
}

func catalogToolNames(key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	return []string{key, strings.ReplaceAll(key, "-", "_"), strings.ReplaceAll(key, "_", "-")}
}

func matchCallable(toolsList []trpctool.Tool, name string) (trpctool.CallableTool, bool) {
	for _, t := range toolsList {
		decl := t.Declaration()
		if decl == nil {
			continue
		}
		if strings.TrimSpace(decl.Name) == name {
			if c, ok := t.(trpctool.CallableTool); ok {
				return c, true
			}
		}
	}
	return nil, false
}
