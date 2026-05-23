package service

import (
	"context"
	"strings"
	"sync"

	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/event"
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/knowledge"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/team"
	tooltrpc "aranea-agents/internal/tools/trpc"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// ChatOrchestrator owns the turn lifecycle: admission, execution, status tracking,
// and post-turn side effects. ChatService delegates all orchestration work here.
type ChatOrchestrator struct {
	teams              biz.TeamRepository
	teamsNative        *team.Runner
	usage              *biz.UsageUsecase
	monitor            *biz.MonitorUsecase
	td                 rt.TurnDeps
	pluginRT           *plugintrpc.Runtime
	pluginManager      *plugintrpc.Manager
	skillDBRepo        trpcskill.Repository
	artifacts          *biz.ArtifactUsecase
	runs               *rt.RunRegistry
	chatUC             *biz.ChatUsecase
	turnJobs           *biz.ChannelTurnJobUsecase
	awaitMetaCache     sync.Map
	resumeInFlight     sync.Map
	a2aUC              *biz.A2AUsecase
	knowledgeRetriever *knowledge.Retriever
	codeExecFactory    *localexec.Factory
}

// ChatOrchestratorDeps groups all dependencies for ChatOrchestrator construction.
type ChatOrchestratorDeps struct {
	rt.TurnDeps
	Runs               *rt.RunRegistry
	PendingQueue       *rt.PendingMessageQueue
	Teams              biz.TeamRepository
	TeamsNative        *team.Runner
	Usage              *biz.UsageUsecase
	Monitor            *biz.MonitorUsecase
	PluginRT           *plugintrpc.Runtime
	PluginManager      *plugintrpc.Manager
	SkillDBRepo        trpcskill.Repository
	Artifacts          *biz.ArtifactUsecase
	A2AUC              *biz.A2AUsecase
	KnowledgeRetriever *knowledge.Retriever
	CodeExecFactory    *localexec.Factory
	MCPServers         *biz.MCPServerUsecase
	GraphFactory       biz.GraphBuilderFactory
	Graphs             *biz.GraphUsecase
	Tasks              *biz.TaskUsecase
	TeamGraphCoord     *team.TeamGraphRunCoordinator
	TurnJobs           *biz.ChannelTurnJobUsecase
}

func NewChatOrchestrator(deps ChatOrchestratorDeps) *ChatOrchestrator {
	runs := coalesceRunRegistry(deps.Runs)
	pending := coalescePendingQueue(deps.PendingQueue)
	sessionLocks := NewSessionLockManager()

	o := &ChatOrchestrator{
		teams:              deps.Teams,
		teamsNative:        deps.TeamsNative,
		usage:              deps.Usage,
		monitor:            deps.Monitor,
		pluginRT:           deps.PluginRT,
		pluginManager:      deps.PluginManager,
		skillDBRepo:        deps.SkillDBRepo,
		artifacts:          deps.Artifacts,
		runs:               runs,
		chatUC:             NewChatUsecaseFromDeps(runs, pending, sessionLocks, deps.Sessions, deps.Pipeline.Bus),
		turnJobs:           deps.TurnJobs,
		a2aUC:              deps.A2AUC,
		knowledgeRetriever: deps.KnowledgeRetriever,
		codeExecFactory:    deps.CodeExecFactory,
		td:                 deps.TurnDeps,
	}

	if deps.TeamsNative != nil {
		deps.TeamsNative.SetKnowledgeRetriever(deps.KnowledgeRetriever)
		deps.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc {
			return o.makeAwaitReplyFunc(runCtx, sessionID, runID)
		})
		deps.TeamsNative.SetRunRegistry(o.runs)
		if deps.Graphs != nil {
			deps.TeamsNative.SetGraphBuildConfigLoader(graphadapter.NewLinkedGraphBuildConfigLoader(deps.Graphs))
		}
		if deps.GraphFactory != nil {
			if builder, ok := deps.GraphFactory.(graphadapter.TeamGraphRootBuilder); ok {
				deps.TeamsNative.SetGraphRootBuilder(builder)
			}
		}
		if deps.Tasks != nil {
			deps.TeamsNative.SetTeamGraphTaskCreator(team.NewTaskUsecaseGraphTaskCreator(deps.Tasks))
		}
		if deps.TeamGraphCoord != nil {
			deps.TeamsNative.SetTeamGraphRunCoordinator(deps.TeamGraphCoord)
			deps.TeamGraphCoord.SetFinisher(deps.TeamsNative)
		}
	}

	configureMCPObserve(deps.TurnDeps.Pipeline.Bus, deps.MCPServers)
	return o
}

// RunGateway exposes the shared session run registry.
func (o *ChatOrchestrator) RunGateway() rt.RunGateway {
	return o.runs
}

// HasActiveRun reports whether a session has an in-flight run.
func (o *ChatOrchestrator) HasActiveRun(sessionID string) bool {
	return o.runs.HasActive(sessionID)
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
	return o.chatUC.EnqueueUserMessage(sessionID, content)
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
func (o *ChatOrchestrator) ActiveRunner(sessionID string) (runner interface{}, requestID string, active bool) {
	return o.runs.ActiveRunner(sessionID)
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
	o.setRunStatus(sessionID, runID, "cancelled", "")
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, o.td.Sessions, sessionID); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.activity.cancel", "取消执行卡片查询失败",
			event.P("session_id", sessionID),
			event.P("error", err.Error()),
		)
	}
	return true
}

// setRunStatus atomically updates the run status and publishes a WS envelope.
func (o *ChatOrchestrator) setRunStatus(sessionID, runID, status, errMsg string) {
	o.setRunStatusWithAwait(sessionID, runID, status, errMsg, nil)
}

func (o *ChatOrchestrator) setRunStatusWithAwait(sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	o.runs.SetStatus(sessionID, runID, status, errMsg)
	if await != nil {
		PublishRunStatusMeta(o.td.Pipeline.Bus, sessionID, runID, status, errMsg, await)
	} else {
		PublishRunStatus(o.td.Pipeline.Bus, sessionID, runID, status, errMsg)
	}
	o.persistRunStatus(context.Background(), sessionID, runID, status, errMsg)
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
func (o *ChatOrchestrator) RegisterAwaitChannel(sessionID string, ch interface{}) {
	o.chatUC.RegisterAwaitChannel(sessionID, ch)
}

func (o *ChatOrchestrator) DeleteAwaitChannel(sessionID string) {
	o.chatUC.DeleteAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) LoadAwaitChannel(sessionID string) (interface{}, bool) {
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
	persistAwaitMarkersToSession(o.td.Sessions, ctx, sessionID, runID, await, syncWrite)
}

func (o *ChatOrchestrator) setAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	o.awaitMetaCache.Store(sessionID, meta)
}

func (o *ChatOrchestrator) getAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	v, ok := o.awaitMetaCache.Load(strings.TrimSpace(sessionID))
	if !ok {
		return biz.ChatAwaitMeta{}, false
	}
	meta, ok := v.(biz.ChatAwaitMeta)
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
	clearAwaitingRunStateFromSession(o.td.Sessions, ctx, sessionID)
}

// tryBeginResume guards against concurrent await resumes.
func (o *ChatOrchestrator) tryBeginResume(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	_, loaded := o.resumeInFlight.LoadOrStore(sessionID, struct{}{})
	return !loaded
}

func (o *ChatOrchestrator) endResume(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	o.resumeInFlight.Delete(sessionID)
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
	if o == nil || o.td.Persist.Session == nil {
		return false
	}
	userID := o.resolveUserID(ctx, sessionID)
	if userID == "" {
		return false
	}
	sess, err := o.td.Persist.Session.GetSession(ctx, trpcsession.Key{
		AppName:   chatagent.TRPCDefaultAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil || sess == nil {
		return false
	}
	_, pending, err := trpcagent.PendingAwaitUserReplyRoute(sess)
	return err == nil && pending
}

func (o *ChatOrchestrator) resolveUserID(ctx context.Context, sessionID string) string {
	if uid := strings.TrimSpace(chatagent.UserIDFromCtx(ctx)); uid != "" {
		return uid
	}
	if o == nil || o.td.Sessions == nil {
		return ""
	}
	sess, err := o.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sess.UserID)
}
