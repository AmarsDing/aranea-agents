package team

import (
	"context"

	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/graph"
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	subagenttool "aranea-agents/internal/tools/subagent"
	tooltrpc "aranea-agents/internal/tools/trpc"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
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

// SessionChildLookup resolves member agent session IDs for team members.
// Used by the team runner to set child_session_id in session activities
// so the frontend can lazy-load member execution processes (thinking/action/reply).
// When nil, child_session_id falls back to the team session ID.
type SessionChildLookup interface {
	LookupChildSessionID(ctx context.Context, parentSessionID, memberAgentKey string) (string, error)
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
	// ComputerUseUC enables the computer_use_* toolset in team member builds
	// (75-computer-use). Optional; nil prunes the toolset.
	ComputerUseUC *bizcu.ComputerUseUsecase
	A2AEnabled    bool
	// SessionChildLookup resolves member agent session IDs for child_session_id
	// in session activities. Optional; when nil, falls back to team session ID.
	SessionChildLookup SessionChildLookup
	// MemberCustomTools injects per-member custom tools (e.g. cli_admin_* for
	// __system_admin__) into team member builds. Without this hook, agent-specific
	// tools that require live deps are only assembled on the direct chat path —
	// a system_admin member inside a team would not see cli_admin_* in its LLM
	// tool list and would hallucinate substitute tools.
	// Optional; when nil, members get only registry tools + deliverable tools.
	MemberCustomTools func(ctx context.Context, ag biz.Agent) []trpctool.Tool
	// GraphEnsurer 运行时惰性物化端口（B10）：加载 team 时确保图资产存在
	// （linked_graph_id 为空 → 先物化）。生产装配 *biz.TeamUsecase；
	// nil 时回退直接读 TeamReader（单测/离线工具）。
	GraphEnsurer biz.TeamGraphAssetEnsurer
	// Replanner 是 G2 智能重规划兜底（ADR-F）：节点静态恢复（fallback_agent /
	// on_failure=skip）未覆盖或失败时，全局 replanner AfterNode 决策落地
	// （Reflexion 重试 / reroute→skip / insert_fallback→HITL）。可选；nil 时
	// team 图执行保持纯静态路径（现状行为）。
	Replanner graph.RuntimeReplanner
}
