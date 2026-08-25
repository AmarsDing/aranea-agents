package service

import (
	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/debug"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/codingbridge"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	"aranea-agents/internal/tools/skillrecommend"
	subagenttool "aranea-agents/internal/tools/subagent"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// RuntimeTooling is a thin grouping of per-turn tooling domains injected into
// every agent turn build. Nested groups follow real co-injection / co-nil-check
// clusters (AS-COG-01): this struct has 6 fields; each nested group has ≤6.
type RuntimeTooling struct {
	Knowledge  KnowledgeTools
	Skill      SkillRuntime
	Plugin     PluginRuntime
	Bridges    ToolBridges
	Sharing    WorkspaceSharing
	Extensions TurnExtensions
}

// KnowledgeTools is the knowledge retrieval stack. Retriever/Router/Federated/
// Evaluator are injected into run context together; Usecase is assembled into
// TRPCMemoryKnowledgeDeps. Each field is independently optional (nil = skip).
type KnowledgeTools struct {
	Retriever          *knowledge.Retriever
	Router             *knowledge.AdaptiveRouter
	FederatedRetriever *knowledge.FederatedRetriever
	Evaluator          *knowledge.RetrievalEvaluator
	Usecase            *biz.KnowledgeUsecase
}

// SkillRuntime is skill catalog, historical ranking, and sandbox code execution
// assembled together into TRPCSkillDeps.
type SkillRuntime struct {
	DBRepo          trpcskill.Repository
	Health          *SkillHealthMetricsAdapter
	CodeExecFactory *localexec.Factory
}

// healthProvider normalizes the adapter to the interface consumed by
// TRPCSkillDeps, mapping a nil adapter to a nil interface so the
// `opts.HealthProvider != nil` guard in ResolveSkillSlugsDetailed is not
// defeated by a typed-nil.
func (s SkillRuntime) healthProvider() skillrecommend.HealthMetricsProvider {
	if s.Health == nil {
		return nil
	}
	return s.Health
}

// PluginRuntime is plugin manager with PluginRT fallback for runner plugins.
// Callers prefer Manager; RT is the legacy fallback when Manager is nil.
type PluginRuntime struct {
	RT      *plugintrpc.Runtime
	Manager *plugintrpc.Manager
}

// ToolBridges are optional ToolSets pruned when the corresponding field is nil
// (kanban / computer_use / coding_* / client_open_*).
type ToolBridges struct {
	Kanban      kanbanpkg.Bridge
	ComputerUse *bizcu.ComputerUseUsecase
	Coding      codingbridge.BridgeService
	Client      *clientbridge.Bridge
}

// WorkspaceSharing is M71 agent resource sharing, assembled via CustomToolFunc
// (memberfs + deptmail for dept leads; sessionaccess for spirit).
type WorkspaceSharing struct {
	ResourceAccess *biz.ResourceAccessUsecase
	DeptMailbox    *biz.DeptMailboxUsecase
	SessionSearch  *biz.SessionSearchUsecase
}

// TurnExtensions are turn-safety and collaboration extras that are not
// themselves toolsets: org context, confirmation gate, outbound routing,
// sub-agent lifecycle, plus Wire-held debug/parallel executors.
type TurnExtensions struct {
	Organization         *biz.OrganizationUsecase
	ToolResultGate       *biz.ToolResultGate
	OutboundRouter       *outbound.Router
	SubAgentService      *subagenttool.Service
	DebugRecorder        *debug.RecorderFactory
	ParallelToolExecutor *tools.ParallelToolExecutor
	// ToolResultPrune 是 R2 确定性剪枝 hook 的消费侧配置（79-runtime-governance），
	// wire 期由 conf.Runtime.ToolResultPruneConfig() 翻译一次，各 turn 构建点透传。
	ToolResultPrune chatagent.ToolResultPruneConfig
}
