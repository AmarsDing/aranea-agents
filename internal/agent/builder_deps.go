package agent

import (
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools/deferred"
	subagenttool "aranea-agents/internal/tools/subagent"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TRPCCatalogDeps documents catalog/repo dependencies on TRPCBuilderDeps.
type TRPCCatalogDeps struct {
	Catalog  *biz.LlmProviderModelUsecase
	AgentUC  *biz.AgentUsecase
	Agents   biz.AgentRepository
	Sys      biz.SystemSettingRepo
	Sessions *biz.SessionUsecase
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
	ToolUC      *biz.ToolUsecase
	MCPTooling  *biz.AgentMCPTooling
	AwaitHook   tooltrpc.ReplyFunc
	CustomTools []trpctool.Tool
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
	SkillUC         *biz.SkillUsecase
	SkillDBRepo     trpcskill.Repository
	CodeExecFactory *localexec.Factory
}

// TRPCBuilderDeps is the stable extension DTO for BuildTRPCLLMAgent / BuildTRPCAgent.
// Field groups match TRPC*Deps types above; composite literals stay flat for Wire/service call sites.
type TRPCBuilderDeps struct {
	// TRPCCatalogDeps
	Catalog  *biz.LlmProviderModelUsecase
	AgentUC  *biz.AgentUsecase
	Agents   biz.AgentRepository
	Sys      biz.SystemSettingRepo
	Sessions *biz.SessionUsecase
	// TRPCModelRouteDeps
	RT         *provider.RoundTrip
	Provider   string
	Model      string
	DialogMode string
	// TRPCToolAssemblyDeps
	ToolUC       *biz.ToolUsecase
	MCPTooling   *biz.AgentMCPTooling
	AwaitHook    tooltrpc.ReplyFunc
	CustomTools  []trpctool.Tool
	KanbanBridge kanbanpkg.Bridge
	// TRPCMemoryKnowledgeDeps
	HasMemory             bool
	MemoryService         trpcmemory.Service
	MemoryAdmin           biz.SessionAdminStore
	MemoryL2Recall        biz.MemoryL2Recaller
	MemoryL3Recall        biz.MemoryL3Recaller
	MemoryCompositeRecall biz.MemoryCompositeRecaller
	KnowledgeRetriever    *knowledge.Retriever
	KnowledgeUsecase      *biz.KnowledgeUsecase
	// TRPCPluginDeps
	Plugins       []trpcplugin.Plugin
	PluginManager *plugintrpc.Manager
	// TRPCSkillDeps
	SkillUC         *biz.SkillUsecase
	SkillDBRepo     trpcskill.Repository
	CodeExecFactory *localexec.Factory
	// PGO-1: Taxonomy is used to resolve the 岗位职责 (position description)
	// from industry_taxonomy for injection into the system instruction.
	// Optional: when nil, category responsibility injection is skipped.
	Taxonomy *biz.TaxonomyUsecase
	ToolResultGate *biz.ToolResultGate
	// DeferredManager controls lazy tool visibility. Optional: when nil,
	// deferred tool filtering is skipped and all tools are always visible.
	DeferredManager *deferred.DeferredToolManager
	// CircuitBreakerRegistry exposes per-tool circuit breakers for admin reset.
	// Optional: when nil, circuit breaker state is not accessible externally.
	CircuitBreakerRegistry *biztool.CircuitBreakerRegistry
	LG loggateway.Logger
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
}

// CatalogGroup returns the catalog subset (for tests and future refactors).
func (d TRPCBuilderDeps) CatalogGroup() TRPCCatalogDeps {
	return TRPCCatalogDeps{
		Catalog: d.Catalog, AgentUC: d.AgentUC, Agents: d.Agents, Sys: d.Sys, Sessions: d.Sessions,
	}
}

func (d TRPCBuilderDeps) Logger() loggateway.Logger {
	if d.LG != nil {
		return d.LG
	}
	return loggateway.NewNoop()
}
