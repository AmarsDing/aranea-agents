package team

import (
	"context"

	"aranea-agents/internal/biz"
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	subagenttool "aranea-agents/internal/tools/subagent"
	tooltrpc "aranea-agents/internal/tools/trpc"
)

// KnowledgeFacade groups knowledge subsystem pointers used by the team Runner.
// TECH-DEBT(COG): concrete_deps=4, limit=0; fields are still concrete types; extract narrow interfaces once
// knowledge tool context injection is refactored to accept interfaces.
type KnowledgeFacade struct {
	Retriever          *knowledge.Retriever
	Router             *knowledge.AdaptiveRouter
	FederatedRetriever *knowledge.FederatedRetriever
	Evaluator          *knowledge.RetrievalEvaluator
}

type RunnerConfig struct {
	GraphLoader       GraphBuildConfigLoader
	AwaitHookProvider func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc
	Knowledge         *KnowledgeFacade
	KnowledgeUsecase  *biz.KnowledgeUsecase
	StreamOptsFactory StreamOptsFactory
	AgentHelper       biz.TeamAgentHelper
	Runs              *rt.RunRegistry
	GraphRoot         graphadapter.TeamGraphRootBuilder
	// TECH-DEBT(COG): concrete_deps=2, limit=0; PluginRT and PluginManager are still concrete types; extract
	// narrow interfaces once plugin/trpc API surface is stabilized.
	PluginRT      *plugintrpc.Runtime
	PluginManager *plugintrpc.Manager
	// Runtime extensions previously only injected for single-agent chat turns.
	// These are threaded into member-agent builds so team agents have the same
	// capability surface as chat agents (subagent, outbound, tool-result gate,
	// organization taxonomy, kanban, A2A call_agent).
	OrganizationUC  *biz.OrganizationUsecase
	ToolResultGate  *biz.ToolResultGate
	OutboundRouter  *outbound.Router
	SubAgentService *subagenttool.Service
	KanbanBridge    kanbanpkg.Bridge
	A2AEnabled      bool
}
