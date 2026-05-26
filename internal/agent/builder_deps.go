package agent

import (
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	tooltrpc "aranea-agents/internal/tools/trpc"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
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
	ToolUC     *biz.ToolUsecase
	MCPTooling *biz.AgentMCPTooling
	AwaitHook  tooltrpc.ReplyFunc
}

// TRPCMemoryKnowledgeDeps documents memory/knowledge ports on TRPCBuilderDeps.
type TRPCMemoryKnowledgeDeps struct {
	HasMemory      bool
	MemoryAdmin    biz.SessionAdminStore
	MemoryL2Recall biz.MemoryL2Recaller
	MemoryL3Recall biz.MemoryL3Recaller
	MemoryCompositeRecall biz.MemoryCompositeRecaller
	KnowledgeRetriever *knowledge.Retriever
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
	ToolUC     *biz.ToolUsecase
	MCPTooling *biz.AgentMCPTooling
	AwaitHook  tooltrpc.ReplyFunc
	// TRPCMemoryKnowledgeDeps
	HasMemory          bool
	MemoryAdmin        biz.SessionAdminStore
	MemoryL2Recall     biz.MemoryL2Recaller
	MemoryL3Recall     biz.MemoryL3Recaller
	MemoryCompositeRecall biz.MemoryCompositeRecaller
	KnowledgeRetriever *knowledge.Retriever
	// TRPCPluginDeps
	Plugins       []trpcplugin.Plugin
	PluginManager *plugintrpc.Manager
	// TRPCSkillDeps
	SkillUC         *biz.SkillUsecase
	SkillDBRepo     trpcskill.Repository
	CodeExecFactory *localexec.Factory
}

// CatalogGroup returns the catalog subset (for tests and future refactors).
func (d TRPCBuilderDeps) CatalogGroup() TRPCCatalogDeps {
	return TRPCCatalogDeps{
		Catalog: d.Catalog, AgentUC: d.AgentUC, Agents: d.Agents, Sys: d.Sys, Sessions: d.Sessions,
	}
}
