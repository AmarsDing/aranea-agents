package service

import (
	"context"
	"fmt"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/outbound"
	rt "aranea-agents/internal/runtime"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/pkg/loggateway"

	"golang.org/x/sync/errgroup"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// AgentBuildParams aggregates the parameters for building TRPC agent deps.
// Introduced to satisfy BI1 (parameter count ≤ 5) while preserving call-site clarity.
// Stability:evolving
type AgentBuildParams struct {
	Session    biz.Session
	Agent      biz.Agent
	RunID      string
	DialogMode string
	Provider   string
	Model      string
	Emitter    *event.TraceEmitter
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
	outboundRouter *outbound.Router
	a2aEnabled     bool
	customToolFunc func(ctx context.Context, ag biz.Agent) []trpctool.Tool
	lg             loggateway.Logger
}

// chatAgentBuildDirectorDeps aggregates constructor dependencies for chatAgentBuildDirector.
// Introduced to satisfy BI1 (parameter count ≤ 5) for the constructor.
// Stability:internal
type chatAgentBuildDirectorDeps struct {
	TurnDeps       rt.TurnDeps
	RT             RuntimeTooling
	AwaitCoord     awaitCoordinator
	SubAgentSvc    *subagenttool.Service
	OutboundRouter *outbound.Router
	A2AEnabled     bool
	CustomToolFunc func(ctx context.Context, ag biz.Agent) []trpctool.Tool
	Logger         loggateway.Logger
}

func newChatAgentBuildDirector(d chatAgentBuildDirectorDeps) *chatAgentBuildDirector {
	return &chatAgentBuildDirector{
		td:             d.TurnDeps,
		rt:             d.RT,
		awaitCoord:     d.AwaitCoord,
		subAgentSvc:    d.SubAgentSvc,
		outboundRouter: d.OutboundRouter,
		a2aEnabled:     d.A2AEnabled,
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

	// Parallel fetch: GetEffectiveTools, computeSkillHash, computeMCPHash
	// are independent DB queries. Running them concurrently reduces BUILD
	// phase latency from ~3x sequential DB round-trips to ~1x.
	var cachedEffTools *biz.AgentEffectiveTools
	var skillHash, mcpHash string

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		if d.td.ReadDeps.AgentsUC != nil {
			if eff, err := d.td.ReadDeps.AgentsUC.GetEffectiveTools(egCtx, p.Agent.ID); err == nil {
				cachedEffTools = &eff
			}
		}
		return nil
	})
	eg.Go(func() error {
		skillHash = d.computeSkillHash(egCtx)
		return nil
	})
	eg.Go(func() error {
		mcpHash = d.computeMCPHash(egCtx, p.Agent.ID)
		return nil
	})
	if err := eg.Wait(); err != nil {
		d.lg.Warn("agent build hash computation failed, hashes default to empty",
			loggateway.StepID("chat_orch_agent_build.hash"),
			loggateway.Err(err))
	}

	// Compute tool hash from cached result (depends on GetEffectiveTools above).
	toolHash := d.computeToolHashFromCached(cachedEffTools)

	deps := chatagent.TRPCBuilderDeps{
		TRPCModelCatalogDeps: chatagent.TRPCModelCatalogDeps{
			ModelCatalog: d.td.ReadDeps.LLM,
			AgentUC:      d.td.ReadDeps.AgentsUC,
			Agents:       d.td.ReadDeps.Agents,
			Sys:          d.td.ReadDeps.Settings,
			Sessions:     d.td.Sessions,
		},
		TRPCModelRouteDeps: chatagent.TRPCModelRouteDeps{
			RT:         d.td.RoundTripForSession(sessionID),
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
			MediaProviders:       d.td.ReadDeps.MediaProviders,
			ArtifactWriter:       d.td.Persist.ArtifactUC,
			CachedEffectiveTools: cachedEffTools,
		},
		TRPCMemoryKnowledgeDeps: chatagent.TRPCMemoryKnowledgeDeps{
			HasMemory:              d.td.Persist.Memory.Available(),
			MemoryService:          d.td.Persist.Memory.TRPC,
			MemoryAdmin:            d.td.Persist.Memory.Admin,
			MemoryActionLogWriter:  d.td.Persist.Memory.ActionLogWriter,
			ManualCompressor:       biz.ManualCompressorFromNative(d.td.Compress),
			MemoryL2Recall:         d.td.Persist.Memory.L2Recall,
			MemoryL3Recall:         d.td.Persist.Memory.L3Recall,
			MemoryCompositeRecall:  d.td.Persist.Memory.CompositeRecall,
			MemoryPreferenceLister: d.td.Persist.Memory.PreferenceLister,
			KnowledgeRetriever:     d.rt.KnowledgeRetriever,
			KnowledgeUsecase:       d.rt.KnowledgeUC,
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
			OutboundRouter:   d.outboundRouter,
			L0SnapshotForcer: d.td.SessionRT,
			LearningLoop:     d.td.LearningLoop,
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
