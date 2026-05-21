package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/runtime"
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
}

func (p *chatRunStatusPersister) PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	persistRunStatusToSession(p.sessions, ctx, sessionID, runID, status, errMsg)
}

func (p *chatRunStatusPersister) PersistAwaitMarkers(ctx context.Context, sessionID, runID string, meta biz.ChatAwaitMeta) {
	persistAwaitMarkersToSession(p.sessions, ctx, sessionID, runID, meta, false)
}

func (p *chatRunStatusPersister) ClearAwaitingRunState(ctx context.Context, sessionID string) {
	clearAwaitingRunStateFromSession(p.sessions, ctx, sessionID)
}

func NewChatRunStatusPersister(sessions *biz.SessionUsecase) biz.ChatRunStatusPersister {
	return &chatRunStatusPersister{sessions: sessions}
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
) *biz.ChatUsecase {
	uc := biz.NewChatUsecase(
		NewRunGatewayAdapter(runs),
		NewSessionLockerAdapter(locks),
		NewPendingQueueAdapter(pending),
		NewChatRunStatusPersister(sessions),
		NewChatEventPublisher(bus),
	)
	uc.StartBackgroundGoroutines()
	return uc
}

func persistRunStatusToSession(sessions *biz.SessionUsecase, ctx context.Context, sessionID, runID, status, errMsg string) {
	if sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	safego.Go(ctx, "chat.persist_run_status", func() {
		bg, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		state, err := sessions.GetSessionState(bg, sessionID)
		if err != nil {
			return
		}
		if state == nil {
			state = map[string]string{}
		}
		if terminalRunStatus(status) {
			state[stateKeyRunStatus] = strings.TrimSpace(status)
			state[stateKeyRunError] = strings.TrimSpace(errMsg)
			state[stateKeyRunUpdatedAt] = now
			delete(state, stateKeyRunID)
			delete(state, stateKeyAwaitRunID)
			delete(state, stateKeyAwaitSince)
		} else {
			state[stateKeyRunID] = strings.TrimSpace(runID)
			state[stateKeyRunStatus] = strings.TrimSpace(status)
			state[stateKeyRunError] = strings.TrimSpace(errMsg)
			state[stateKeyRunUpdatedAt] = now
		}
		_ = sessions.SaveSessionState(bg, sessionID, state)
	})
}

func persistAwaitMarkersToSession(sessions *biz.SessionUsecase, ctx context.Context, sessionID, runID string, await biz.ChatAwaitMeta, syncWrite bool) {
	if sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	write := func(bg context.Context) {
		state, err := sessions.GetSessionState(bg, sessionID)
		if err != nil {
			return
		}
		if state == nil {
			state = map[string]string{}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		state[stateKeyAwaitRunID] = strings.TrimSpace(runID)
		state[stateKeyAwaitSince] = now
		if k := strings.TrimSpace(await.Kind); k != "" {
			state[stateKeyAwaitKind] = k
		} else {
			delete(state, stateKeyAwaitKind)
		}
		if k := strings.TrimSpace(await.ToolKey); k != "" {
			state[stateKeyAwaitToolKey] = k
		} else {
			delete(state, stateKeyAwaitToolKey)
		}
		if k := strings.TrimSpace(await.ToolCallID); k != "" {
			state[stateKeyAwaitToolCallID] = k
		} else {
			delete(state, stateKeyAwaitToolCallID)
		}
		_ = sessions.SaveSessionState(bg, sessionID, state)
	}
	if syncWrite {
		bg, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		write(bg)
		return
	}
	safego.Go(ctx, "chat.persist_await_markers", func() {
		bg, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		write(bg)
	})
}

func clearAwaitingRunStateFromSession(sessions *biz.SessionUsecase, ctx context.Context, sessionID string) {
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
		state, err := sessions.GetSessionState(bg, sessionID)
		if err != nil || len(state) == 0 {
			return
		}
		delete(state, stateKeyRunID)
		delete(state, stateKeyRunStatus)
		delete(state, stateKeyRunError)
		delete(state, stateKeyRunUpdatedAt)
		delete(state, stateKeyAwaitRunID)
		delete(state, stateKeyAwaitSince)
		delete(state, stateKeyAwaitKind)
		delete(state, stateKeyAwaitToolKey)
		delete(state, stateKeyAwaitToolCallID)
		_ = sessions.SaveSessionState(bg, sessionID, state)
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
