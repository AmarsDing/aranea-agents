package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	araneasession "aranea-agents/internal/session"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// AwaitResumeGuard prevents concurrent await resume.
// Stability:evolving
type AwaitResumeGuard interface {
	TryBeginResume(sessionID string) bool
	EndResume(sessionID string)
}

// AwaitChannelRegistry manages await channels.
// Stability:evolving
type AwaitChannelRegistry interface {
	RegisterAwaitChannel(sessionID string, ch biz.AwaitChannel)
	DeleteAwaitChannel(sessionID string)
	LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool)
	TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool
}

// AwaitReplyBuilder creates await_user_reply callback functions.
// Stability:evolving
type AwaitReplyBuilder interface {
	MakeAwaitReplyFunc(runCtx context.Context, sessionID, runID string) func(context.Context) (string, error)
}

// AwaitResumption handles await state checking and resumption.
// Stability:evolving
type AwaitResumption interface {
	PublishAwaitResumed(sessionID, runID string)
	SessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool)
	CanResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool)
	ResumeAwaitAfterRestart(ctx context.Context, sessionID, reply, runID string) error
	HasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool
}

// awaitCoordinator is the composite interface.
// Stability:evolving
type awaitCoordinator interface {
	AwaitResumeGuard
	AwaitChannelRegistry
	AwaitReplyBuilder
	AwaitResumption
	Sweep()
}

// chatAwaitCoordinator implements awaitCoordinator.
//
// Part of the TECH-DEBT(BL8) resolution: extracting await/resume coordination
// from ChatOrchestrator to reduce cognitive complexity (AS-COG-01).
//
// Phase 3b-D Task 10: migrated from v1 ActivityEventBus to v2 EventBus.
// PublishAwaitResumed now emits biz.NewStepCreatedEvent (Kind=StepKindNotice).
type chatAwaitCoordinator struct {
	chatUC         *biz.ChatUsecase
	runStatus      runStatusTracker
	sessionState   sessionStateTransitor
	sessionRT      func() *araneasession.Runtime // lazy accessor
	eventBus       biz.EventBus
	seq            rt.EventPublisher
	resumeInFlight *TypedSyncMap[string, struct{}]
	lg             loggateway.Logger
}

// chatAwaitCoordinatorDeps aggregates constructor dependencies for chatAwaitCoordinator.
// Introduced to satisfy BI1 (parameter count ≤ 5) for the constructor.
// Stability:internal
type chatAwaitCoordinatorDeps struct {
	ChatUC       *biz.ChatUsecase
	RunStatus    runStatusTracker
	SessionState sessionStateTransitor
	SessionRT    func() *araneasession.Runtime
	EventBus     biz.EventBus
	Seq          rt.EventPublisher
	Logger       loggateway.Logger
}

func newChatAwaitCoordinator(d chatAwaitCoordinatorDeps) *chatAwaitCoordinator {
	return &chatAwaitCoordinator{
		chatUC:         d.ChatUC,
		runStatus:      d.RunStatus,
		sessionState:   d.SessionState,
		sessionRT:      d.SessionRT,
		eventBus:       d.EventBus,
		seq:            d.Seq,
		resumeInFlight: NewTypedSyncMap[string, struct{}](orchMapMaxIdle),
		lg:             d.Logger,
	}
}

// Compile-time interface check.
var _ awaitCoordinator = (*chatAwaitCoordinator)(nil)

// TryBeginResume guards against concurrent await resumes.
func (a *chatAwaitCoordinator) TryBeginResume(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	_, loaded := a.resumeInFlight.LoadOrStore(sessionID, struct{}{})
	return !loaded
}

// EndResume clears the resuming mark.
func (a *chatAwaitCoordinator) EndResume(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	a.resumeInFlight.Delete(sessionID)
}

// PublishAwaitResumed publishes an await_resumed v2 StepCreatedEvent
// (Kind=StepKindNotice). Replaces the legacy v1 ActivityEvent (Kind=notice)
// and the legacy EnvelopeTypeRunStatus publish.
//
// Phase 3b-D Task 10: the original v1 event carried run_id/status/await_resumed/
// source in Activity.Meta. The v2 Step entity has no Meta field, so these
// details are dropped. runID was already published via PublishRunStatus
// (run_status_publish.go, owned by Task 9) before this call, so the run
// status is independently delivered to WS clients.
func (a *chatAwaitCoordinator) PublishAwaitResumed(sessionID, runID string) {
	if a.seq == nil && a.eventBus == nil {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	step := biz.Step{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Kind:            biz.StepKindNotice,
		NoticeType:      "info",
		Content:         "已恢复运行",
		Status:          biz.StepStatusRunning,
		AuthorAgentKey:  "chat-service",
	}
	ev := biz.NewStepCreatedEvent(step)
	if a.seq != nil {
		a.seq.Publish(context.Background(), ev)
		return
	}
	a.eventBus.Publish(context.Background(), ev)
}

// SessionAwaitingUser checks if a session is in awaiting_user state.
func (a *chatAwaitCoordinator) SessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	snap, ok := a.runStatus.HydrateRunStatusFromSession(ctx, sessionID)
	if !ok {
		return persistedRunStatus{}, false
	}
	if strings.TrimSpace(strings.ToLower(snap.Status)) != "awaiting_user" {
		return persistedRunStatus{}, false
	}
	return snap, true
}

// CanResumeAwait reports whether a cross-process await resume is allowed.
func (a *chatAwaitCoordinator) CanResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	if snap, awaiting := a.SessionAwaitingUser(ctx, sessionID); awaiting {
		return strings.TrimSpace(snap.RunID), true
	}
	if a.hasPendingAwaitUserReplyRoute(ctx, sessionID) {
		if snap, ok := a.runStatus.HydrateRunStatusFromSession(ctx, sessionID); ok {
			return strings.TrimSpace(snap.RunID), true
		}
		return "", true
	}
	return "", false
}

func (a *chatAwaitCoordinator) hasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	rtPort := a.sessionRT()
	if rtPort == nil {
		return false
	}
	userID := ctxuser.TRPCUserKey(ctx)
	if userID == "" {
		return false
	}
	return rtPort.HasPendingAwaitUserReply(ctx, userID, sessionID)
}

// ResumeAwaitAfterRestart resumes an await after process restart.
func (a *chatAwaitCoordinator) ResumeAwaitAfterRestart(ctx context.Context, sessionID, reply, runID string) error {
	if !a.TryBeginResume(sessionID) {
		return errResumeInFlight
	}
	if err := a.runStatus.ClearAwaitingRunStateSync(ctx, sessionID); err != nil {
		a.EndResume(sessionID)
		return err
	}
	a.PublishAwaitResumed(sessionID, runID)
	// NOTE: The actual turn re-execution is handled by the caller (ChatOrchestrator)
	// because it needs access to the full turn execution pipeline.
	// This method only handles the coordination state.
	return nil
}

// RegisterAwaitChannel registers an await channel for a session.
func (a *chatAwaitCoordinator) RegisterAwaitChannel(sessionID string, ch biz.AwaitChannel) {
	a.chatUC.RegisterAwaitChannel(sessionID, ch)
}

// DeleteAwaitChannel removes an await channel for a session.
func (a *chatAwaitCoordinator) DeleteAwaitChannel(sessionID string) {
	a.chatUC.DeleteAwaitChannel(sessionID)
}

// LoadAwaitChannel loads an await channel for a session.
func (a *chatAwaitCoordinator) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	return a.chatUC.LoadAwaitChannel(sessionID)
}

// TrySendAwaitChannel tries to send a message to an await channel.
func (a *chatAwaitCoordinator) TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool {
	return a.chatUC.TrySendAwaitChannel(sessionID, msg)
}

// MakeAwaitReplyFunc creates the await_user_reply callback function.
func (a *chatAwaitCoordinator) MakeAwaitReplyFunc(runCtx context.Context, sessionID, runID string) func(context.Context) (string, error) {
	return func(toolCtx context.Context) (string, error) {
		ch := make(biz.AwaitChannel, 1)
		a.chatUC.RegisterAwaitChannel(sessionID, ch)
		awaitMeta := AwaitStatusMeta{Kind: biz.ChatAwaitKindReply}
		if req, ok := serviceawaitreply.ToolConfirmRequestFromContext(toolCtx); ok {
			awaitMeta = AwaitStatusMeta{
				Kind:       biz.ChatAwaitKindToolConfirm,
				ToolKey:    req.ToolKey,
				ToolCallID: req.ToolCallID,
			}
		}
		if err := a.runStatus.SetRunStatusWithAwait(toolCtx, sessionID, runID, "awaiting_user", "", &awaitMeta); err != nil {
			a.lg.Warn("set run status awaiting_user failed",
				loggateway.StepID("chat.await"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(err))
		}
		if awaitMeta.Kind == biz.ChatAwaitKindToolConfirm {
			a.sessionState.TransitionStatus(toolCtx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonToolConfirmation)
		} else {
			a.sessionState.TransitionStatus(toolCtx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonAgentAwaitingReply)
		}
		a.runStatus.PersistAwaitMarkers(toolCtx, sessionID, runID, awaitMeta, true)
		defer func() {
			a.chatUC.DeleteAwaitChannel(sessionID)
			a.runStatus.ClearAwaitMetaCache(sessionID)
			if err := a.runStatus.SetRunStatus(toolCtx, sessionID, runID, "running", ""); err != nil {
				a.lg.Warn("set run status running on resume failed",
					loggateway.StepID("chat.await_resume"),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("run_id", runID),
					loggateway.Err(err))
			}
			a.sessionState.TransitionStatus(toolCtx, sessionID, sessstatus.SessionStatusRunning, "")
		}()
		select {
		case r, ok := <-ch:
			if !ok {
				return "", toolCtx.Err()
			}
			return r.Reply, nil
		case <-toolCtx.Done():
			return "", toolCtx.Err()
		case <-runCtx.Done():
			return "", runCtx.Err()
		}
	}
}

// HasPendingAwaitUserReplyRoute checks if a session has a pending await_user_reply route.
func (a *chatAwaitCoordinator) HasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	return a.hasPendingAwaitUserReplyRoute(ctx, sessionID)
}

// Sweep removes expired entries from the resume-in-flight map.
func (a *chatAwaitCoordinator) Sweep() {
	a.resumeInFlight.Sweep()
}
