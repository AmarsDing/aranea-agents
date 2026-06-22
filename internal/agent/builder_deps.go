package agent

import (
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools/deferred"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	subagenttool "aranea-agents/internal/tools/subagent"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TRPCModelCatalogDeps documents model-catalog/repo dependencies on TRPCBuilderDeps.
type TRPCModelCatalogDeps struct {
	ModelCatalog biz.TeamModelCatalog
	AgentUC      biz.TeamAgentLookup
	Agents       biz.AgentRepository
	Sys          biz.SystemSettingRepo
	Sessions     biz.SessionTurnManager
}

// TRPCModelRouteDeps documents provider/model routing on TRPCBuilderDeps.
type TRPCModelRouteDeps struct {
	RT         *provider.RoundTrip
	Provider   string
	Model      string
	DialogMode string
}

// TRPCToolAssemblyDeps documents tool/MCP assembly on TRPCBuilderDeps.
type TRPCToolAssemblyDeps struct {
	ToolUC       biz.TeamToolLookup
	MCPTooling   *biz.AgentMCPTooling
	AwaitHook    tooltrpc.ReplyFunc
	CustomTools  []trpctool.Tool
	KanbanBridge kanbanpkg.Bridge
	// CachedEffectiveTools carries a pre-fetched effective-tools result so
	// that buildToolsetsForAgent can skip its own GetEffectiveTools call.
	// When nil, buildToolsetsForAgent falls back to fetching it itself.
	// IMPORTANT: must belong to the current agent (same agentID used in
	// the surrounding build context); passing a stale or mismatched result
	// will produce incorrect tool keys.
	CachedEffectiveTools *biz.AgentEffectiveTools
}

// TRPCMemoryKnowledgeDeps documents memory/knowledge ports on TRPCBuilderDeps.
type TRPCMemoryKnowledgeDeps struct {
	HasMemory             bool
	MemoryService         trpcmemory.Service
	MemoryAdmin           biz.SessionAdminStore
	MemoryL2Recall        biz.MemoryL2Recaller
	MemoryL3Recall        biz.MemoryL3Recaller
	MemoryCompositeRecall biz.MemoryCompositeRecaller
	KnowledgeRetriever    *knowledge.Retriever
	KnowledgeUsecase      *biz.KnowledgeUsecase
}

// TRPCPluginDeps documents plugin/callback chain on TRPCBuilderDeps.
type TRPCPluginDeps struct {
	Plugins       []trpcplugin.Plugin
	PluginManager *plugintrpc.Manager
}

// TRPCSkillDeps documents skill resolution on TRPCBuilderDeps.
type TRPCSkillDeps struct {
	SkillUC         biz.TeamSkillLookup
	SkillDBRepo     trpcskill.Repository
	CodeExecFactory *localexec.Factory
}

// TRPCExtensionDeps documents cross-cutting / optional extensions on TRPCBuilderDeps.
type TRPCExtensionDeps struct {
	// PGO-1: Taxonomy is used to resolve the 岗位职责 (position description)
	// from industry_taxonomy for injection into the system instruction.
	// Optional: when nil, category responsibility injection is skipped.
	Organization   *biz.OrganizationUsecase
	ToolResultGate *biz.ToolResultGate
	// DeferredManager controls lazy tool visibility. Optional: when nil,
	// deferred tool filtering is skipped and all tools are always visible.
	DeferredManager *deferred.DeferredToolManager
	// CircuitBreakerRegistry exposes per-tool circuit breakers for admin reset.
	// Optional: when nil, circuit breaker state is not accessible externally.
	CircuitBreakerRegistry *biztool.CircuitBreakerRegistry
	LG                     loggateway.Logger
	// Cache version hashes: optional strings computed by the caller.
	// When non-empty they are folded into the build cache fingerprint so that
	// tool / skill / MCP changes invalidate the cached agent.
	ToolVersionHash  string
	SkillVersionHash string
	MCPVersionHash   string
	// OutboundRouter enables the message tool for proactive outbound messaging.
	// Optional: when nil, the message tool is unavailable.
	OutboundRouter *outbound.Router
	// SubAgentService enables subagent spawn/list/get/cancel tools.
	// Optional: when nil, subagent tools are unavailable.
	SubAgentService *subagenttool.Service
	// L0SnapshotForcer allows the compression pipeline to signal that the next
	// L0 snapshot write should bypass throttling. Optional: when nil, force
	// flags are ignored and normal throttle rules apply.
	L0SnapshotForcer biz.L0SnapshotForcer
}

// TRPCBuilderDeps is the stable extension DTO for BuildTRPCLLMAgent / BuildTRPCAgent.
// Fields are grouped into cohesive sub-dependency structs (AS-COG-01 compliance).
// All embedded fields are promoted, so d.ModelCatalog works the same as before.
type TRPCBuilderDeps struct {
	TRPCModelCatalogDeps
	TRPCModelRouteDeps
	TRPCToolAssemblyDeps
	TRPCMemoryKnowledgeDeps
	TRPCPluginDeps
	TRPCSkillDeps
	TRPCExtensionDeps
}

// ModelCatalogGroup returns the model-catalog subset (for tests and future refactors).
func (d TRPCBuilderDeps) ModelCatalogGroup() TRPCModelCatalogDeps {
	return d.TRPCModelCatalogDeps
}

func (d TRPCBuilderDeps) Logger() loggateway.Logger {
	if d.LG != nil {
		return d.LG
	}
	return loggateway.NewNoop()
}

// WithDeferredManager returns a copy of deps with DeferredManager set.
// Use this instead of mutating deps in-place to avoid side effects on
// the caller's copy.
func (d TRPCBuilderDeps) WithDeferredManager(dm *deferred.DeferredToolManager) TRPCBuilderDeps {
	d.DeferredManager = dm
	return d
}

// WithCircuitBreakerRegistry returns a copy of deps with CircuitBreakerRegistry set.
func (d TRPCBuilderDeps) WithCircuitBreakerRegistry(r *biztool.CircuitBreakerRegistry) TRPCBuilderDeps {
	d.CircuitBreakerRegistry = r
	return d
}
