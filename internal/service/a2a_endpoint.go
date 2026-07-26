package service

import (
	"context"
	"io"
	"net/http"
	"strings"

	a2atrpc "aranea-agents/internal/a2a/trpc"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

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
	ag, err := s.orch.td().ReadDeps.Agents.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	biz.HydrateAgentKind(&ag)
	if biz.IsA2AProxyAgent(ag) {
		return nil, nil, biz.ErrNotFound
	}
	if s.orch.a2aUC() == nil {
		return nil, nil, biz.ErrNotFound
	}
	card, err := s.orch.a2aUC().GetAgentCard(ctx, agentID)
	if err != nil || !card.Enabled {
		return nil, nil, biz.ErrNotFound
	}

	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	deps := chatagent.TRPCBuilderDeps{
		TRPCModelCatalogDeps: chatagent.TRPCModelCatalogDeps{
			ModelCatalog: s.orch.td().ReadDeps.LLM,
			AgentUC:      s.orch.td().ReadDeps.AgentsUC,
			Agents:       s.orch.td().ReadDeps.Agents,
			Sys:          s.orch.td().ReadDeps.Settings,
			Sessions:     s.orch.td().Sessions,
		},
		TRPCModelRouteDeps: chatagent.TRPCModelRouteDeps{
			RT:       s.orch.td().RoundTrip(),
			Provider: prov,
			Model:    mod,
		},
		TRPCToolAssemblyDeps: chatagent.TRPCToolAssemblyDeps{
			ToolUC:       s.orch.td().ReadDeps.ToolUC,
			MCPTooling:   s.orch.td().Persist.AgentMCP,
			KanbanBridge: s.orch.rt().KanbanBridge,
		},
		TRPCMemoryKnowledgeDeps: chatagent.TRPCMemoryKnowledgeDeps{
			HasMemory:             s.orch.td().Persist.Memory.Available(),
			MemoryService:         s.orch.td().Persist.Memory.TRPC,
			MemoryAdmin:           s.orch.td().Persist.Memory.Admin,
			MemoryActionLogWriter: s.orch.td().Persist.Memory.ActionLogWriter,
			ManualCompressor:      biz.ManualCompressorFromNative(s.orch.td().Compress),
			MemoryL2Recall:        s.orch.td().Persist.Memory.L2Recall,
			MemoryL3Recall:        s.orch.td().Persist.Memory.L3Recall,
			MemoryCompositeRecall: s.orch.td().Persist.Memory.CompositeRecall,
			KnowledgeRetriever:    s.orch.rt().KnowledgeRetriever,
		},
		TRPCPluginDeps: chatagent.TRPCPluginDeps{
			PluginManager: s.orch.rt().PluginManager,
		},
		TRPCSkillDeps: chatagent.TRPCSkillDeps{
			SkillUC:         s.orch.td().ReadDeps.SkillUC,
			SkillDBRepo:     s.orch.rt().SkillDBRepo,
			CodeExecFactory: s.orch.rt().CodeExecFactory,
		},
		TRPCExtensionDeps: chatagent.TRPCExtensionDeps{
			Organization:     s.orch.rt().OrganizationUC,
			ToolResultGate:   s.orch.rt().ToolResultGate,
			SubAgentService:  s.orch.subAgentService(),
			OutboundRouter:   s.orch.outboundRouter(),
			A2AEnabled:       s.orch.a2aUC() != nil,
			L0SnapshotForcer: s.orch.td().SessionRT,
			LearningLoop:     s.orch.td().LearningLoop,
		},
	}
	// Inject CustomTools for built-in agents (Spirit, Skills Butler, Memory Butler, System Admin).
	// Without this, agents accessed via A2A would silently lack their core tools.
	deps.CustomTools = append(deps.CustomTools, s.orch.cliAdminTools(ctx, ag)...)
	deps.CustomTools = append(deps.CustomTools, s.orch.spiritCustomTools(ag)...)
	deps.CustomTools = append(deps.CustomTools, s.orch.skillsButlerTools(ctx, ag)...)
	deps.CustomTools = append(deps.CustomTools, s.orch.memoryButlerTools(ctx, ag)...)
	var plugins []trpcplugin.Plugin
	wsID := workspace.IDFromContext(ctx)
	if s.orch.rt().PluginManager != nil {
		plugins = s.orch.rt().PluginManager.RunnerPluginsForAgent(ag.ID, wsID)
	} else if s.orch.rt().PluginRT != nil {
		plugins = s.orch.rt().PluginRT.PluginsForAgent(ag.ID, wsID)
	}
	deps.Plugins = plugins

	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, s.orch.lg())
	if err != nil {
		return nil, nil, err
	}
	lookup := map[string]trpcagent.Agent{}
	if key := strings.TrimSpace(ag.AgentKey); key != "" {
		lookup[key] = root
	}
	rl := chatagent.ResolveRalphLoopTurn(ag.Settings)
	if rl.SkipErr != nil {
		s.lg.Warn("Ralph Loop 配置无效，已跳过",
			loggateway.StepID("a2a.runner.ralph_loop"), loggateway.Str("agent_id", ag.ID), loggateway.Err(rl.SkipErr))
	}
	runner, err := s.orch.tdPtr().CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:          plugins,
		BuilderDeps:      deps,
		AgentFactoryKeys: []string{ag.AgentKey},
		LookupAgents:     lookup,
		RalphLoop:        rl.Config,
		// P0-03 fix: unify memory scope with real agent ID.
		AppName: ag.ID,
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
