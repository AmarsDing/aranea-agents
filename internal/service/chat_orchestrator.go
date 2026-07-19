package service

import (
	"context"
	"time"

	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/debug"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/graph"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/internal/runtime/turn"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/tools"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	"aranea-agents/internal/tools/security"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	orchMapMaxIdle     = 30 * time.Minute
	orchMapSweepPeriod = 5 * time.Minute
)

// sessionStateTransitor is the interface for session status transitions.
// *biz.TurnLifecycleUsecase satisfies this interface, allowing the service
// layer to delegate session state management to the biz layer.
type sessionStateTransitor interface {
	TransitionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason)
}

// Compile-time check: *biz.TurnLifecycleUsecase satisfies sessionStateTransitor.
var _ sessionStateTransitor = (*biz.TurnLifecycleUsecase)(nil)

// RuntimeTooling groups plugin, skill, knowledge, and code-execution dependencies
// that are injected into every agent turn build. Moving these out of the flat
// ChatOrchestratorDeps reduces the Wire parameter count and makes the
// responsibility boundary explicit.
type RuntimeTooling struct {
	PluginRT                    *plugintrpc.Runtime
	PluginManager               *plugintrpc.Manager
	SkillDBRepo                 trpcskill.Repository
	KnowledgeRetriever          *knowledge.Retriever
	KnowledgeRouter             *knowledge.AdaptiveRouter
	KnowledgeFederatedRetriever *knowledge.FederatedRetriever
	KnowledgeEvaluator          *knowledge.RetrievalEvaluator
	KnowledgeUC                 *biz.KnowledgeUsecase
	CodeExecFactory             *localexec.Factory
	KanbanBridge                kanbanpkg.Bridge
	DebugRecorder               *debug.RecorderFactory
	OrganizationUC              *biz.OrganizationUsecase
	ToolResultGate              *biz.ToolResultGate
	OutboundRouter              *outbound.Router
	SubAgentService             *subagenttool.Service
	// ParallelToolExecutor enables batch tool call parallelism (B5 integration).
	// Nil when ARANEA_PARALLEL_AUTO is disabled; callers fall back to serial execution.
	ParallelToolExecutor *tools.ParallelToolExecutor
}

// TeamOrchestrationDeps groups team execution and graph compilation dependencies.
// These are only used when a session is owned by a team or when graph execution
// is triggered from the chat orchestrator.
type TeamOrchestrationDeps struct {
	TeamUC           *biz.TeamUsecase
	TeamsNative      biz.TeamRunnerWirePort
	GraphFactory     biz.GraphBuilderFactory
	Graphs           *biz.GraphUsecase
	Tasks            *biz.TaskUsecase
	TeamGraphCoord   biz.TeamGraphCoordPort
	TeamMediator     biz.TeamMediatorPort
	SpiritUC         biz.SpiritTeamController
	TaskPlanner      biz.TaskPlannerPort
	AgentAllocator   biz.AgentAllocatorPort
	TaskOrchestrator biz.TaskOrchestratorPort
	// P1 fix (2026-06-18): Previously-orphan graph components now wired into
	// production via TeamOrchestrationDeps. NL2GraphConverter enables natural
	// language → graph build config conversion; RuntimeReplanner enables
	// automatic replanning on graph node failures.
	NL2GraphConverter graph.NL2GraphConverter
	RuntimeReplanner  graph.RuntimeReplanner
}

// ChannelTurnJobDeps groups channel turn job tracking and session run management.
// These are used for channel async job lifecycle.
type ChannelTurnJobDeps struct {
	TurnJobs    *biz.ChannelTurnJobUsecase
	SessionRuns *biz.SessionRunUsecase
	Channels    *biz.ChannelUsecase
}

// ChannelNotifierDeps groups notification dependencies for session run escalation.
type ChannelNotifierDeps struct {
	RunEscalation SessionRunEscalationNotifier
}

// ChatChannelDeps groups channel turn job and notification dependencies.
type ChatChannelDeps struct {
	ChJobs   ChannelTurnJobDeps
	ChNotify ChannelNotifierDeps
}

// chatTurnCoreDeps groups core turn execution dependencies: TurnDeps, RuntimeTooling,
// admission gate, admission usecase, and turn timeout. Consolidating these into a
// single struct reduces ChatOrchestrator's field count (AS-COG-01).
type chatTurnCoreDeps struct {
	TD               rt.TurnDeps
	RT               RuntimeTooling
	AdmitGate        *turn.AdmissionGate
	Admission        *biz.TurnAdmissionUsecase
	TurnTimeout      time.Duration
	ActivityWriter   biz.ActivityWriter   // AF phase: Activity persistence for direct create/update
	ActivityUpserter biz.ActivityUpserter // AF phase: Activity persistence for ActivityProjector
	ActivityReader   biz.ActivityReader   // AF phase: Activity lookup for Confirm API (legacy v1; retained until Task 15)
	StepReader       biz.StepV2Reader     // Phase 3b-D Task 5: v2 step reader for ConfirmActivity
}

// chatTurnLifecycle combines session state transition, turn metrics recording,
// and turn event publishing into a single composite interface.
// Merging 3 sub-manager interfaces into 1 reduces ChatOrchestrator's field count (AS-COG-01).
type chatTurnLifecycle interface {
	sessionStateTransitor
	turnRecorder
	turnEventPublisher
}

// chatRunManager combines run status tracking, pending queue management,
// await coordination, and session run lifecycle into a single composite interface.
// Merging 4 sub-manager interfaces into 1 reduces ChatOrchestrator's field count (AS-COG-01).
type chatRunManager interface {
	runStatusTracker
	pendingQueueManager
	awaitCoordinator
	sessionRunLifecycle
}

// ChatOrchestrator owns the turn lifecycle: admission, execution, status tracking,
// and post-turn side effects. ChatService delegates all orchestration work here.
//
// Field count: 13 (well under AS-COG-01 limit of 15).
type ChatOrchestrator struct {
	core         chatTurnCoreDeps
	channelDeps  ChatChannelDeps
	usageDeps    ChatUsageDeps
	teamExecDeps ChatTeamDeps
	evoDeps      ChatEvolutionDeps
	infraDeps    ChatInfraDeps

	runs       *rt.RunRegistry
	chatUC     *biz.ChatUsecase
	turnLC     chatTurnLifecycle
	runMgr     chatRunManager
	agentBuild agentBuildDirector

	// cmdSafetyChecker enforces command safety rules (protected paths like
	// .aws/.ssh/.env) on every tool call via the framework's per-run
	// permission policy. Non-protected tools pass through with zero overhead.
	cmdSafetyChecker *security.CommandSafetyPermissionChecker

	// v2Seq is the v2 Sequencer (persist + WS) extracted from V2ProjectorFactory.
	v2Seq rt.EventPublisher

	// immediateFactWriter persists <fact> tags from agent responses to memory_fact
	// immediately after each turn, bridging the async gap to Sleep-time consolidation.
	immediateFactWriter *biz.ImmediateFactWriter

	sweepStop chan struct{}
}

// chatTurnLifecycleImpl combines sessionStateTransitor, turnRecorder, and
// turnEventPublisher into a single struct satisfying chatTurnLifecycle.
type chatTurnLifecycleImpl struct {
	sessionStateTransitor
	turnRecorder
	turnEventPublisher
}

var _ chatTurnLifecycle = (*chatTurnLifecycleImpl)(nil)

// chatRunManagerImpl combines runStatusTracker, pendingQueueManager,
// awaitCoordinator, and sessionRunLifecycle into a single struct satisfying chatRunManager.
type chatRunManagerImpl struct {
	runStatusTracker
	pendingQueueManager
	awaitCoordinator
	sessionRunLifecycle
}

// Sweep resolves the ambiguity between runStatusTracker.Sweep and
// awaitCoordinator.Sweep by delegating to both.
func (m *chatRunManagerImpl) Sweep() {
	m.runStatusTracker.Sweep()
	m.awaitCoordinator.Sweep()
}

var _ chatRunManager = (*chatRunManagerImpl)(nil)

// Accessor methods preserve call-site compatibility after field grouping (AS-COG-01).
func (o *ChatOrchestrator) td() rt.TurnDeps                        { return o.core.TD }
func (o *ChatOrchestrator) tdPtr() *rt.TurnDeps                    { return &o.core.TD }
func (o *ChatOrchestrator) rt() RuntimeTooling                     { return o.core.RT }
func (o *ChatOrchestrator) admitGate() *turn.AdmissionGate         { return o.core.AdmitGate }
func (o *ChatOrchestrator) admission() *biz.TurnAdmissionUsecase   { return o.core.Admission }
func (o *ChatOrchestrator) turnTimeout() time.Duration             { return o.core.TurnTimeout }
func (o *ChatOrchestrator) activityWriter() biz.ActivityWriter     { return o.core.ActivityWriter }
func (o *ChatOrchestrator) activityUpserter() biz.ActivityUpserter { return o.core.ActivityUpserter }
func (o *ChatOrchestrator) activityReader() biz.ActivityReader     { return o.core.ActivityReader }
func (o *ChatOrchestrator) stepReader() biz.StepV2Reader           { return o.core.StepReader }

func (o *ChatOrchestrator) team() TeamOrchestrationDeps   { return o.teamExecDeps.Team }
func (o *ChatOrchestrator) chJobs() ChannelTurnJobDeps    { return o.channelDeps.ChJobs }
func (o *ChatOrchestrator) chNotify() ChannelNotifierDeps { return o.channelDeps.ChNotify }

func (o *ChatOrchestrator) usage() *biz.UsageUsecase                   { return o.usageDeps.Usage }
func (o *ChatOrchestrator) monitor() *biz.MonitorUsecase               { return o.usageDeps.Monitor }
func (o *ChatOrchestrator) artifacts() *biz.ArtifactUsecase            { return o.usageDeps.Artifacts }
func (o *ChatOrchestrator) skillStats() biz.SkillInvocationStatsReader { return o.usageDeps.SkillStats }
func (o *ChatOrchestrator) expAnalytics() *biz.ExperienceAnalyticsUsecase {
	return o.usageDeps.ExpAnalytics
}

func (o *ChatOrchestrator) spiritAssembler() *SpiritTeamAssembler {
	return o.teamExecDeps.SpiritAssembler
}
func (o *ChatOrchestrator) spiritSynthesis() *SpiritSynthesisService {
	return o.teamExecDeps.SpiritSynthesis
}
func (o *ChatOrchestrator) teamStarter() biz.TeamStarterPort { return o.teamExecDeps.TeamStarter }
func (o *ChatOrchestrator) graphExec() biz.GraphExecutor     { return o.teamExecDeps.GraphExec }

func (o *ChatOrchestrator) skillEvo() *biz.SkillEvolutionUsecase { return o.evoDeps.SkillEvo }
func (o *ChatOrchestrator) evolution() *biz.EvolutionUsecase     { return o.evoDeps.Evolution }

func (o *ChatOrchestrator) a2aUC() *biz.A2AUsecase             { return o.infraDeps.A2AUC }
func (o *ChatOrchestrator) mcpServers() *biz.MCPServerUsecase  { return o.infraDeps.MCPServers }
func (o *ChatOrchestrator) orchCache() *biz.OrchestrationCache { return o.infraDeps.OrchCache }
func (o *ChatOrchestrator) outboundRouter() *outbound.Router   { return o.infraDeps.OutboundRouter }
func (o *ChatOrchestrator) subAgentService() *subagenttool.Service {
	return o.infraDeps.SubAgentService
}
func (o *ChatOrchestrator) lg() loggateway.Logger { return o.infraDeps.LG }

// immediateFactWriter returns the ImmediateFactWriter for persisting <fact> tags.
// Returns nil when memory consolidation writer is not wired (graceful degradation).
func (o *ChatOrchestrator) factWriter() *biz.ImmediateFactWriter { return o.immediateFactWriter }

// profileResolver returns the Wire-injected ProfileResolver, or nil when not
// configured. Callers must nil-check before use.
func (o *ChatOrchestrator) profileResolver() *chatagent.ProfileResolver {
	return o.infraDeps.ProfileResolver
}

// heartbeatEmitter returns the Wire-injected RunHeartbeatEmitter, or nil when
// not configured. Callers must nil-check before use.
func (o *ChatOrchestrator) heartbeatEmitter() *RunHeartbeatEmitter {
	return o.infraDeps.HeartbeatEmitter
}

// deadLetterQueue returns the Wire-injected DeadLetterQueue for pending-queue
// failures (A4), or nil when not configured. Callers must nil-check before use.
func (o *ChatOrchestrator) deadLetterQueue() *lifecycle.DeadLetterQueue {
	return o.infraDeps.DeadLetterQueue
}

// Sub-manager accessors delegate to the composite interfaces.
func (o *ChatOrchestrator) sessionStateMgr() sessionStateTransitor { return o.turnLC }
func (o *ChatOrchestrator) turnMetrics() turnRecorder              { return o.turnLC }
func (o *ChatOrchestrator) eventPublisher() turnEventPublisher     { return o.turnLC }
func (o *ChatOrchestrator) runStatus() runStatusTracker            { return o.runMgr }
func (o *ChatOrchestrator) pendingQ() pendingQueueManager          { return o.runMgr }
func (o *ChatOrchestrator) awaitCoord() awaitCoordinator           { return o.runMgr }
func (o *ChatOrchestrator) sessionRunLC() sessionRunLifecycle      { return o.runMgr }

// ChatTurnDeps groups turn execution lifecycle dependencies: session pipeline,
// run registry, runtime tooling, admission control, and turn timeout.
type ChatTurnDeps struct {
	rt.TurnDeps
	Runs             *rt.RunRegistry
	PendingQueue     *rt.PendingMessageQueue
	RT               RuntimeTooling
	TurnTimeout      time.Duration
	Admission        *biz.TurnAdmissionUsecase
	ActivityWriter   biz.ActivityWriter   // AF phase: Activity persistence for direct create/update
	ActivityUpserter biz.ActivityUpserter // AF phase: Activity persistence for ActivityProjector
	ActivityReader   biz.ActivityReader   // AF phase: Activity lookup for Confirm API (legacy v1; retained until Task 15)
	StepReader       biz.StepV2Reader     // Phase 3b-D Task 5: v2 step reader for ConfirmActivity
}

// ChatUsageDeps groups usage tracking, monitoring, artifact, and analytics dependencies.
type ChatUsageDeps struct {
	Usage        *biz.UsageUsecase
	Monitor      *biz.MonitorUsecase
	Artifacts    *biz.ArtifactUsecase
	SkillStats   biz.SkillInvocationStatsReader
	ExpAnalytics *biz.ExperienceAnalyticsUsecase
}

// ChatTeamDeps groups team orchestration, graph execution, and spirit assembly dependencies.
type ChatTeamDeps struct {
	Team            TeamOrchestrationDeps
	TeamStarter     biz.TeamStarterPort
	GraphExec       biz.GraphExecutor
	SpiritAssembler *SpiritTeamAssembler
	SpiritSynthesis *SpiritSynthesisService
}

// ChatEvolutionDeps groups skill evolution and agent evolution dependencies.
type ChatEvolutionDeps struct {
	SkillEvo  *biz.SkillEvolutionUsecase
	Evolution *biz.EvolutionUsecase
}

// ChatInfraDeps groups cross-cutting infrastructure dependencies: logging,
// orchestration cache, A2A, MCP, outbound routing, sub-agent service, and
// the biz-layer turn lifecycle usecase.
type ChatInfraDeps struct {
	LG              loggateway.Logger
	OrchCache       *biz.OrchestrationCache
	A2AUC           *biz.A2AUsecase
	MCPServers      *biz.MCPServerUsecase
	OutboundRouter  *outbound.Router
	SubAgentService *subagenttool.Service
	TurnLifecycle   *biz.TurnLifecycleUsecase
	// HeartbeatEmitter publishes run_heartbeat events so the frontend can
	// detect stale runs within 30s (P1-7). When nil, no heartbeats are
	// emitted and stale detection degrades gracefully.
	HeartbeatEmitter *RunHeartbeatEmitter
	// DeadLetterQueue holds pending-queue messages that failed processing
	// (A4). When nil, failed messages are only logged (legacy behavior).
	DeadLetterQueue *lifecycle.DeadLetterQueue
	// ProfileResolver resolves the active runtime profile for an agent
	// and converts it to framework RunOptions applied per turn. When nil,
	// no profile overrides are applied (graceful degradation).
	ProfileResolver *chatagent.ProfileResolver
	// V2ProjectorFactory creates per-turn v2 ActivityProjector instances.
	// Each turn (spirit + each team member) gets its own instance, isolating
	// per-turn streaming state. The factory shares the singleton Sequencer +
	// SeqAssigner so Seq allocation remains globally monotonic per spirit
	// session. Wired via Wire DI; nil = v2 disabled.
	V2ProjectorFactory *v2.ProjectorFactory
	// MemoryConsolidationWriter persists facts to memory_fact. Used to create
	// ImmediateFactWriter for <fact> tag extraction. When nil, immediate fact
	// extraction is disabled (graceful degradation).
	MemoryConsolidationWriter biz.MemoryConsolidationWriter
}

// ChatOrchestratorDeps groups all dependencies for ChatOrchestrator construction.
// Sub-aggregates (ChatTurnDeps, ChatUsageDeps, ChatTeamDeps, ChatEvolutionDeps,
// ChatInfraDeps, ChatChannelDeps) reduce the flat parameter count and make
// responsibility boundaries explicit.
type ChatOrchestratorDeps struct {
	Turn      ChatTurnDeps
	Usage     ChatUsageDeps
	Channel   ChatChannelDeps
	Team      ChatTeamDeps
	Evolution ChatEvolutionDeps
	Infra     ChatInfraDeps
}

func coalesceRunRegistry(r *rt.RunRegistry) *rt.RunRegistry {
	if r != nil {
		return r
	}
	return rt.NewRunRegistry()
}

func coalescePendingQueue(q *rt.PendingMessageQueue) *rt.PendingMessageQueue {
	if q != nil {
		return q
	}
	return rt.NewPendingMessageQueue()
}

func NewChatOrchestrator(deps ChatOrchestratorDeps) *ChatOrchestrator {
	runs := coalesceRunRegistry(deps.Turn.Runs)
	pending := coalescePendingQueue(deps.Turn.PendingQueue)
	sessionLocks := biz.NewSessionLockManager()
	var v2Seq rt.EventPublisher
	if pf := deps.Infra.V2ProjectorFactory; pf != nil {
		v2Seq = pf.Seq()
	}
	chatUC := NewChatUsecaseFromDeps(runs, pending, sessionLocks, deps.Turn.Sessions, deps.Turn.Pipeline.EventBus, v2Seq, deps.Infra.LG)
	// Wire provider/model resolution ports (BA4): biz-layer methods need
	// access to RefineLLM config, LLM catalog, and session updates.
	chatUC.SetRefineLLMLookup(deps.Turn.ReadDeps.Settings)
	chatUC.SetModelLister(deps.Turn.ReadDeps.LLM)
	chatUC.SetSessionUpdater(deps.Turn.Sessions)

	// Build individual sub-managers first.
	stateMgr := sessionStateTransitor(deps.Infra.TurnLifecycle)
	metrics := turnRecorder(newChatTurnMetrics(deps.Turn.Sessions, deps.Usage.Usage, deps.Infra.LG))
	evtPub := turnEventPublisher(newChatTurnEventPublisher(deps.Turn.Sessions, deps.Turn.Pipeline.EventBus, v2Seq, deps.Infra.LG))
	rStatus := runStatusTracker(newChatRunStatusTracker(runs, deps.Turn.Sessions, deps.Turn.Pipeline.EventBus, deps.Infra.LG))
	pendQ := pendingQueueManager(newChatPendingQueueManager(chatUC))
	awaitCoord := awaitCoordinator(newChatAwaitCoordinator(chatAwaitCoordinatorDeps{
		ChatUC:       chatUC,
		RunStatus:    rStatus,
		SessionState: stateMgr,
		SessionRT: func() *araneasession.Runtime {
			if deps.Turn.SessionRT != nil {
				return deps.Turn.SessionRT
			}
			if deps.Turn.Persist.Session == nil {
				return nil
			}
			return araneasession.NewRuntime(deps.Turn.Persist.Session, deps.Infra.LG)
		},
		EventBus: deps.Turn.Pipeline.EventBus,
		Seq:      v2Seq,
		Logger:   deps.Infra.LG,
	}))
	sessRunLC := sessionRunLifecycle(newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns:  deps.Channel.ChJobs.SessionRuns,
		Channels:     deps.Channel.ChJobs.Channels,
		Sessions:     deps.Turn.Sessions,
		RunStatus:    rStatus,
		SessionState: stateMgr,
		Runs:         runs,
		Escalation:   deps.Channel.ChNotify.RunEscalation,
		Logger:       deps.Infra.LG,
	}))

	turnTimeout := deps.Turn.TurnTimeout
	if turnTimeout <= 0 {
		turnTimeout = chatagent.DefaultTurnTimeout
	}

	o := &ChatOrchestrator{
		core: chatTurnCoreDeps{
			TD:               deps.Turn.TurnDeps,
			RT:               deps.Turn.RT,
			Admission:        deps.Turn.Admission,
			TurnTimeout:      turnTimeout,
			ActivityWriter:   deps.Turn.ActivityWriter,
			ActivityUpserter: deps.Turn.ActivityUpserter,
			ActivityReader:   deps.Turn.ActivityReader,
			StepReader:       deps.Turn.StepReader,
		},
		channelDeps:  deps.Channel,
		usageDeps:    deps.Usage,
		teamExecDeps: deps.Team,
		evoDeps:      deps.Evolution,
		infraDeps:    deps.Infra,
		runs:         runs,
		chatUC:       chatUC,
		v2Seq:        v2Seq,
		immediateFactWriter: biz.NewImmediateFactWriter(deps.Infra.MemoryConsolidationWriter, deps.Infra.LG),
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: stateMgr,
			turnRecorder:          metrics,
			turnEventPublisher:    evtPub,
		},
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    rStatus,
			pendingQueueManager: pendQ,
			awaitCoordinator:    awaitCoord,
			sessionRunLifecycle: sessRunLC,
		},
		cmdSafetyChecker: security.NewCommandSafetyPermissionChecker(deps.Infra.LG),
	}
	o.agentBuild = newChatAgentBuildDirector(chatAgentBuildDirectorDeps{
		TurnDeps:       deps.Turn.TurnDeps,
		RT:             deps.Turn.RT,
		AwaitCoord:     awaitCoord,
		SubAgentSvc:    deps.Infra.SubAgentService,
		OutboundRouter: deps.Infra.OutboundRouter,
		A2AEnabled:     deps.Infra.A2AUC != nil,
		CustomToolFunc: func(ctx context.Context, ag biz.Agent) []trpctool.Tool {
			var tools []trpctool.Tool
			tools = append(tools, o.cliAdminTools(ctx, ag)...)
			tools = append(tools, o.spiritCustomTools(ag)...)
			tools = append(tools, o.skillsButlerTools(ctx, ag)...)
			tools = append(tools, o.memoryButlerTools(ctx, ag)...)
			return tools
		},
		Logger: deps.Infra.LG,
	})
	o.core.AdmitGate = newTurnAdmissionGate(turn.RunRegistryAdapter{Registry: runs}, chatUC, pendQ.SessionPendingMergeFollowup)

	// Wire the threshold resolver so that TurnAdmissionUsecase.EvaluateContextPressure
	// uses the orchestrator's channel-aware threshold lookup policy.
	if o.admission() != nil {
		o.admission().SetThresholdResolver(biz.ThresholdResolverFunc(o.resolveContextAdmissionThresholdForSession))
		// Wire the channel config resolver so channel entry points use the
		// long-task config threshold directly instead of the agent L0 threshold.
		if o.sessionRunLC() != nil {
			o.admission().SetChannelConfigResolver(biz.ChannelLongTaskConfigResolverFunc(o.sessionRunLC().ResolveChannelLongTaskConfig))
		}
	}

	if deps.Team.Team.TeamsNative != nil {
		deps.Team.Team.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) biz.AwaitReplyFunc {
			return o.awaitCoord().MakeAwaitReplyFunc(runCtx, sessionID, runID)
		})
		if deps.Team.Team.TeamMediator != nil {
			deps.Team.Team.TeamsNative.SetMediator(deps.Team.Team.TeamMediator)
			deps.Team.Team.TeamMediator.SetFinisher(deps.Team.Team.TeamsNative)
			if deps.Team.Team.TeamGraphCoord != nil {
				deps.Team.Team.TeamGraphCoord.SetFinisher(deps.Team.Team.TeamMediator)
				deps.Team.Team.TeamGraphCoord.RecoverSessions(context.Background())
			}
		}
	}

	o.sweepStop = make(chan struct{})
	safego.GoBackground("orch-map-sweep", o.sweepLoop)
	return o
}

// Compile-time interface assertions.
var (
	_ biz.TurnExecutor          = (*ChatOrchestrator)(nil)
	_ biz.ChannelTurnGateway    = (*ChatService)(nil)
	_ biz.TurnExecutorGateway   = (*ChatService)(nil)
	_ biz.TurnRunControlGateway = (*ChatService)(nil)
	_ biz.TurnGateway           = (*ChatService)(nil)
	_ biz.TurnControlGateway    = (*ChatService)(nil)
	_ biz.DurableResumeGateway  = (*ChatService)(nil)
	_ biz.PendingQueueGateway   = (*ChatService)(nil)
	_ biz.A2ARunnerFactory      = (*ChatService)(nil)
)

// Execute implements biz.TurnExecutor — the shared entry point for all turn
// execution paths (Web, WS, Channel, Cron, A2A).
func (o *ChatOrchestrator) Execute(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	return o.RunNativeAgentTurnWithOutcome(ctx, input)
}

// RunGateway exposes the shared session run registry.
func (o *ChatOrchestrator) RunGateway() rt.RunGateway {
	return o.runs
}

// HasActiveRun reports whether a session has an in-flight run.
func (o *ChatOrchestrator) HasActiveRun(sessionID string) bool {
	return o.runs.HasActive(sessionID)
}

// HasActiveRunner reports whether the active run has a live trpc runner (steer-ready).
func (o *ChatOrchestrator) HasActiveRunner(sessionID string) bool {
	if o == nil || o.runs == nil {
		return false
	}
	_, _, ok := o.runs.ActiveRunner(sessionID)
	return ok
}

// CancelRun stops the active run for a session.
func (o *ChatOrchestrator) CancelRun(ctx context.Context, sessionID string) bool {
	// P0-01 fix: verify session ownership before cancelling.
	authUserID := ctxuser.FromContext(ctx)
	if authUserID != ctxuser.DefaultUserID {
		if sess, err := o.td().Sessions.Get(ctx, sessionID); err == nil && sess.UserID != "" && authUserID != sess.UserID {
			o.lg().Warn("cancel denied: session ownership mismatch",
				loggateway.StepID("chat.cancel_ownership"),
				loggateway.SessionID(sessionID),
				loggateway.Str("auth_user", authUserID),
				loggateway.Str("session_user", sess.UserID))
			return false
		}
	}
	return o.cancelActiveRun(ctx, sessionID)
}

// LastPendingMessageID returns the most recently enqueued pending message id.
func (o *ChatOrchestrator) LastPendingMessageID(sessionID string) string {
	return o.pendingQ().LastPendingMessageID(sessionID)
}

// GetPendingMessages returns pending messages for a session.
func (o *ChatOrchestrator) GetPendingMessages(sessionID string) []biz.PendingQueueEntry {
	return o.pendingQ().GetPendingMessages(sessionID)
}

// CancelPendingMessage cancels a pending message.
func (o *ChatOrchestrator) CancelPendingMessage(sessionID, pendingID string) bool {
	return o.pendingQ().CancelPendingMessage(sessionID, pendingID)
}

// UpdatePendingMessage updates a pending message's content.
func (o *ChatOrchestrator) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return o.pendingQ().UpdatePendingMessage(sessionID, pendingID, content)
}

// EnqueueUserMessage enqueues a user message when a turn is active.
func (o *ChatOrchestrator) EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return o.pendingQ().EnqueueUserMessage(sessionID, content)
}

// SetSessionPendingMergeFollowup toggles followup merge for pending queue enqueues (CH-BOR-01).
func (o *ChatOrchestrator) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	o.pendingQ().SetSessionPendingMergeFollowup(sessionID, merge)
}

// InterruptAndSendMessage promotes a pending message to the front, marks it high priority,
// and cancels the current turn so the pending queue processor picks it up next.
func (o *ChatOrchestrator) InterruptAndSendMessage(ctx context.Context, sessionID, pendingEntryID string) error {
	if o == nil || o.chatUC == nil {
		return nil
	}
	return o.chatUC.InterruptAndSendMessage(ctx, sessionID, pendingEntryID)
}

// DequeuePendingMessage dequeues the next pending message.
func (o *ChatOrchestrator) DequeuePendingMessage(sessionID string) (biz.PendingQueueEntry, bool) {
	return o.pendingQ().DequeuePendingMessage(sessionID)
}

// GetRunStatus returns the current run lifecycle state for a session.
func (o *ChatOrchestrator) GetRunStatus(ctx context.Context, sessionID string) (runID, status, errMsg string, updatedAt string, ok bool) {
	return o.runStatus().GetRunStatus(ctx, sessionID)
}

// ActiveRunner returns the active runner for a session, if any.
func (o *ChatOrchestrator) ActiveRunner(sessionID string) (runner trpcrunner.Runner, requestID string, active bool) {
	return o.runs.ActiveRunner(sessionID)
}

func (o *ChatOrchestrator) transitionSessionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	o.sessionStateMgr().TransitionStatus(ctx, sessionID, targetStatus, reason)
}

// cancelActiveRun cancels the active run for a session.
func (o *ChatOrchestrator) cancelActiveRun(ctx context.Context, sessionID string) bool {
	if o == nil || sessionID == "" {
		return false
	}
	stopped, runID := o.runs.Cancel(sessionID, "user_cancel")
	if !stopped {
		return false
	}
	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, biz.SessionRunPhaseCancelled, ""); err != nil {
		o.lg().Warn("set run status failed on cancel",
			loggateway.StepID("chat.cancel_run"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonUserCancelled)
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, o.stepReader(), o.activityWriter(), sessionID, o.lg()); err != nil {
		o.lg().Warn("取消执行卡片查询失败",
			loggateway.StepID("chat.activity.cancel"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
	return true
}

// setRunStatus delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) setRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, status, errMsg); err != nil {
		o.lg().Warn("set run status failed",
			loggateway.StepID("chat.set_run_status"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Str("status", status),
			loggateway.Err(err))
	}
}

func (o *ChatOrchestrator) setRunStatusWithAwait(ctx context.Context, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	if err := o.runStatus().SetRunStatusWithAwait(ctx, sessionID, runID, status, errMsg, await); err != nil {
		o.lg().Warn("set run status with await failed",
			loggateway.StepID("chat.set_run_status"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Str("status", status),
			loggateway.Err(err))
	}
}

func (o *ChatOrchestrator) publishRunStatus(sessionID, runID, status, errMsg string) {
	o.runStatus().PublishRunStatus(sessionID, runID, status, errMsg)
}

// publishTurnTimeoutNotification pushes a timeout alert via WS so the user knows
// the turn is taking longer than expected, without failing the turn.
func (o *ChatOrchestrator) publishTurnTimeoutNotification(ctx context.Context, sessionID, runID string, timeout time.Duration) {
	monitorBus := o.td().Pipeline.MonitorEventBus
	if monitorBus == nil {
		return
	}
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeAlertNotify, "chat-service")
	ev.SessionID = sessionID
	ev.Metadata = map[string]any{
		"alert_kind":    "turn_timeout",
		"timeout":       timeout.String(),
		"message":       "对话响应时间较长，请耐心等待或手动停止",
		"i18n_key":      "chat.turn.timeout_notify",
		"request_id":    sessionID,
		"invocation_id": runID,
	}
	monitorBus.Publish(ctx, ev)
}

func (o *ChatOrchestrator) lockSession(sessionID string) func() {
	return o.chatUC.LockSession(sessionID)
}

// AttachNativeTurnAfterHook sets the post-turn hook.
func (o *ChatOrchestrator) AttachNativeTurnAfterHook(hook biz.NativeTurnAfterHook) {
	if o == nil || hook == nil {
		return
	}
	o.core.TD.AfterTurn = hook
}

// SetTaskOrchestrator sets the TaskOrchestratorPort on the TeamOrchestrationDeps.
//
// Deprecated: TaskOrchestrator is now backfilled via provideChatServiceDeps in wire.go.
// This method is retained for potential ad-hoc use but is no longer called by Wire.
func (o *ChatOrchestrator) SetTaskOrchestrator(orch biz.TaskOrchestratorPort) {
	if o == nil {
		return
	}
	o.teamExecDeps.Team.TaskOrchestrator = orch
}

// AwaitChannel operations delegate to awaitCoord.
func (o *ChatOrchestrator) RegisterAwaitChannel(sessionID string, ch biz.AwaitChannel) {
	o.awaitCoord().RegisterAwaitChannel(sessionID, ch)
}

func (o *ChatOrchestrator) DeleteAwaitChannel(sessionID string) {
	o.awaitCoord().DeleteAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	return o.awaitCoord().LoadAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool {
	return o.awaitCoord().TrySendAwaitChannel(sessionID, msg)
}

// persistRunStatus delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) persistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error {
	return o.runStatus().PersistRunStatus(ctx, sessionID, runID, status, errMsg)
}

// hydrateRunStatusFromSession delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return o.runStatus().HydrateRunStatusFromSession(ctx, sessionID)
}

func (o *ChatOrchestrator) persistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool) {
	o.runStatus().PersistAwaitMarkers(ctx, sessionID, runID, await, syncWrite)
}

func (o *ChatOrchestrator) setAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	o.runStatus().SetAwaitMetaCache(sessionID, meta)
}

func (o *ChatOrchestrator) getAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	return o.runStatus().GetAwaitMetaCache(sessionID)
}

func (o *ChatOrchestrator) clearAwaitMetaCache(sessionID string) {
	o.runStatus().ClearAwaitMetaCache(sessionID)
}

func (o *ChatOrchestrator) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	return o.runStatus().ResolveAwaitMeta(ctx, sessionID, status)
}

func (o *ChatOrchestrator) clearAwaitingRunStateSync(ctx context.Context, sessionID string) error {
	return o.runStatus().ClearAwaitingRunStateSync(ctx, sessionID)
}

func (o *ChatOrchestrator) clearAwaitingRunState(ctx context.Context, sessionID string) {
	o.runStatus().ClearAwaitingRunState(ctx, sessionID)
}

// tryBeginResume delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) tryBeginResume(sessionID string) bool {
	return o.awaitCoord().TryBeginResume(sessionID)
}

func (o *ChatOrchestrator) endResume(sessionID string) {
	o.awaitCoord().EndResume(sessionID)
}

func (o *ChatOrchestrator) Close() {
	if o == nil {
		return
	}
	if o.sweepStop != nil {
		select {
		case o.sweepStop <- struct{}{}:
		default:
		}
	}
}

func (o *ChatOrchestrator) sweepLoop() {
	ticker := time.NewTicker(orchMapSweepPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			o.sweepStaleMaps()
		case <-o.sweepStop:
			return
		}
	}
}

func (o *ChatOrchestrator) sweepStaleMaps() {
	o.runStatus().Sweep()
	o.pendingQ().Sweep()
	o.awaitCoord().Sweep()
}

// publishAwaitResumed delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) publishAwaitResumed(sessionID, runID string) {
	o.awaitCoord().PublishAwaitResumed(sessionID, runID)
}

// sessionAwaitingUser delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) sessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	if coord := o.awaitCoord(); coord != nil {
		return coord.SessionAwaitingUser(ctx, sessionID)
	}
	return persistedRunStatus{}, false
}

// canResumeAwait delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) canResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	return o.awaitCoord().CanResumeAwait(ctx, sessionID)
}

// hasPendingAwaitUserReplyRoute delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) hasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	return o.awaitCoord().HasPendingAwaitUserReplyRoute(ctx, sessionID)
}

func (o *ChatOrchestrator) sessionRuntime() *araneasession.Runtime {
	if o == nil {
		return nil
	}
	if o.td().SessionRT != nil {
		return o.td().SessionRT
	}
	if o.td().Persist.Session == nil {
		return nil
	}
	return araneasession.NewRuntime(o.td().Persist.Session, o.lg())
}

func (o *ChatOrchestrator) resolveUserID(ctx context.Context, sessionID string) string {
	return ctxuser.TRPCUserKey(ctx)
}
