package service

import (
	"context"
	"io"
	"net/http"
	"strings"

	a2atrpc "aranea-agents/internal/a2a/trpc"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type A2AEndpointBuilder struct {
	factory biz.A2ARunnerFactory
}

func NewA2AEndpointBuilder(factory biz.A2ARunnerFactory) *A2AEndpointBuilder {
	return &A2AEndpointBuilder{factory: factory}
}

func (b *A2AEndpointBuilder) BuildHandler(ctx context.Context, agentID, publicURL string) (http.Handler, func(), error) {
	if b == nil || b.factory == nil {
		return nil, nil, biz.ErrNotFound
	}
	closer, handler, err := b.factory.BuildA2ARunner(ctx, agentID, publicURL)
	if err != nil {
		return nil, nil, err
	}
	return handler, func() { closer.Close() }, nil
}

var _ a2atrpc.EndpointBuilder = (*A2AEndpointBuilder)(nil)

func (s *ChatService) BuildA2ARunner(ctx context.Context, agentID, publicURL string) (io.Closer, http.Handler, error) {
	if s == nil || s.orch == nil {
		return nil, nil, biz.ErrNotFound
	}
	ag, err := s.orch.td.Catalog.Agents.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	biz.HydrateAgentKind(&ag)
	if biz.IsA2AProxyAgent(ag) {
		return nil, nil, biz.ErrNotFound
	}
	if s.orch.a2aUC == nil {
		return nil, nil, biz.ErrNotFound
	}
	card, err := s.orch.a2aUC.GetAgentCard(ctx, agentID)
	if err != nil || !card.Enabled {
		return nil, nil, biz.ErrNotFound
	}

	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	deps := chatagent.TRPCBuilderDeps{
		Catalog:                s.orch.td.Catalog.LLM,
		AgentUC:                s.orch.td.Catalog.AgentsUC,
		Agents:                 s.orch.td.Catalog.Agents,
		RT:                     s.orch.td.RoundTrip(),
		SkillUC:                s.orch.td.Catalog.SkillUC,
		MCPTooling:             s.orch.td.Persist.AgentMCP,
		ToolUC:                 s.orch.td.Catalog.ToolUC,
		Sessions:               s.orch.td.Sessions,
		Sys:                    s.orch.td.Catalog.Settings,
		Provider:               prov,
		Model:                  mod,
		SkillDBRepo:            s.orch.rt.SkillDBRepo,
		HasMemory:              s.orch.td.Persist.Memory.Available(),
		PluginManager:          s.orch.rt.PluginManager,
		MemoryAdmin:            s.orch.td.Persist.Memory.Admin,
		MemoryL2Recall:         s.orch.td.Persist.Memory.L2Recall,
		MemoryL3Recall:         s.orch.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall:  s.orch.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever:     s.orch.rt.KnowledgeRetriever,
		CodeExecFactory:        s.orch.rt.CodeExecFactory,
		KanbanBridge:           s.orch.rt.KanbanBridge,
		IndustryUC:             s.orch.rt.IndustryUC,
		DepartmentUC:           s.orch.rt.DepartmentUC,
		PositionUC:             s.orch.rt.PositionUC,
	}
	var plugins []trpcplugin.Plugin
	if s.orch.rt.PluginManager != nil {
		plugins = s.orch.rt.PluginManager.RunnerPluginsForAgent(ag.ID)
	} else if s.orch.rt.PluginRT != nil {
		plugins = s.orch.rt.PluginRT.PluginsForAgent(ag.ID)
	}
	deps.Plugins = plugins

	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps)
	if err != nil {
		return nil, nil, err
	}
	lookup := map[string]trpcagent.Agent{}
	if key := strings.TrimSpace(ag.AgentKey); key != "" {
		lookup[key] = root
	}
	rl := chatagent.ResolveRalphLoopTurn(ag.Settings)
	if rl.SkipErr != nil {
		event.CtxFlowLogWarn(ctx, "a2a.runner.ralph_loop", "Ralph Loop 配置无效，已跳过",
			event.P("agent_id", ag.ID), event.P("error", rl.SkipErr.Error()))
	}
	runner, err := s.orch.td.CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:          plugins,
		BuilderDeps:      deps,
		AgentFactoryKeys: []string{ag.AgentKey},
		LookupAgents:     lookup,
		RalphLoop:        rl.Config,
	})
	if err != nil {
		return nil, nil, err
	}
	streaming := true
	handler, err := a2atrpc.BuildA2AEndpointServer(runner, ag, card, publicURL, streaming)
	if err != nil {
		runner.Close()
		return nil, nil, err
	}
	return runner, handler, nil
}

var _ biz.A2ARunnerFactory = (*ChatService)(nil)
