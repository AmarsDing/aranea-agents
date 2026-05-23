package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/testexec"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CatalogModelResolver resolves models via LlmProviderModelUsecase.
type CatalogModelResolver struct {
	Catalog *biz.LlmProviderModelUsecase
	RT      *provider.RoundTrip
}

var _ graphtrpc.ModelResolver = (*CatalogModelResolver)(nil)

func NewCatalogModelResolver(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip) *CatalogModelResolver {
	if rt == nil {
		rt = &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	}
	return &CatalogModelResolver{Catalog: catalog, RT: rt}
}

func (r *CatalogModelResolver) ResolveModel(ctx context.Context, modelName string) (trpcmodel.Model, error) {
	if r == nil || r.Catalog == nil {
		return nil, fmt.Errorf("graph: model catalog not configured")
	}
	prov, api, err := parseModelRef(ctx, r.Catalog, modelName)
	if err != nil {
		return nil, err
	}
	return provider.TRPCModelForProviderModel(ctx, r.Catalog, r.RT, prov, api)
}

func parseModelRef(ctx context.Context, catalog *biz.LlmProviderModelUsecase, modelName string) (prov, api string, err error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", "", fmt.Errorf("graph: model_name is required for LLM nodes")
	}
	for _, sep := range []string{"/", "|", ":"} {
		if i := strings.Index(modelName, sep); i > 0 {
			return strings.TrimSpace(modelName[:i]), strings.TrimSpace(modelName[i+1:]), nil
		}
	}
	api = modelName
	rows, listErr := catalog.List(ctx)
	if listErr != nil {
		return "", api, nil
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Model), api) {
			return strings.TrimSpace(row.Provider), api, nil
		}
	}
	return "", api, nil
}

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

// CatalogAgentResolver builds catalog agents for graph agent nodes.
type CatalogAgentResolver struct {
	Deps chatagent.TRPCBuilderDeps
}

var _ graphtrpc.AgentResolver = (*CatalogAgentResolver)(nil)

func NewCatalogAgentResolver(deps chatagent.TRPCBuilderDeps) *CatalogAgentResolver {
	return &CatalogAgentResolver{Deps: deps}
}

func (r *CatalogAgentResolver) ResolveAgent(ctx context.Context, agentRef string) (trpcagent.Agent, error) {
	if r == nil {
		return nil, fmt.Errorf("graph: agent catalog not configured")
	}
	ag, err := r.resolveBizAgent(ctx, agentRef)
	if err != nil {
		return nil, err
	}
	return chatagent.BuildTRPCAgentCached(ctx, ag, r.Deps)
}

func (r *CatalogAgentResolver) resolveBizAgent(ctx context.Context, agentRef string) (biz.Agent, error) {
	ref := strings.TrimSpace(agentRef)
	if ref == "" {
		return biz.Agent{}, fmt.Errorf("graph: agent ref is required")
	}
	if r.Deps.AgentUC != nil {
		if ag, err := r.Deps.AgentUC.Get(ctx, ref); err == nil {
			return ag, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return biz.Agent{}, err
		}
	}
	if r.Deps.Agents != nil {
		if ag, err := r.Deps.Agents.GetAgentByAgentKey(ctx, ref); err == nil {
			if r.Deps.AgentUC != nil {
				return r.Deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return biz.Agent{}, err
		}
		if ag, err := r.Deps.Agents.GetAgentByID(ctx, ref); err == nil {
			if r.Deps.AgentUC != nil {
				return r.Deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return biz.Agent{}, err
		}
	}
	return biz.Agent{}, fmt.Errorf("graph: agent %q not found", ref)
}
