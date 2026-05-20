package service

import (
	"context"
	"net/http"
	"strings"

	chatagent "aranea-agents/internal/agent"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"

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
		Catalog:            b.chat.td.Catalog.LLM,
		AgentUC:            b.chat.td.Catalog.AgentsUC,
		Agents:             b.chat.td.Catalog.Agents,
		RT:                 b.chat.td.RoundTrip(),
		SkillUC:            b.chat.td.Catalog.SkillUC,
		MCPTooling:         b.chat.td.Persist.AgentMCP,
		ToolUC:             b.chat.td.Catalog.ToolUC,
		Sessions:           b.chat.td.Sessions,
		Sys:                b.chat.td.Catalog.Settings,
		Provider:           prov,
		Model:              mod,
		SkillDBRepo:        b.chat.skillDBRepo,
		HasMemory:          b.chat.td.Persist.Memory.Available(),
		PluginManager:      b.chat.pluginManager,
		MemoryAdmin:        b.chat.td.Persist.Memory.Admin,
		KnowledgeRetriever: b.chat.knowledgeRetriever,
	}
	var plugins []trpcplugin.Plugin
	if b.chat.pluginManager != nil {
		plugins = b.chat.pluginManager.RunnerPluginsForAgent(ag.ID)
	} else if b.chat.pluginRT != nil {
		plugins = b.chat.pluginRT.PluginsForAgent(ag.ID)
	}
	deps.Plugins = plugins

	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps)
	if err != nil {
		return nil, nil, err
	}
	if b.chat.td.RunnerMgr == nil {
		b.chat.td.RunnerMgr = rt.NewRunnerManagerFromPersist(b.chat.td.Persist)
	}
	runner, err := b.chat.td.RunnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:          plugins,
		BuilderDeps:      deps,
		AgentFactoryKeys: []string{ag.AgentKey},
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
