package service

import (
	"context"

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

	deps := chatagent.TRPCBuilderDeps{
		// TRPCModelCatalogDeps
		ModelCatalog: d.td.ReadDeps.LLM,
		AgentUC:      d.td.ReadDeps.AgentsUC,
		Agents:       d.td.ReadDeps.Agents,
		Sys:          d.td.ReadDeps.Settings,
		Sessions:     d.td.Sessions,
		// TRPCModelRouteDeps
		RT:         d.td.RoundTrip(),
		Provider:   p.Provider,
		Model:      p.Model,
		DialogMode: p.DialogMode,
		// TRPCToolAssemblyDeps
		ToolUC:       d.td.ReadDeps.ToolUC,
		MCPTooling:   d.td.Persist.AgentMCP,
		AwaitHook:    d.awaitCoord.MakeAwaitReplyFunc(ctx, sessionID, p.RunID),
		CustomTools:  customTools,
		KanbanBridge: d.rt.KanbanBridge,
		// TRPCMemoryKnowledgeDeps
		HasMemory:             d.td.Persist.Memory.Available(),
		MemoryService:         d.td.Persist.Memory.TRPC,
		MemoryAdmin:           d.td.Persist.Memory.Admin,
		MemoryL2Recall:        d.td.Persist.Memory.L2Recall,
		MemoryL3Recall:        d.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall: d.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever:    d.rt.KnowledgeRetriever,
		KnowledgeUsecase:      d.rt.KnowledgeUC,
		// TRPCPluginDeps (Plugins set by caller after agent build)
		PluginManager: d.rt.PluginManager,
		// TRPCSkillDeps
		SkillUC:         d.td.ReadDeps.SkillUC,
		SkillDBRepo:     d.rt.SkillDBRepo,
		CodeExecFactory: d.rt.CodeExecFactory,
		// Extended deps
		Organization:    d.rt.OrganizationUC,
		ToolResultGate: d.rt.ToolResultGate,
		SubAgentService: d.subAgentSvc,
		L0SnapshotForcer: d.td.SessionRT,
	}

	return deps, nil
}
