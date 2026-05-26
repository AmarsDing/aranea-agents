package biz

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

const awaitChanMaxAge = 30 * time.Minute

type awaitChanEntry struct {
	ch        interface{}
	createdAt time.Time
}

type ChatRunStatus struct {
	RunID     string
	Status    string
	ErrMsg    string
	UpdatedAt time.Time
}

type ChatRunGateway interface {
	HasActive(sessionID string) bool
	Cancel(sessionID string) (stopped bool, runID string)
	EnqueueUserMessage(sessionID, content string) (bool, error)
	SetStatus(sessionID, runID, status, errMsg string)
	GetStatus(sessionID string) (ChatRunStatus, bool)
}

type ChatSessionLocker interface {
	Lock(sessionID string) func()
}

type ChatPendingQueue interface {
	List(sessionID string) []PendingQueueEntry
	Enqueue(sessionID, content string) string
	EnqueueFollowup(sessionID, content string) string
	Dequeue(sessionID string) (PendingQueueEntry, bool)
	Remove(sessionID, entryID string) bool
	Update(sessionID, entryID, newContent string) bool
	Close()
}

type PendingQueueEntry struct {
	ID        string
	Content   string
	Status    string
	CreatedAt string
}

type ChatAwaitMeta struct {
	Kind       string
	ToolKey    string
	ToolCallID string
}

const (
	ChatAwaitKindReply       = "reply"
	ChatAwaitKindToolConfirm = "tool_confirm"
)

const (
	ChatEnqueueRejectNone        = ""
	ChatEnqueueRejectNoActiveRun = "no_active_run"
	ChatEnqueueRejectQueueFull   = "queue_full"
)

type ChatRunStatusPersister interface {
	PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string)
	PersistAwaitMarkers(ctx context.Context, sessionID, runID string, meta ChatAwaitMeta)
	ClearAwaitingRunState(ctx context.Context, sessionID string)
}

type ChatEventPublisher interface {
	PublishRunStatus(sessionID, runID, status, errMsg string)
	PublishMessageQueued(sessionID string)
}

type ChatUsecase struct {
	runs       ChatRunGateway
	locker     ChatSessionLocker
	pending    ChatPendingQueue
	persist    ChatRunStatusPersister
	publisher  ChatEventPublisher
	awaitChans sync.Map
	bgCancel   context.CancelFunc
}

func NewChatUsecase(
	runs ChatRunGateway,
	locker ChatSessionLocker,
	pending ChatPendingQueue,
	persist ChatRunStatusPersister,
	publisher ChatEventPublisher,
) *ChatUsecase {
	return &ChatUsecase{
		runs:      runs,
		locker:    locker,
		pending:   pending,
		persist:   persist,
		publisher: publisher,
	}
}

func (uc *ChatUsecase) LockSession(sessionID string) func() {
	return uc.locker.Lock(sessionID)
}

func (uc *ChatUsecase) HasActiveRun(sessionID string) bool {
	return uc.runs.HasActive(sessionID)
}

func (uc *ChatUsecase) CancelRun(sessionID string) (bool, string) {
	return uc.runs.Cancel(sessionID)
}

func (uc *ChatUsecase) SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	uc.runs.SetStatus(sessionID, runID, status, errMsg)
	uc.publisher.PublishRunStatus(sessionID, runID, status, errMsg)
	uc.persist.PersistRunStatus(ctx, sessionID, runID, status, errMsg)
}

func (uc *ChatUsecase) GetRunStatus(sessionID string) (ChatRunStatus, bool) {
	return uc.runs.GetStatus(sessionID)
}

func (uc *ChatUsecase) GetPendingMessages(sessionID string) []PendingQueueEntry {
	return uc.pending.List(sessionID)
}

func (uc *ChatUsecase) EnqueuePendingMessage(sessionID, content string) string {
	return uc.pending.Enqueue(sessionID, content)
}

func (uc *ChatUsecase) CancelPendingMessage(sessionID, pendingID string) bool {
	return uc.pending.Remove(sessionID, pendingID)
}

func (uc *ChatUsecase) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return uc.pending.Update(sessionID, pendingID, content)
}

func (uc *ChatUsecase) DequeuePendingMessage(sessionID string) (PendingQueueEntry, bool) {
	return uc.pending.Dequeue(sessionID)
}

func (uc *ChatUsecase) EnqueueUserMessage(sessionID, content string, mergeFollowup bool) (accepted, queued bool, pendingID, rejectReason string, err error) {
	unlock := uc.locker.Lock(sessionID)
	defer unlock()

	if !uc.runs.HasActive(sessionID) {
		return false, false, "", ChatEnqueueRejectNoActiveRun, nil
	}

	enqueued, enqueueErr := uc.runs.EnqueueUserMessage(sessionID, content)
	if enqueueErr != nil {
		return false, false, "", "", enqueueErr
	}
	if enqueued {
		uc.publisher.PublishMessageQueued(sessionID)
		return true, false, "", "", nil
	}

	var pid string
	if mergeFollowup {
		pid = uc.pending.EnqueueFollowup(sessionID, content)
	} else {
		pid = uc.pending.Enqueue(sessionID, content)
	}
	if pid == "" {
		return false, false, "", ChatEnqueueRejectQueueFull, nil
	}
	uc.publisher.PublishMessageQueued(sessionID)
	return true, true, pid, "", nil
}

func (uc *ChatUsecase) RegisterAwaitChannel(sessionID string, ch interface{}) {
	uc.awaitChans.Store(sessionID, awaitChanEntry{ch: ch, createdAt: time.Now()})
}

func (uc *ChatUsecase) DeleteAwaitChannel(sessionID string) {
	uc.awaitChans.Delete(sessionID)
}

func (uc *ChatUsecase) LoadAwaitChannel(sessionID string) (interface{}, bool) {
	val, ok := uc.awaitChans.Load(sessionID)
	if !ok {
		return nil, false
	}
	entry, ok := val.(awaitChanEntry)
	if !ok {
		return val, true
	}
	return entry.ch, true
}

func (uc *ChatUsecase) PersistAwaitMarkers(ctx context.Context, sessionID, runID string, meta ChatAwaitMeta) {
	uc.persist.PersistAwaitMarkers(ctx, sessionID, runID, meta)
}

func (uc *ChatUsecase) ClearAwaitingRunState(ctx context.Context, sessionID string) {
	uc.persist.ClearAwaitingRunState(ctx, sessionID)
}

func (uc *ChatUsecase) Close() {
	if uc.bgCancel != nil {
		uc.bgCancel()
	}
	uc.pending.Close()
}

func (uc *ChatUsecase) StartBackgroundGoroutines() {
	ctx, cancel := context.WithCancel(context.Background())
	uc.bgCancel = cancel
	safego.Go(ctx, "chat-usecase-gc", func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				uc.awaitChans.Range(func(key, val interface{}) bool {
					sid, ok := key.(string)
					if !ok || strings.TrimSpace(sid) == "" {
						uc.awaitChans.Delete(key)
						return true
					}
					entry, ok := val.(awaitChanEntry)
					if ok && now.Sub(entry.createdAt) > awaitChanMaxAge {
						slog.Warn("await channel expired, cleaning up", "session_id", sid, "age", now.Sub(entry.createdAt).Round(time.Second))
						uc.awaitChans.Delete(key)
					}
					return true
				})
			}
		}
	})
}
