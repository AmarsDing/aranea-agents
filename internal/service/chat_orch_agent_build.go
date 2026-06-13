package service

import (
	"context"
	"fmt"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// AgentBuildParams aggregates the parameters for building TRPC agent deps.
// Introduced to satisfy BI1 (parameter count ≤ 5) while preserving call-site clarity.
// Stability:evolving
type AgentBuildParams struct {
	Session   biz.Session
	Agent     biz.Agent
	RunID     string
	DialogMode string
	Provider  string
	Model     string
	Emitter   *event.TraceEmitter
}

// agentBuildDirector constructs the TRPCBuilderDeps for agent turn execution.
// It encapsulates the 30+ field dependency assembly that was previously inline
// in runSingleAgentViaTRPC, reducing cognitive complexity of the BUILD phase.
// Stability:evolving
type agentBuildDirector interface {
	// BuildTRPCDeps constructs the TRPCBuilderDeps for a single agent turn.
	BuildTRPCDeps(ctx context.Context, p AgentBuildParams) (chatagent.TRPCBuilderDeps, error)
}

// chatAgentBuildDirector implements agentBuildDirector.
//
// Part of the TECH-DEBT(BL8) resolution: extracting TRPCBuilderDeps construction
// from ChatOrchestrator to reduce cognitive complexity (AS-COG-01).
type chatAgentBuildDirector struct {
	td             rt.TurnDeps
	rt             RuntimeTooling
	awaitCoord     awaitCoordinator
	subAgentSvc    *subagenttool.Service
	customToolFunc func(ctx context.Context, ag biz.Agent) []trpctool.Tool
	lg             loggateway.Logger
}

// chatAgentBuildDirectorDeps aggregates constructor dependencies for chatAgentBuildDirector.
// Introduced to satisfy BI1 (parameter count ≤ 5) for the constructor.
// Stability:internal
type chatAgentBuildDirectorDeps struct {
	TurnDeps      rt.TurnDeps
	RT            RuntimeTooling
	AwaitCoord    awaitCoordinator
	SubAgentSvc   *subagenttool.Service
	CustomToolFunc func(ctx context.Context, ag biz.Agent) []trpctool.Tool
	Logger        loggateway.Logger
}

func newChatAgentBuildDirector(d chatAgentBuildDirectorDeps) *chatAgentBuildDirector {
	return &chatAgentBuildDirector{
		td:             d.TurnDeps,
		rt:             d.RT,
		awaitCoord:     d.AwaitCoord,
		subAgentSvc:    d.SubAgentSvc,
		customToolFunc: d.CustomToolFunc,
		lg:             d.Logger,
	}
}

// Compile-time interface check.
var _ agentBuildDirector = (*chatAgentBuildDirector)(nil)

// BuildTRPCDeps constructs the TRPCBuilderDeps for a single agent turn.
func (d *chatAgentBuildDirector) BuildTRPCDeps(ctx context.Context, p AgentBuildParams) (chatagent.TRPCBuilderDeps, error) {
	sessionID := p.Session.ID

	// Assemble custom tools via callback (cliAdmin, spirit, skillsButler, memoryButler).
	var customTools []trpctool.Tool
	if d.customToolFunc != nil {
		customTools = d.customToolFunc(ctx, p.Agent)
	}

	// Fetch effective tools ONCE and reuse for hash computation and tool assembly.
	// Previously GetEffectiveTools was called 3 times per turn (computeToolHash,
	// computeMCPHash, loadEffectiveToolKeys), each triggering 3-5 DB queries.
	var cachedEffTools *biz.AgentEffectiveTools
	if d.td.ReadDeps.AgentsUC != nil {
		if eff, err := d.td.ReadDeps.AgentsUC.GetEffectiveTools(ctx, p.Agent.ID); err == nil {
			cachedEffTools = &eff
		}
	}

	// Compute content-based version hashes for tool/skill/MCP configurations.
	// These hashes are folded into the build cache key so that configuration
	// changes automatically invalidate the cached agent (defense-in-depth).
	toolHash := d.computeToolHashFromCached(cachedEffTools)
	skillHash := d.computeSkillHash(ctx)
	mcpHash := d.computeMCPHash(ctx, p.Agent.ID)

	deps := chatagent.TRPCBuilderDeps{
		TRPCModelCatalogDeps: chatagent.TRPCModelCatalogDeps{
			ModelCatalog: d.td.ReadDeps.LLM,
			AgentUC:      d.td.ReadDeps.AgentsUC,
			Agents:       d.td.ReadDeps.Agents,
			Sys:          d.td.ReadDeps.Settings,
			Sessions:     d.td.Sessions,
		},
		TRPCModelRouteDeps: chatagent.TRPCModelRouteDeps{
			RT:         d.td.RoundTrip(),
			Provider:   p.Provider,
			Model:      p.Model,
			DialogMode: p.DialogMode,
		},
		TRPCToolAssemblyDeps: chatagent.TRPCToolAssemblyDeps{
			ToolUC:               d.td.ReadDeps.ToolUC,
			MCPTooling:           d.td.Persist.AgentMCP,
			AwaitHook:            d.awaitCoord.MakeAwaitReplyFunc(ctx, sessionID, p.RunID),
			CustomTools:          customTools,
			KanbanBridge:         d.rt.KanbanBridge,
			CachedEffectiveTools: cachedEffTools,
		},
		TRPCMemoryKnowledgeDeps: chatagent.TRPCMemoryKnowledgeDeps{
			HasMemory:             d.td.Persist.Memory.Available(),
			MemoryService:         d.td.Persist.Memory.TRPC,
			MemoryAdmin:           d.td.Persist.Memory.Admin,
			MemoryL2Recall:        d.td.Persist.Memory.L2Recall,
			MemoryL3Recall:        d.td.Persist.Memory.L3Recall,
			MemoryCompositeRecall: d.td.Persist.Memory.CompositeRecall,
			KnowledgeRetriever:    d.rt.KnowledgeRetriever,
			KnowledgeUsecase:      d.rt.KnowledgeUC,
		},
		TRPCPluginDeps: chatagent.TRPCPluginDeps{
			PluginManager: d.rt.PluginManager,
		},
		TRPCSkillDeps: chatagent.TRPCSkillDeps{
			SkillUC:         d.td.ReadDeps.SkillUC,
			SkillDBRepo:     d.rt.SkillDBRepo,
			CodeExecFactory: d.rt.CodeExecFactory,
		},
		TRPCExtensionDeps: chatagent.TRPCExtensionDeps{
			Organization:     d.rt.OrganizationUC,
			ToolResultGate:   d.rt.ToolResultGate,
			SubAgentService:  d.subAgentSvc,
			L0SnapshotForcer: d.td.SessionRT,
			ToolVersionHash:  toolHash,
			SkillVersionHash: skillHash,
			MCPVersionHash:   mcpHash,
		},
	}

	return deps, nil
}

// computeToolHashFromCached produces a content hash from a pre-fetched effective tool result.
// This avoids a redundant GetEffectiveTools DB call when the result is already available.
func (d *chatAgentBuildDirector) computeToolHashFromCached(eff *biz.AgentEffectiveTools) string {
	if eff == nil {
		return ""
	}
	entries := make([]versionHashEntry, 0, len(eff.Items))
	for _, item := range eff.Items {
		state := "0"
		if item.Enabled {
			state = "1"
		}
		entries = append(entries, versionHashEntry{
			ID:        fmt.Sprintf("%s:%s", item.ToolKey, state),
			UpdatedAt: item.EffectiveState,
		})
	}
	return computeVersionHash(entries)
}

// computeToolHash produces a content hash from the agent's effective tool configuration.
// It hashes the sorted list of effective tool keys + their enabled states, so any change
// in which tools are available to the agent invalidates the cache.
// Deprecated: use computeToolHashFromCached when the effective tools result is already available.
func (d *chatAgentBuildDirector) computeToolHash(ctx context.Context, agentID string) string {
	if d.td.ReadDeps.AgentsUC == nil {
		return ""
	}
	eff, err := d.td.ReadDeps.AgentsUC.GetEffectiveTools(ctx, agentID)
	if err != nil {
		return ""
	}
	entries := make([]versionHashEntry, 0, len(eff.Items))
	for _, item := range eff.Items {
		state := "0"
		if item.Enabled {
			state = "1"
		}
		entries = append(entries, versionHashEntry{
			ID:        fmt.Sprintf("%s:%s", item.ToolKey, state),
			UpdatedAt: item.EffectiveState,
		})
	}
	return computeVersionHash(entries)
}

// computeSkillHash produces a content hash from the currently enabled published skill slugs.
// When skills are added/removed/toggled, this hash changes and invalidates the agent cache.
func (d *chatAgentBuildDirector) computeSkillHash(ctx context.Context) string {
	if d.td.ReadDeps.SkillUC == nil {
		return ""
	}
	slugs, err := d.td.ReadDeps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil {
		return ""
	}
	entries := make([]versionHashEntry, len(slugs))
	for i, slug := range slugs {
		entries[i] = versionHashEntry{ID: slug}
	}
	return computeVersionHash(entries)
}

// computeMCPHash produces a content hash from the agent's effective MCP servers.
// When MCP servers are added/removed/reconfigured, this hash changes and invalidates the cache.
func (d *chatAgentBuildDirector) computeMCPHash(ctx context.Context, agentID string) string {
	if d.td.Persist.AgentMCP == nil {
		return ""
	}
	servers, err := d.td.Persist.AgentMCP.EffectiveServersForAgent(ctx, agentID)
	if err != nil {
		return ""
	}
	entries := make([]versionHashEntry, len(servers))
	for i, s := range servers {
		entries[i] = versionHashEntry{ID: s.ID, UpdatedAt: s.ConfigJSON}
	}
	return computeVersionHash(entries)
}
