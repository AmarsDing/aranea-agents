package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

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

type sessionLockerAdapter struct {
	*sessionLockManager
}

func NewSessionLockerAdapter(m *sessionLockManager) biz.ChatSessionLocker {
	return &sessionLockerAdapter{sessionLockManager: m}
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

func (a *pendingQueueAdapter) Close() {
	a.PendingMessageQueue.Close()
}

func NewPendingQueueAdapter(q *runtime.PendingMessageQueue) biz.ChatPendingQueue {
	return &pendingQueueAdapter{PendingMessageQueue: q}
}

type chatRunStatusPersister struct {
	sessions *biz.SessionUsecase
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

func NewChatRunStatusPersister(sessions *biz.SessionUsecase, lg loggateway.Logger) biz.ChatRunStatusPersister {
	return &chatRunStatusPersister{sessions: sessions, lg: lg}
}

type chatEventPublisher struct {
	bus event.Bus
}

func (pub *chatEventPublisher) PublishRunStatus(sessionID, runID, status, errMsg string) {
	PublishRunStatus(pub.bus, sessionID, runID, status, errMsg)
}

func (pub *chatEventPublisher) PublishMessageQueued(sessionID string) {
	publishMessageQueuedToBus(pub.bus, sessionID)
}

func NewChatEventPublisher(bus event.Bus) biz.ChatEventPublisher {
	return &chatEventPublisher{bus: bus}
}

// NewChatUsecaseFromDeps wires the shared run registry, pending queue, and session
// lock into a single Biz orchestration entrypoint for ChatService.
func NewChatUsecaseFromDeps(
	runs *runtime.RunRegistry,
	pending *runtime.PendingMessageQueue,
	locks *sessionLockManager,
	sessions *biz.SessionUsecase,
	bus event.Bus,
	lg loggateway.Logger,
) *biz.ChatUsecase {
	uc := biz.NewChatUsecase(
		NewRunGatewayAdapter(runs),
		NewSessionLockerAdapter(locks),
		NewPendingQueueAdapter(pending),
		NewChatRunStatusPersister(sessions, lg),
		NewChatEventPublisher(bus),
		lg,
	)
	uc.StartBackgroundGoroutines()
	return uc
}

func persistRunStatusToSession(sessions *biz.SessionUsecase, ctx context.Context, sessionID, runID, status, errMsg string) error {
	if sessions == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	bg, bgCancel := context.WithTimeout(ctx, 5*time.Second)
	defer bgCancel()

	if terminalRunStatus(status) {
		return sessions.PatchSessionState(bg, sessionID,
			map[string]string{},
			[]string{stateKeyRunID, stateKeyAwaitRunID, stateKeyAwaitSince},
		)
	}
	return sessions.PatchSessionState(bg, sessionID,
		map[string]string{
			stateKeyRunID: strings.TrimSpace(runID),
		},
		nil,
	)
}

func persistAwaitMarkersToSession(sessions *biz.SessionUsecase, ctx context.Context, sessionID, runID string, await biz.ChatAwaitMeta, syncWrite bool, lg loggateway.Logger) {
	if sessions == nil {
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

func clearAwaitingRunStateFromSession(sessions *biz.SessionUsecase, ctx context.Context, sessionID string, lg loggateway.Logger) {
	if sessions == nil {
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

func publishMessageQueuedToBus(bus event.Bus, sessionID string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "chat-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"status": "queued",
		"hint":   "message_queued",
	}
	bus.Publish(context.Background(), env)
}
