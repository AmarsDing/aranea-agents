package agent

import (
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	bizmedia "aranea-agents/internal/biz/media"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/cache"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/codingbridge"
	"aranea-agents/internal/tools/deferred"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	"aranea-agents/internal/tools/skillrecommend"
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
	// MediaProviders resolves media generation provider configs for the
	// generate_image / generate_video / image_to_video tools. Optional: when
	// nil, media tools are skipped even if enabled in effective tool keys.
	MediaProviders bizmedia.ProviderReader
	// ArtifactWriter persists generated media into the artifact store
	// (PersistingProvider). Optional: when nil, media results keep their
	// original remote URLs (best-effort degrade).
	ArtifactWriter biz.ArtifactSaver
	// CachedEffectiveTools carries a pre-fetched effective-tools result so
	// that buildToolsetsForAgent can skip its own GetEffectiveTools call.
	// When nil, buildToolsetsForAgent falls back to fetching it itself.
	// IMPORTANT: must belong to the current agent (same agentID used in
	// the surrounding build context); passing a stale or mismatched result
	// will produce incorrect tool keys.
	CachedEffectiveTools *biz.AgentEffectiveTools
	// ComputerUseUC enables the computer_use_* desktop automation toolset
	// (75-computer-use). Optional: when nil, computer-use tools are pruned
	// from assembly even if enabled in effective tool keys.
	ComputerUseUC *bizcu.ComputerUseUsecase
	// CodingBridgeSvc enables the coding agent bridge ToolSet (coding_dispatch_task /
	// coding_check_task / coding_cancel_task). Optional: when nil, coding tools are
	// pruned from assembly even if enabled in effective tool keys (76-coding-agent-bridge).
	CodingBridgeSvc codingbridge.BridgeService
	// MCPCacheInvalidators expire mcp_tool_set tools/list caches mid-turn
	// (E8). Optional: when empty, catalog refresh is a no-op.
	MCPCacheInvalidators []tools.MCPCacheInvalidator
}

// TRPCMemoryKnowledgeDeps documents memory/knowledge ports on TRPCBuilderDeps.
type TRPCMemoryKnowledgeDeps struct {
	HasMemory     bool
	MemoryService trpcmemory.Service
	// MemoryLayerPorts holds independent L0–L4 persistence ports (ISP).
	// Replaces the deprecated SessionAdminStore MemoryAdmin field.
	biz.MemoryLayerPorts
	MemoryActionLogWriter biz.MemoryActionLogWriter
	MemoryL2Recall        biz.MemoryL2Recaller
	MemoryL3Recall        biz.MemoryL3Recaller
	MemoryCompositeRecall biz.MemoryCompositeRecaller
	// MemoryPreferenceLister feeds the pinned preference/constraint block
	// (FR-M3). Optional: when nil, pinned injection is skipped.
	MemoryPreferenceLister biz.MemoryPreferenceLister
	// MemoryProfileCardReader feeds the resident profile card block (FR-12.7):
	// one distilled card per (agent, user), injected unconditionally at the
	// first memory-block position when L3 injection is enabled. Optional:
	// when nil, the card block is skipped.
	MemoryProfileCardReader biz.MemoryProfileCardReader
	// MemoryFactInjectCounter bumps injected_count for the L3 facts actually
	// written into the prompt (FR-12.6 three-stage counters). Optional: when
	// nil, injected counting is skipped (recalled/cited still work).
	MemoryFactInjectCounter biz.MemoryFactInjectCounter
	// MemoryReconsolidator triggers L4 memory reconsolidation when entities
	// are recalled into the prompt (design §15.7, FR-10.5). Optional: when
	// nil, the before-model hook skips the reconsolidation trigger.
	MemoryReconsolidator biz.L4Reconsolidator
	// AgentCaseRecaller feeds the task-experience case block (P3 M3): the
	// agent's distilled goal/approach/pitfalls from past sessions, merged
	// into the recall cue alongside L2/L3. Optional: nil skips case recall.
	AgentCaseRecaller  biz.AgentCaseRecaller
	KnowledgeRetriever *knowledge.Retriever
	KnowledgeUsecase   *biz.KnowledgeUsecase
	// ManualCompressor handles session-level compression triggered by the
	// compact tool. When wired, agents can actively invoke the compact tool
	// to compress older conversation history into a summary. May be nil
	// (the compact tool becomes unavailable).
	ManualCompressor biz.ManualCompressor
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
	// SkillHealthProvider feeds historical performance (success rate / avg
	// duration) into the Layer B ranking fusion branch of
	// ResolveSkillSlugsDetailed (R1, 2026-08-13). Optional: when nil, the
	// branch is skipped and ranking stays keyword/embedding only.
	SkillHealthProvider skillrecommend.HealthMetricsProvider
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
	// ResultCache stores results of cache-enabled catalog tools (PERF-1).
	// Optional: when nil, the agent package's process-wide default instance
	// (512 entries) is used, preserving the historical cache.Global() behavior.
	ResultCache *cache.ResultCache
	LG          loggateway.Logger
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
	// ClientBridge enables the client tool bridge ToolSet (client_open_app /
	// client_open_url) executed on the user's desktop companion.
	// Optional: when nil, client bridge tools are pruned from assembly.
	ClientBridge *clientbridge.Bridge
	// A2AEnabled indicates whether the A2A invoker will be injected at runtime.
	// When false, the call_agent tool is pruned to avoid registering a tool that
	// always fails with "invoker not configured".
	A2AEnabled bool
	// L0SnapshotForcer allows the compression pipeline to signal that the next
	// L0 snapshot write should bypass throttling. Optional: when nil, force
	// flags are ignored and normal throttle rules apply.
	L0SnapshotForcer biz.L0SnapshotForcer
	// LearningLoop records tool_call observations into the learning loop.
	// Optional: when nil, observation recording is skipped.
	LearningLoop biz.ObservationRecorder
	// TeamCompletionChecker provides team completion status checking for the team completion guard.
	// Optional: when nil, the team completion guard is disabled.
	TeamCompletionChecker TeamCompletionChecker
	// WebResearchReady is true when this build actually assembled web_research
	// (resident or deferred). The fact-query guard must not block web_fetch
	// when the preferred tool was pruned (no Tavily/SerpAPI key).
	WebResearchReady bool
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

// SetTeamCompletionChecker injects the TeamCompletionChecker at runtime to break circular dependencies.
// This method should be called after TRPCBuilderDeps is created but before it's used to build agents.
func (d *TRPCBuilderDeps) SetTeamCompletionChecker(checker TeamCompletionChecker) {
	d.TeamCompletionChecker = checker
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
