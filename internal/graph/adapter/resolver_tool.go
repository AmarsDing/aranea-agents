package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/testexec"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CatalogToolResolver resolves catalog tools by key for graph tool nodes.
type CatalogToolResolver struct {
	Tools *biz.ToolUsecase
}

var _ graphtrpc.ToolResolver = (*CatalogToolResolver)(nil)

func NewCatalogToolResolver(tools *biz.ToolUsecase) *CatalogToolResolver {
	return &CatalogToolResolver{Tools: tools}
}

func (r *CatalogToolResolver) ResolveTools(ctx context.Context, toolNames []string) (map[string]trpctool.Tool, error) {
	if r == nil || r.Tools == nil {
		return nil, fmt.Errorf("graph: tool catalog not configured")
	}
	out := make(map[string]trpctool.Tool)
	for _, name := range toolNames {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		row, err := r.Tools.GetTool(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("graph: tool %q: %w", key, err)
		}
		callable, resolvedKey, err := callableFromBizTool(ctx, row)
		if err != nil {
			return nil, fmt.Errorf("graph: tool %q: %w", key, err)
		}
		out[resolvedKey] = callable
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("graph: at least one tool name required for tool nodes")
	}
	return out, nil
}

func callableFromBizTool(ctx context.Context, row biz.Tool) (trpctool.Tool, string, error) {
	merged := mergeToolConfigJSON(row.ConfigJSON, row.DefaultConfigJSON)
	asm, ok := testexec.AssemblyForCatalogKey(row.Key, merged, nil)
	if !ok {
		spec, ok := openAPISpecFromBizTool(row)
		if !ok {
			return nil, "", fmt.Errorf("unsupported catalog tool %q", row.Key)
		}
		asm = tools.AssemblyConfig{
			EnabledTools: []string{"openapi"},
			OpenAPISpecs: []tools.OpenAPISpecConfig{spec},
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
	return nil, "", fmt.Errorf("callable tool %q not found after assembly", row.Key)
}

func mergeToolConfigJSON(configJSON, defaultJSON string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(defaultJSON) != "" {
		_ = json.Unmarshal([]byte(defaultJSON), &out)
	}
	if strings.TrimSpace(configJSON) != "" {
		var overlay map[string]any
		if json.Unmarshal([]byte(configJSON), &overlay) == nil {
			for k, v := range overlay {
				out[k] = v
			}
		}
	}
	return out
}

func openAPISpecFromBizTool(row biz.Tool) (tools.OpenAPISpecConfig, bool) {
	var meta map[string]any
	if strings.TrimSpace(row.MetadataJSON) != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON), &meta)
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
