package service

import (
	"context"
	"strings"
	"sync"
	"time"

	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/debug"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	subagenttool "aranea-agents/internal/tools/subagent"
	araneasession "aranea-agents/internal/session"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/turn"
	"aranea-agents/internal/team"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

const (
	orchMapMaxIdle     = 30 * time.Minute
	orchMapSweepPeriod = 5 * time.Minute
)

type timestampedEntry struct {
	value     any
	createdAt time.Time
}

// RuntimeTooling groups plugin, skill, knowledge, and code-execution dependencies
// that are injected into every agent turn build. Moving these out of the flat
// ChatOrchestratorDeps reduces the Wire parameter count and makes the
// responsibility boundary explicit.
type RuntimeTooling struct {
	PluginRT                   *plugintrpc.Runtime
	PluginManager              *plugintrpc.Manager
	SkillDBRepo                trpcskill.Repository
	KnowledgeRetriever         *knowledge.Retriever
	KnowledgeRouter            *knowledge.AdaptiveRouter
	KnowledgeFederatedRetriever *knowledge.FederatedRetriever
	KnowledgeEvaluator         *knowledge.RetrievalEvaluator
	KnowledgeUC                *biz.KnowledgeUsecase
	CodeExecFactory            *localexec.Factory
	KanbanBridge               kanbanpkg.Bridge
	DebugRecorder              *debug.RecorderFactory
	OrganizationUC           *biz.OrganizationUsecase
	ToolResultGate             *biz.ToolResultGate
	OutboundRouter             *outbound.Router
	SubAgentService            *subagenttool.Service
}

// TeamOrchestrationDeps groups team execution and graph compilation dependencies.
// These are only used when a session is owned by a team or when graph execution
// is triggered from the chat orchestrator.
type TeamOrchestrationDeps struct {
	TeamUC           *biz.TeamUsecase
	TeamsNative      *team.Runner
	GraphFactory     biz.GraphBuilderFactory
	Graphs           *biz.GraphUsecase
	Tasks            *biz.TaskUsecase
	TeamGraphCoord   *team.TeamGraphRunCoordinator
	TeamMediator     *team.TeamRunMediator
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

// ChatOrchestrator owns the turn lifecycle: admission, execution, status tracking,
// and post-turn side effects. ChatService delegates all orchestration work here.
type ChatOrchestrator struct {
	td              rt.TurnDeps
	rt              RuntimeTooling
	team            TeamOrchestrationDeps
	chJobs          ChannelTurnJobDeps
	chNotify        ChannelNotifierDeps
	admitGate       *turn.AdmissionGate
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
	turnTimeout     time.Duration
	skillEvo        *biz.SkillEvolutionUsecase
	evolution       *biz.EvolutionUsecase
	skillStats      biz.SkillInvocationStatsReader
	outboundRouter  *outbound.Router
	subAgentService *subagenttool.Service
	expAnalytics    *biz.ExperienceAnalyticsUsecase

	sessionRunBindings   sync.Map
	awaitMetaCache       sync.Map
	resumeInFlight       sync.Map
	pendingMergeFollowup sync.Map
	sweepStop            chan struct{}
}

// ChatOrchestratorDeps groups all dependencies for ChatOrchestrator construction.
// Sub-aggregates (RuntimeTooling, TeamOrchestrationDeps, ChannelTurnJobDeps, ChannelNotifierDeps) reduce
// the flat parameter count and make responsibility boundaries explicit.
// TECH-DEBT(BL8): ChatOrchestratorDeps 含 20+ 字段，属于上帝对象。
// 应进一步按职责域拆分为更小的功能性依赖组（如 ChatTurnDeps、ChatUsageDeps、ChatChannelDeps），
// 通过 Wire 组合注入。拆分时需确保 ChatOrchestrator 的方法签名不变。
type ChatOrchestratorDeps struct {
	rt.TurnDeps
	Runs            *rt.RunRegistry
	PendingQueue    *rt.PendingMessageQueue
	RT              RuntimeTooling
	Team            TeamOrchestrationDeps
	ChJobs          ChannelTurnJobDeps
	ChNotify        ChannelNotifierDeps
	Usage           *biz.UsageUsecase
	Monitor         *biz.MonitorUsecase
	Artifacts       *biz.ArtifactUsecase
	A2AUC           *biz.A2AUsecase
	MCPServers      *biz.MCPServerUsecase
	LG              loggateway.Logger
	SpiritAssembler *SpiritTeamAssembler
	SpiritSynthesis *SpiritSynthesisService
	OrchCache       *biz.OrchestrationCache
	TeamStarter     biz.TeamStarterPort
	TurnTimeout     time.Duration
	SkillEvo        *biz.SkillEvolutionUsecase
	Evolution       *biz.EvolutionUsecase
	SkillStats      biz.SkillInvocationStatsReader
	OutboundRouter  *outbound.Router
	SubAgentService *subagenttool.Service
	ExpAnalytics    *biz.ExperienceAnalyticsUsecase
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
	runs := coalesceRunRegistry(deps.Runs)
	pending := coalescePendingQueue(deps.PendingQueue)
	sessionLocks := NewSessionLockManager()

	o := &ChatOrchestrator{
		td:              deps.TurnDeps,
		rt:              deps.RT,
		team:            deps.Team,
		chJobs:          deps.ChJobs,
		chNotify:        deps.ChNotify,
		usage:           deps.Usage,
		monitor:         deps.Monitor,
		artifacts:       deps.Artifacts,
		a2aUC:           deps.A2AUC,
		mcpServers:      deps.MCPServers,
		runs:            runs,
		chatUC:          NewChatUsecaseFromDeps(runs, pending, sessionLocks, deps.Sessions, deps.Pipeline.Bus, deps.LG),
		lg:              deps.LG,
		spiritAssembler: deps.SpiritAssembler,
		spiritSynthesis: deps.SpiritSynthesis,
		orchCache:       deps.OrchCache,
		teamStarter:     deps.TeamStarter,
		turnTimeout:     deps.TurnTimeout,
		skillEvo:        deps.SkillEvo,
		evolution:       deps.Evolution,
		skillStats:      deps.SkillStats,
		outboundRouter:  deps.OutboundRouter,
		subAgentService: deps.SubAgentService,
		expAnalytics:    deps.ExpAnalytics,
	}
	if o.turnTimeout <= 0 {
		o.turnTimeout = chatagent.DefaultTurnTimeout
	}
	o.admitGate = newTurnAdmissionGate(turn.RunRegistryAdapter{Registry: runs}, o.chatUC, o.sessionPendingMergeFollowup)

	if deps.Team.TeamsNative != nil {
		deps.Team.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc {
			return o.makeAwaitReplyFunc(runCtx, sessionID, runID)
		})
		if deps.Team.TeamMediator != nil {
			deps.Team.TeamsNative.SetMediator(deps.Team.TeamMediator)
			deps.Team.TeamMediator.SetFinisher(deps.Team.TeamsNative)
			if deps.Team.TeamGraphCoord != nil {
				deps.Team.TeamMediator.SetCoordinator(deps.Team.TeamGraphCoord)
				deps.Team.TeamGraphCoord.SetFinisher(deps.Team.TeamMediator)
				deps.Team.TeamGraphCoord.RecoverSessions(context.Background())
			}
		}
	}

	configureMCPObserve(deps.TurnDeps.Pipeline.Bus, deps.MCPServers)
	o.sweepStop = make(chan struct{})
	safego.Go(nil, "orch-map-sweep", o.sweepLoop)
	return o
}

// Compile-time interface assertions.
var (
	_ biz.TurnExecutor           = (*ChatOrchestrator)(nil)
	_ biz.NativeTurnGateway      = (*ChatService)(nil)
	_ biz.TurnExecutorGateway    = (*ChatService)(nil)
	_ biz.TurnRunControlGateway  = (*ChatService)(nil)
	_ biz.TurnGateway            = (*ChatService)(nil)
	_ biz.TurnControlGateway     = (*ChatService)(nil)
	_ biz.DurableResumeGateway   = (*ChatService)(nil)
	_ biz.PendingQueueGateway    = (*ChatService)(nil)
	_ biz.A2ARunnerFactory       = (*ChatService)(nil)
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
	if o == nil || o.chatUC == nil {
		return ""
	}
	entries := o.chatUC.GetPendingMessages(sessionID)
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].ID
}

// GetPendingMessages returns pending messages for a session.
func (o *ChatOrchestrator) GetPendingMessages(sessionID string) []biz.PendingQueueEntry {
	return o.chatUC.GetPendingMessages(sessionID)
}

// CancelPendingMessage cancels a pending message.
func (o *ChatOrchestrator) CancelPendingMessage(sessionID, pendingID string) bool {
	return o.chatUC.CancelPendingMessage(sessionID, pendingID)
}

// UpdatePendingMessage updates a pending message's content.
func (o *ChatOrchestrator) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return o.chatUC.UpdatePendingMessage(sessionID, pendingID, content)
}

// EnqueueUserMessage enqueues a user message when a turn is active.
func (o *ChatOrchestrator) EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return o.chatUC.EnqueueUserMessage(sessionID, content, o.sessionPendingMergeFollowup(sessionID))
}

// SetSessionPendingMergeFollowup toggles followup merge for pending queue enqueues (CH-BOR-01).
func (o *ChatOrchestrator) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || o == nil {
		return
	}
	if merge {
		o.pendingMergeFollowup.Store(sessionID, timestampedEntry{value: true, createdAt: time.Now()})
	} else {
		o.pendingMergeFollowup.Delete(sessionID)
	}
}

func (o *ChatOrchestrator) sessionPendingMergeFollowup(sessionID string) bool {
	if o == nil {
		return false
	}
	v, ok := o.pendingMergeFollowup.Load(strings.TrimSpace(sessionID))
	if !ok {
		return false
	}
	te, ok := v.(timestampedEntry)
	if !ok {
		return false
	}
	b, _ := te.value.(bool)
	return b
}

// DequeuePendingMessage dequeues the next pending message.
func (o *ChatOrchestrator) DequeuePendingMessage(sessionID string) (biz.PendingQueueEntry, bool) {
	return o.chatUC.DequeuePendingMessage(sessionID)
}

// GetRunStatus returns the current run lifecycle state for a session.
func (o *ChatOrchestrator) GetRunStatus(ctx context.Context, sessionID string) (runID, status, errMsg string, updatedAt string, ok bool) {
	if entry, ok2 := o.runs.GetStatus(sessionID); ok2 {
		ua := ""
		if !entry.UpdatedAt.IsZero() {
			ua = entry.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		return entry.RunID, entry.Status, entry.ErrMsg, ua, true
	}
	return "", "", "", "", false
}

// ActiveRunner returns the active runner for a session, if any.
func (o *ChatOrchestrator) ActiveRunner(sessionID string) (runner trpcrunner.Runner, requestID string, active bool) {
	return o.runs.ActiveRunner(sessionID)
}

func (o *ChatOrchestrator) transitionSessionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	if o == nil || o.td.Sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if err := o.td.Sessions.TransitionStatus(ctx, sessionID, targetStatus, reason); err != nil {
		o.lg.Warn("session status transition failed",
			loggateway.StepID("chat.transition_status"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("target_status", string(targetStatus)),
			loggateway.Str("reason", string(reason)),
			loggateway.Err(err),
		)
	}
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
	o.setRunStatus(ctx, sessionID, runID, "cancelled", "")
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

// setRunStatus atomically updates the run status and publishes a WS envelope.
func (o *ChatOrchestrator) setRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	o.setRunStatusWithAwait(ctx, sessionID, runID, status, errMsg, nil)
}

func (o *ChatOrchestrator) setRunStatusWithAwait(ctx context.Context, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	o.runs.SetStatus(sessionID, runID, status, errMsg)
	bind, _ := o.sessionRunBinding(sessionID)
	if await != nil {
		PublishRunStatusFull(o.td.Pipeline.Bus, sessionID, runID, status, errMsg, await, bind.sessionRunID, bind.turnID)
	} else {
		PublishRunStatusFull(o.td.Pipeline.Bus, sessionID, runID, status, errMsg, nil, bind.sessionRunID, bind.turnID)
	}
	o.persistRunStatus(ctx, sessionID, runID, status, errMsg)
}

func (o *ChatOrchestrator) publishRunStatus(sessionID, runID, status, errMsg string) {
	PublishRunStatus(o.td.Pipeline.Bus, sessionID, runID, status, errMsg)
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

// AwaitChannel operations delegate to chatUC.
func (o *ChatOrchestrator) RegisterAwaitChannel(sessionID string, ch biz.AwaitChannel) {
	o.chatUC.RegisterAwaitChannel(sessionID, ch)
}

func (o *ChatOrchestrator) DeleteAwaitChannel(sessionID string) {
	o.chatUC.DeleteAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	return o.chatUC.LoadAwaitChannel(sessionID)
}

// persistRunStatus persists run status to session state.
func (o *ChatOrchestrator) persistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	persistRunStatusToSession(o.td.Sessions, ctx, sessionID, runID, status, errMsg)
}

// hydrateRunStatusFromSession loads run status from session state.
func (o *ChatOrchestrator) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	if o == nil || o.td.Sessions == nil {
		return persistedRunStatus{}, false
	}
	state, err := o.td.Sessions.GetSessionState(ctx, sessionID)
	if err != nil || len(state) == 0 {
		return persistedRunStatus{}, false
	}
	status := strings.TrimSpace(state[stateKeyRunStatus])
	if status == "" {
		return persistedRunStatus{}, false
	}
	return persistedRunStatus{
		RunID:           strings.TrimSpace(state[stateKeyRunID]),
		Status:          status,
		ErrorMessage:    strings.TrimSpace(state[stateKeyRunError]),
		UpdatedAt:       strings.TrimSpace(state[stateKeyRunUpdatedAt]),
		AwaitKind:       strings.TrimSpace(state[stateKeyAwaitKind]),
		AwaitToolKey:    strings.TrimSpace(state[stateKeyAwaitToolKey]),
		AwaitToolCallID: strings.TrimSpace(state[stateKeyAwaitToolCallID]),
	}, true
}

func (o *ChatOrchestrator) persistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool) {
	o.setAwaitMetaCache(sessionID, await)
	persistAwaitMarkersToSession(o.td.Sessions, ctx, sessionID, runID, await, syncWrite, o.lg)
}

func (o *ChatOrchestrator) setAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	o.awaitMetaCache.Store(sessionID, timestampedEntry{value: meta, createdAt: time.Now()})
}

func (o *ChatOrchestrator) getAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	v, ok := o.awaitMetaCache.Load(strings.TrimSpace(sessionID))
	if !ok {
		return biz.ChatAwaitMeta{}, false
	}
	te, ok := v.(timestampedEntry)
	if !ok {
		return biz.ChatAwaitMeta{}, false
	}
	meta, ok := te.value.(biz.ChatAwaitMeta)
	return meta, ok
}

func (o *ChatOrchestrator) clearAwaitMetaCache(sessionID string) {
	o.awaitMetaCache.Delete(strings.TrimSpace(sessionID))
}

func (o *ChatOrchestrator) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	if strings.TrimSpace(status) != "awaiting_user" {
		return biz.ChatAwaitMeta{}
	}
	if meta, ok := o.getAwaitMetaCache(sessionID); ok {
		return meta
	}
	if snap, ok := o.hydrateRunStatusFromSession(ctx, sessionID); ok {
		return biz.ChatAwaitMeta{
			Kind:       snap.AwaitKind,
			ToolKey:    snap.AwaitToolKey,
			ToolCallID: snap.AwaitToolCallID,
		}
	}
	return biz.ChatAwaitMeta{}
}

func (o *ChatOrchestrator) clearAwaitingRunStateSync(ctx context.Context, sessionID string) error {
	if o == nil || o.td.Sessions == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	state, err := o.td.Sessions.GetSessionState(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(state) == 0 {
		return nil
	}
	o.clearAwaitMetaCache(sessionID)
	delete(state, stateKeyRunID)
	delete(state, stateKeyRunStatus)
	delete(state, stateKeyRunError)
	delete(state, stateKeyRunUpdatedAt)
	delete(state, stateKeyAwaitRunID)
	delete(state, stateKeyAwaitSince)
	delete(state, stateKeyAwaitKind)
	delete(state, stateKeyAwaitToolKey)
	delete(state, stateKeyAwaitToolCallID)
	return o.td.Sessions.SaveSessionState(ctx, sessionID, state)
}

func (o *ChatOrchestrator) clearAwaitingRunState(ctx context.Context, sessionID string) {
	o.clearAwaitMetaCache(sessionID)
	clearAwaitingRunStateFromSession(o.td.Sessions, ctx, sessionID, o.lg)
}

// tryBeginResume guards against concurrent await resumes.
func (o *ChatOrchestrator) tryBeginResume(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	_, loaded := o.resumeInFlight.LoadOrStore(sessionID, timestampedEntry{value: struct{}{}, createdAt: time.Now()})
	return !loaded
}

func (o *ChatOrchestrator) endResume(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	o.resumeInFlight.Delete(sessionID)
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
	now := time.Now()
	o.awaitMetaCache.Range(func(key, value any) bool {
		if te, ok := value.(timestampedEntry); ok && now.Sub(te.createdAt) > orchMapMaxIdle {
			o.awaitMetaCache.Delete(key)
		}
		return true
	})
	o.pendingMergeFollowup.Range(func(key, value any) bool {
		if te, ok := value.(timestampedEntry); ok && now.Sub(te.createdAt) > orchMapMaxIdle {
			o.pendingMergeFollowup.Delete(key)
		}
		return true
	})
	o.resumeInFlight.Range(func(key, value any) bool {
		if te, ok := value.(timestampedEntry); ok && now.Sub(te.createdAt) > orchMapMaxIdle {
			o.resumeInFlight.Delete(key)
		}
		return true
	})
	o.sessionRunBindings.Range(func(key, value any) bool {
		if te, ok := value.(timestampedEntry); ok && now.Sub(te.createdAt) > orchMapMaxIdle {
			o.sessionRunBindings.Delete(key)
		}
		return true
	})
}

func (o *ChatOrchestrator) publishAwaitResumed(sessionID, runID string) {
	bus := o.td.Pipeline.Bus
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "chat-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"run_id":        runID,
		"status":        "running",
		"await_resumed": true,
	}
	bus.Publish(context.Background(), env)
}

func (o *ChatOrchestrator) sessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	snap, ok := o.hydrateRunStatusFromSession(ctx, sessionID)
	if !ok {
		return persistedRunStatus{}, false
	}
	if strings.TrimSpace(strings.ToLower(snap.Status)) != "awaiting_user" {
		return persistedRunStatus{}, false
	}
	return snap, true
}

// canResumeAwait reports whether a cross-process await resume is allowed.
func (o *ChatOrchestrator) canResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	if snap, awaiting := o.sessionAwaitingUser(ctx, sessionID); awaiting {
		return strings.TrimSpace(snap.RunID), true
	}
	if o.hasPendingAwaitUserReplyRoute(ctx, sessionID) {
		if snap, ok := o.hydrateRunStatusFromSession(ctx, sessionID); ok {
			return strings.TrimSpace(snap.RunID), true
		}
		return "", true
	}
	return "", false
}

func (o *ChatOrchestrator) hasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	rtPort := o.sessionRuntime()
	if rtPort == nil {
		return false
	}
	userID := o.resolveUserID(ctx, sessionID)
	if userID == "" {
		return false
	}
	return rtPort.HasPendingAwaitUserReply(ctx, userID, sessionID)
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
