package service

import (
	"context"
	"time"

	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/debug"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/turn"
	araneasession "aranea-agents/internal/session"
	kanbanpkg "aranea-agents/internal/tools/kanban"
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

// ChatOrchestrator owns the turn lifecycle: admission, execution, status tracking,
// and post-turn side effects. ChatService delegates all orchestration work here.
type ChatOrchestrator struct {
	td              rt.TurnDeps
	rt              RuntimeTooling
	team            TeamOrchestrationDeps
	chJobs          ChannelTurnJobDeps
	chNotify        ChannelNotifierDeps
	admitGate       *turn.AdmissionGate
	admission       *biz.TurnAdmissionUsecase
	usage           *biz.UsageUsecase
	monitor         *biz.MonitorUsecase
	artifacts       *biz.ArtifactUsecase
	a2aUC           *biz.A2AUsecase
	mcpServers      *biz.MCPServerUsecase
	runs            *rt.RunRegistry
	chatUC          *biz.ChatUsecase
	lg              loggateway.Logger
	spiritAssembler *SpiritTeamAssembler
	spiritSynthesis *SpiritSynthesisService
	orchCache       *biz.OrchestrationCache
	teamStarter     biz.TeamStarterPort
	graphExec       biz.GraphExecutor
	turnTimeout     time.Duration
	skillEvo        *biz.SkillEvolutionUsecase
	evolution       *biz.EvolutionUsecase
	skillStats      biz.SkillInvocationStatsReader
	outboundRouter  *outbound.Router
	subAgentService *subagenttool.Service
	expAnalytics    *biz.ExperienceAnalyticsUsecase

	sweepStop chan struct{}

	// Extracted sub-managers (TECH-DEBT(BL8) resolution).
	sessionStateMgr sessionStateTransitor
	turnMetrics     turnRecorder
	eventPublisher  turnEventPublisher
	runStatus       runStatusTracker
	pendingQ        pendingQueueManager
	awaitCoord      awaitCoordinator
	sessionRunLC    sessionRunLifecycle
	agentBuild      agentBuildDirector
}

// ChatTurnDeps groups turn execution lifecycle dependencies: session pipeline,
// run registry, runtime tooling, admission control, and turn timeout.
type ChatTurnDeps struct {
	rt.TurnDeps
	Runs         *rt.RunRegistry
	PendingQueue *rt.PendingMessageQueue
	RT           RuntimeTooling
	TurnTimeout  time.Duration
	Admission    *biz.TurnAdmissionUsecase
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

	o := &ChatOrchestrator{
		td:              deps.Turn.TurnDeps,
		rt:              deps.Turn.RT,
		team:            deps.Team.Team,
		chJobs:          deps.Channel.ChJobs,
		chNotify:        deps.Channel.ChNotify,
		usage:           deps.Usage.Usage,
		admission:       deps.Turn.Admission,
		monitor:         deps.Usage.Monitor,
		artifacts:       deps.Usage.Artifacts,
		a2aUC:           deps.Infra.A2AUC,
		mcpServers:      deps.Infra.MCPServers,
		runs:            runs,
		chatUC:          NewChatUsecaseFromDeps(runs, pending, sessionLocks, deps.Turn.Sessions, deps.Turn.Pipeline.Bus, deps.Infra.LG),
		lg:              deps.Infra.LG,
		spiritAssembler: deps.Team.SpiritAssembler,
		spiritSynthesis: deps.Team.SpiritSynthesis,
		orchCache:       deps.Infra.OrchCache,
		teamStarter:     deps.Team.TeamStarter,
		graphExec:       deps.Team.GraphExec,
		turnTimeout:     deps.Turn.TurnTimeout,
		skillEvo:        deps.Evolution.SkillEvo,
		evolution:       deps.Evolution.Evolution,
		skillStats:      deps.Usage.SkillStats,
		outboundRouter:  deps.Infra.OutboundRouter,
		subAgentService: deps.Infra.SubAgentService,
		expAnalytics:    deps.Usage.ExpAnalytics,
		sessionStateMgr: deps.Infra.TurnLifecycle,
		turnMetrics:     newChatTurnMetrics(deps.Turn.Sessions, deps.Usage.Usage, deps.Infra.LG),
		eventPublisher:  newChatTurnEventPublisher(deps.Turn.Sessions, deps.Turn.Pipeline.Bus, deps.Infra.LG),
		runStatus:       newChatRunStatusTracker(runs, deps.Turn.Sessions, deps.Turn.Pipeline.Bus, deps.Infra.LG),
	}
	o.pendingQ = newChatPendingQueueManager(o.chatUC)
	o.awaitCoord = newChatAwaitCoordinator(chatAwaitCoordinatorDeps{
		ChatUC:       o.chatUC,
		RunStatus:    o.runStatus,
		SessionState: o.sessionStateMgr,
		SessionRT:    o.sessionRuntime,
		Bus:          deps.Turn.Pipeline.Bus,
		Logger:       deps.Infra.LG,
	})
	o.sessionRunLC = newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns:  deps.Channel.ChJobs.SessionRuns,
		Channels:     deps.Channel.ChJobs.Channels,
		Sessions:     deps.Turn.Sessions,
		RunStatus:    o.runStatus,
		SessionState: o.sessionStateMgr,
		Runs:         runs,
		Escalation:   deps.Channel.ChNotify.RunEscalation,
		Logger:       deps.Infra.LG,
	})
	o.agentBuild = newChatAgentBuildDirector(chatAgentBuildDirectorDeps{
		TurnDeps:    deps.Turn.TurnDeps,
		RT:          deps.Turn.RT,
		AwaitCoord:  o.awaitCoord,
		SubAgentSvc: deps.Infra.SubAgentService,
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
	if o.turnTimeout <= 0 {
		o.turnTimeout = chatagent.DefaultTurnTimeout
	}
	o.admitGate = newTurnAdmissionGate(turn.RunRegistryAdapter{Registry: runs}, o.chatUC, o.pendingQ.SessionPendingMergeFollowup)

	// Wire the threshold resolver so that TurnAdmissionUsecase.EvaluateContextPressure
	// uses the orchestrator's channel-aware threshold lookup policy.
	if o.admission != nil {
		o.admission.SetThresholdResolver(biz.ThresholdResolverFunc(o.resolveContextAdmissionThresholdForSession))
		// Wire the channel config resolver so channel entry points use the
		// long-task config threshold directly instead of the agent L0 threshold.
		if o.sessionRunLC != nil {
			o.admission.SetChannelConfigResolver(biz.ChannelLongTaskConfigResolverFunc(o.sessionRunLC.ResolveChannelLongTaskConfig))
		}
	}

	if deps.Team.Team.TeamsNative != nil {
		deps.Team.Team.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) biz.AwaitReplyFunc {
			return o.awaitCoord.MakeAwaitReplyFunc(runCtx, sessionID, runID)
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

	configureMCPObserve(deps.Turn.TurnDeps.Pipeline.Bus, deps.Infra.MCPServers)
	o.sweepStop = make(chan struct{})
	safego.Go(nil, "orch-map-sweep", o.sweepLoop)
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
	nativeResult, err := o.RunNativeAgentTurnWithOutcome(ctx, input)
	result, classifyErr := turn.ClassifyNativeOutcome(nativeResult, mapTurnExecutorError(err))
	if classifyErr != nil && isTurnMessageQueued(classifyErr) {
		return result, ErrTurnMessageQueued
	}
	return result, classifyErr
}

func mapTurnExecutorError(err error) error {
	if err == nil || !isTurnMessageQueued(err) {
		return err
	}
	return turn.QueuedSentinel
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
	return o.cancelActiveRun(ctx, sessionID)
}

// LastPendingMessageID returns the most recently enqueued pending message id.
func (o *ChatOrchestrator) LastPendingMessageID(sessionID string) string {
	return o.pendingQ.LastPendingMessageID(sessionID)
}

// GetPendingMessages returns pending messages for a session.
func (o *ChatOrchestrator) GetPendingMessages(sessionID string) []biz.PendingQueueEntry {
	return o.pendingQ.GetPendingMessages(sessionID)
}

// CancelPendingMessage cancels a pending message.
func (o *ChatOrchestrator) CancelPendingMessage(sessionID, pendingID string) bool {
	return o.pendingQ.CancelPendingMessage(sessionID, pendingID)
}

// UpdatePendingMessage updates a pending message's content.
func (o *ChatOrchestrator) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return o.pendingQ.UpdatePendingMessage(sessionID, pendingID, content)
}

// EnqueueUserMessage enqueues a user message when a turn is active.
func (o *ChatOrchestrator) EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return o.pendingQ.EnqueueUserMessage(sessionID, content)
}

// SetSessionPendingMergeFollowup toggles followup merge for pending queue enqueues (CH-BOR-01).
func (o *ChatOrchestrator) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	o.pendingQ.SetSessionPendingMergeFollowup(sessionID, merge)
}

// DequeuePendingMessage dequeues the next pending message.
func (o *ChatOrchestrator) DequeuePendingMessage(sessionID string) (biz.PendingQueueEntry, bool) {
	return o.pendingQ.DequeuePendingMessage(sessionID)
}

// GetRunStatus returns the current run lifecycle state for a session.
func (o *ChatOrchestrator) GetRunStatus(ctx context.Context, sessionID string) (runID, status, errMsg string, updatedAt string, ok bool) {
	return o.runStatus.GetRunStatus(ctx, sessionID)
}

// ActiveRunner returns the active runner for a session, if any.
func (o *ChatOrchestrator) ActiveRunner(sessionID string) (runner trpcrunner.Runner, requestID string, active bool) {
	return o.runs.ActiveRunner(sessionID)
}

func (o *ChatOrchestrator) transitionSessionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	o.sessionStateMgr.TransitionStatus(ctx, sessionID, targetStatus, reason)
}

// cancelActiveRun cancels the active run for a session.
func (o *ChatOrchestrator) cancelActiveRun(ctx context.Context, sessionID string) bool {
	if o == nil || sessionID == "" {
		return false
	}
	stopped, runID := o.runs.Cancel(sessionID)
	if !stopped {
		return false
	}
	o.runStatus.SetRunStatus(ctx, sessionID, runID, "cancelled", "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonUserCancelled)
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, o.td.Sessions, sessionID, o.lg); err != nil {
		o.lg.Warn("取消执行卡片查询失败",
			loggateway.StepID("chat.activity.cancel"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
	return true
}

// setRunStatus delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) setRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	o.runStatus.SetRunStatus(ctx, sessionID, runID, status, errMsg)
}

func (o *ChatOrchestrator) setRunStatusWithAwait(ctx context.Context, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	o.runStatus.SetRunStatusWithAwait(ctx, sessionID, runID, status, errMsg, await)
}

func (o *ChatOrchestrator) publishRunStatus(sessionID, runID, status, errMsg string) {
	o.runStatus.PublishRunStatus(sessionID, runID, status, errMsg)
}

func (o *ChatOrchestrator) lockSession(sessionID string) func() {
	return o.chatUC.LockSession(sessionID)
}

// AttachNativeTurnAfterHook sets the post-turn hook.
func (o *ChatOrchestrator) AttachNativeTurnAfterHook(hook biz.NativeTurnAfterHook) {
	if o == nil || hook == nil {
		return
	}
	o.td.AfterTurn = hook
}

// SetTaskOrchestrator sets the TaskOrchestratorPort on the TeamOrchestrationDeps.
// This breaks the Wire injection cycle: TaskOrchestrator → SpiritTeamAssembler → TeamStarterPort → ChatService.
func (o *ChatOrchestrator) SetTaskOrchestrator(orch biz.TaskOrchestratorPort) {
	if o == nil {
		return
	}
	o.team.TaskOrchestrator = orch
}

// AwaitChannel operations delegate to awaitCoord.
func (o *ChatOrchestrator) RegisterAwaitChannel(sessionID string, ch biz.AwaitChannel) {
	o.awaitCoord.RegisterAwaitChannel(sessionID, ch)
}

func (o *ChatOrchestrator) DeleteAwaitChannel(sessionID string) {
	o.awaitCoord.DeleteAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	return o.awaitCoord.LoadAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool {
	return o.awaitCoord.TrySendAwaitChannel(sessionID, msg)
}

// persistRunStatus delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) persistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	o.runStatus.PersistRunStatus(ctx, sessionID, runID, status, errMsg)
}

// hydrateRunStatusFromSession delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return o.runStatus.HydrateRunStatusFromSession(ctx, sessionID)
}

func (o *ChatOrchestrator) persistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool) {
	o.runStatus.PersistAwaitMarkers(ctx, sessionID, runID, await, syncWrite)
}

func (o *ChatOrchestrator) setAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	o.runStatus.SetAwaitMetaCache(sessionID, meta)
}

func (o *ChatOrchestrator) getAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	return o.runStatus.GetAwaitMetaCache(sessionID)
}

func (o *ChatOrchestrator) clearAwaitMetaCache(sessionID string) {
	o.runStatus.ClearAwaitMetaCache(sessionID)
}

func (o *ChatOrchestrator) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	return o.runStatus.ResolveAwaitMeta(ctx, sessionID, status)
}

func (o *ChatOrchestrator) clearAwaitingRunStateSync(ctx context.Context, sessionID string) error {
	return o.runStatus.ClearAwaitingRunStateSync(ctx, sessionID)
}

func (o *ChatOrchestrator) clearAwaitingRunState(ctx context.Context, sessionID string) {
	o.runStatus.ClearAwaitingRunState(ctx, sessionID)
}

// tryBeginResume delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) tryBeginResume(sessionID string) bool {
	return o.awaitCoord.TryBeginResume(sessionID)
}

func (o *ChatOrchestrator) endResume(sessionID string) {
	o.awaitCoord.EndResume(sessionID)
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
	o.runStatus.Sweep()
	o.pendingQ.Sweep()
	o.awaitCoord.Sweep()
}

// publishAwaitResumed delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) publishAwaitResumed(sessionID, runID string) {
	o.awaitCoord.PublishAwaitResumed(sessionID, runID)
}

// sessionAwaitingUser delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) sessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return o.awaitCoord.SessionAwaitingUser(ctx, sessionID)
}

// canResumeAwait delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) canResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	return o.awaitCoord.CanResumeAwait(ctx, sessionID)
}

// hasPendingAwaitUserReplyRoute delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) hasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	return o.awaitCoord.HasPendingAwaitUserReplyRoute(ctx, sessionID)
}

func (o *ChatOrchestrator) sessionRuntime() *araneasession.Runtime {
	if o == nil {
		return nil
	}
	if o.td.SessionRT != nil {
		return o.td.SessionRT
	}
	if o.td.Persist.Session == nil {
		return nil
	}
	return araneasession.NewRuntime(o.td.Persist.Session, o.lg)
}

func (o *ChatOrchestrator) resolveUserID(ctx context.Context, sessionID string) string {
	return ctxuser.TRPCUserKey(ctx)
}
