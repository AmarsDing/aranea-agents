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
	"aranea-agents/internal/sandbox"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/calendar"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/codingbridge"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	"aranea-agents/internal/tools/mail"
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
	// Credential 是 E 阶段凭据托管用例（credential_login_execute /
	// credential_token_issue 的 vault 裁决/解密封口）。nil 时凭据工具被裁剪。
	Credential *biz.CredentialUsecase
	// Calendar 是 E 阶段 W1c 日程工具窄口（calendar_list /
	// calendar_create_draft；service 层 session→user 解析适配）。nil 时日程
	// 工具被裁剪（诚实降级）。
	Calendar calendar.Service
	// Mail 是 E 阶段 W1d 邮件工具窄口（mail_list / mail_read / mail_search /
	// mail_to_task；service 层 session→user 解析适配）。nil 时邮件工具被裁剪。
	Mail mail.Service
	// SandboxFS is the process-wide shared session-lease store backing
	// sandbox_fs_write/read (M82 P1-2); the same instance is bound to the
	// codeexecutor sandbox backend so code_exec and sandbox_fs share one
	// sandbox per session. nil prunes the toolset.
	SandboxFS *sandbox.SessionLeases
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
