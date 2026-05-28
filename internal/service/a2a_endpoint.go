package service

import (
	"context"
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

// A2AEndpointBuilder builds cached A2A HTTP handlers for enabled catalog agents.
type A2AEndpointBuilder struct {
	chat   *ChatService
	a2aUC  *biz.A2AUsecase
	agents biz.AgentRepository
}

// NewA2AEndpointBuilder constructs the builder used by the public A2A registry.
func NewA2AEndpointBuilder(chat *ChatService, a2aUC *biz.A2AUsecase, agents biz.AgentRepository) *A2AEndpointBuilder {
	return &A2AEndpointBuilder{chat: chat, a2aUC: a2aUC, agents: agents}
}

// BuildHandler implements trpc.EndpointBuilder.
func (b *A2AEndpointBuilder) BuildHandler(ctx context.Context, agentID, publicURL string) (http.Handler, func(), error) {
	if b == nil || b.chat == nil || b.agents == nil {
		return nil, nil, biz.ErrNotFound
	}
	ag, err := b.agents.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	biz.HydrateAgentKind(&ag)
	if biz.IsA2AProxyAgent(ag) {
		return nil, nil, biz.ErrNotFound
	}
	card, err := b.a2aUC.GetAgentCard(ctx, agentID)
	if err != nil || !card.Enabled {
		return nil, nil, biz.ErrNotFound
	}

	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	deps := chatagent.TRPCBuilderDeps{
		Catalog:            b.chat.orch.td.Catalog.LLM,
		AgentUC:            b.chat.orch.td.Catalog.AgentsUC,
		Agents:             b.chat.orch.td.Catalog.Agents,
		RT:                 b.chat.orch.td.RoundTrip(),
		SkillUC:            b.chat.orch.td.Catalog.SkillUC,
		MCPTooling:         b.chat.orch.td.Persist.AgentMCP,
		ToolUC:             b.chat.orch.td.Catalog.ToolUC,
		Sessions:           b.chat.orch.td.Sessions,
		Sys:                b.chat.orch.td.Catalog.Settings,
		Provider:           prov,
		Model:              mod,
		SkillDBRepo:        b.chat.orch.rt.SkillDBRepo,
		HasMemory:          b.chat.orch.td.Persist.Memory.Available(),
		PluginManager:      b.chat.orch.rt.PluginManager,
		MemoryAdmin:        b.chat.orch.td.Persist.Memory.Admin,
		MemoryL2Recall:        b.chat.orch.td.Persist.Memory.L2Recall,
		MemoryL3Recall:        b.chat.orch.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall: b.chat.orch.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever: b.chat.orch.rt.KnowledgeRetriever,
		CodeExecFactory:    b.chat.orch.rt.CodeExecFactory,
		KanbanBridge:       b.chat.orch.rt.KanbanBridge,
	}
	var plugins []trpcplugin.Plugin
	if b.chat.orch.rt.PluginManager != nil {
		plugins = b.chat.orch.rt.PluginManager.RunnerPluginsForAgent(ag.ID)
	} else if b.chat.orch.rt.PluginRT != nil {
		plugins = b.chat.orch.rt.PluginRT.PluginsForAgent(ag.ID)
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
	runner, err := b.chat.orch.td.CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
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
	closeFn := func() { runner.Close() }
	return handler, closeFn, nil
}

var _ a2atrpc.EndpointBuilder = (*A2AEndpointBuilder)(nil)
