package service

import (
	"context"
	"reflect"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// isNilInterface checks whether an interface value is nil, including typed-nil
// pointers (e.g. (*SessionUsecase)(nil) stored in an interface).
func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

type runGatewayAdapter struct {
	*runtime.RunRegistry
}

func (a *runGatewayAdapter) GetStatus(sessionID string) (biz.ChatRunStatus, bool) {
	entry, ok := a.RunRegistry.GetStatus(sessionID)
	if !ok {
		return biz.ChatRunStatus{}, false
	}
	return biz.ChatRunStatus{
		RunID:     entry.RunID,
		Status:    entry.Status,
		ErrMsg:    entry.ErrMsg,
		UpdatedAt: entry.UpdatedAt,
	}, true
}

func NewRunGatewayAdapter(r *runtime.RunRegistry) biz.ChatRunGateway {
	return &runGatewayAdapter{RunRegistry: r}
}

type pendingQueueAdapter struct {
	*runtime.PendingMessageQueue
}

func (a *pendingQueueAdapter) List(sessionID string) []biz.PendingQueueEntry {
	entries := a.PendingMessageQueue.List(sessionID)
	out := make([]biz.PendingQueueEntry, len(entries))
	for i, e := range entries {
		out[i] = biz.PendingQueueEntry{
			ID:        e.ID,
			Content:   e.Content,
			Status:    e.Status,
			CreatedAt: e.CreatedAt,
		}
	}
	return out
}

func (a *pendingQueueAdapter) Enqueue(sessionID, content string) string {
	return a.PendingMessageQueue.Enqueue(sessionID, content)
}

func (a *pendingQueueAdapter) EnqueueFollowup(sessionID, content string) string {
	return a.PendingMessageQueue.EnqueueFollowup(sessionID, content, "\n")
}

func (a *pendingQueueAdapter) Dequeue(sessionID string) (biz.PendingQueueEntry, bool) {
	e, ok := a.PendingMessageQueue.Dequeue(sessionID)
	if !ok {
		return biz.PendingQueueEntry{}, false
	}
	return biz.PendingQueueEntry{
		ID:        e.ID,
		Content:   e.Content,
		Status:    e.Status,
		CreatedAt: e.CreatedAt,
	}, true
}

func (a *pendingQueueAdapter) Peek(sessionID string) (biz.PendingQueueEntry, bool) {
	e, ok := a.PendingMessageQueue.Peek(sessionID)
	if !ok {
		return biz.PendingQueueEntry{}, false
	}
	return biz.PendingQueueEntry{
		ID:        e.ID,
		Content:   e.Content,
		Status:    e.Status,
		CreatedAt: e.CreatedAt,
	}, true
}

func (a *pendingQueueAdapter) Close() {
	a.PendingMessageQueue.Close()
}

func NewPendingQueueAdapter(q *runtime.PendingMessageQueue) biz.ChatPendingQueue {
	return &pendingQueueAdapter{PendingMessageQueue: q}
}

type chatRunStatusPersister struct {
	sessions biz.SessionStatePort
	lg       loggateway.Logger
}

func (p *chatRunStatusPersister) PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error {
	return persistRunStatusToSession(p.sessions, ctx, sessionID, runID, status, errMsg)
}

func (p *chatRunStatusPersister) PersistAwaitMarkers(ctx context.Context, sessionID, runID string, meta biz.ChatAwaitMeta) {
	persistAwaitMarkersToSession(p.sessions, ctx, sessionID, runID, meta, false, p.lg)
}

func (p *chatRunStatusPersister) ClearAwaitingRunState(ctx context.Context, sessionID string) {
	clearAwaitingRunStateFromSession(p.sessions, ctx, sessionID, p.lg)
}

func NewChatRunStatusPersister(sessions biz.SessionStatePort, lg loggateway.Logger) biz.ChatRunStatusPersister {
	return &chatRunStatusPersister{sessions: sessions, lg: lg}
}

type chatEventPublisher struct {
	activityBus biz.ActivityEventBus
}

func (pub *chatEventPublisher) PublishRunStatus(sessionID, runID, status, errMsg string) {
	PublishRunStatus(pub.activityBus, sessionID, runID, status, errMsg)
}

func (pub *chatEventPublisher) PublishMessageQueued(sessionID string) {
	publishMessageQueuedToBus(pub.activityBus, sessionID)
}

func NewChatEventPublisher(activityBus biz.ActivityEventBus) biz.ChatEventPublisher {
	return &chatEventPublisher{activityBus: activityBus}
}

// NewChatUsecaseFromDeps wires the shared run registry, pending queue, and session
// lock into a single Biz orchestration entrypoint for ChatService.
func NewChatUsecaseFromDeps(
	runs *runtime.RunRegistry,
	pending *runtime.PendingMessageQueue,
	locks *biz.SessionLockManager,
	sessions biz.SessionStatePort,
	activityBus biz.ActivityEventBus,
	lg loggateway.Logger,
) *biz.ChatUsecase {
	uc := biz.NewChatUsecase(
		NewRunGatewayAdapter(runs),
		locks,
		NewPendingQueueAdapter(pending),
		NewChatRunStatusPersister(sessions, lg),
		NewChatEventPublisher(activityBus),
		lg,
	)
	uc.StartBackgroundGoroutines()
	return uc
}

func persistRunStatusToSession(sessions biz.SessionStatePort, ctx context.Context, sessionID, runID, status, errMsg string) error {
	if isNilInterface(sessions) {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	bg, bgCancel := context.WithTimeout(ctx, 5*time.Second)
	defer bgCancel()

	now := time.Now().UTC().Format(time.RFC3339)
	status = strings.TrimSpace(status)
	errMsg = strings.TrimSpace(errMsg)

	sets := map[string]string{
		stateKeyRunStatus:    status,
		stateKeyRunUpdatedAt: now,
	}
	if errMsg != "" {
		sets[stateKeyRunError] = errMsg
	}

	if terminalRunStatus(status) {
		// Terminal status: clear run_id and await markers, but keep
		// status/error/updated_at for crash-recovery hydration.
		deletes := []string{
			stateKeyRunID,
			stateKeyAwaitRunID,
			stateKeyAwaitSince,
		}
		// If errMsg is empty, clear the error field too; otherwise the
		// previous error would persist after a successful terminal state.
		if errMsg == "" {
			deletes = append(deletes, stateKeyRunError)
		}
		return sessions.PatchSessionState(bg, sessionID, sets, deletes)
	}

	// Non-terminal status: persist run_id alongside status/updated_at.
	sets[stateKeyRunID] = strings.TrimSpace(runID)
	// Clear any stale error from a previous failed attempt when entering
	// a non-terminal state (e.g., retry after failure).
	if errMsg == "" {
		deletes := []string{stateKeyRunError}
		return sessions.PatchSessionState(bg, sessionID, sets, deletes)
	}
	return sessions.PatchSessionState(bg, sessionID, sets, nil)
}

func persistAwaitMarkersToSession(sessions biz.SessionStatePort, ctx context.Context, sessionID, runID string, await biz.ChatAwaitMeta, syncWrite bool, lg loggateway.Logger) {
	if isNilInterface(sessions) {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sets := map[string]string{
		stateKeyAwaitRunID: strings.TrimSpace(runID),
		stateKeyAwaitSince: now,
	}
	var deletes []string
	if k := strings.TrimSpace(await.Kind); k != "" {
		sets[stateKeyAwaitKind] = k
	} else {
		deletes = append(deletes, stateKeyAwaitKind)
	}
	if k := strings.TrimSpace(await.ToolKey); k != "" {
		sets[stateKeyAwaitToolKey] = k
	} else {
		deletes = append(deletes, stateKeyAwaitToolKey)
	}
	if k := strings.TrimSpace(await.ToolCallID); k != "" {
		sets[stateKeyAwaitToolCallID] = k
	} else {
		deletes = append(deletes, stateKeyAwaitToolCallID)
	}

	patch := func(bg context.Context) {
		if err := sessions.PatchSessionState(bg, sessionID, sets, deletes); err != nil {
			lg.Warn("PatchSessionState failed", loggateway.StepID("chat.persist_await_markers"), loggateway.Err(err), loggateway.Str("session_id", sessionID))
		}
	}
	if syncWrite {
		bg, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		patch(bg)
		return
	}
	safego.Go(ctx, "chat.persist_await_markers", func() {
		bg, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		patch(bg)
	})
}

func clearAwaitingRunStateFromSession(sessions biz.SessionStatePort, ctx context.Context, sessionID string, lg loggateway.Logger) {
	if isNilInterface(sessions) {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	safego.Go(ctx, "chat.clear_await_state", func() {
		bg, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		if err := sessions.PatchSessionState(bg, sessionID, nil, []string{
			stateKeyRunID, stateKeyRunStatus, stateKeyRunError, stateKeyRunUpdatedAt,
			stateKeyAwaitRunID, stateKeyAwaitSince, stateKeyAwaitKind,
			stateKeyAwaitToolKey, stateKeyAwaitToolCallID,
		}); err != nil {
			lg.Warn("PatchSessionState failed", loggateway.StepID("chat.clear_await_state"), loggateway.Err(err), loggateway.Str("session_id", sessionID))
		}
	})
}

// publishMessageQueuedToBus publishes a message_queued ActivityEvent
// (Kind=notice, Domain=chat). Replaces the legacy EnvelopeTypeRunStatus publish.
func publishMessageQueuedToBus(activityBus biz.ActivityEventBus, sessionID string) {
	if activityBus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	activityBus.Publish(context.Background(), biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindNotice,
			Status:    biz.ActivityStatusPending,
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
			AgentKey:  "chat-service",
			AgentName: "对话服务",
			Content:   "消息已加入队列",
			Meta: map[string]any{
				"status":      "queued",
				"hint":        "message_queued",
				"source":      "chat-service",
				"notice_type": "info",
			},
		},
		Domain: biz.ActivityDomainChat,
	})
}
