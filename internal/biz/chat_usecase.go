package biz

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// awaitChanMaxAge is the maximum lifetime of an await channel entry before GC
// reclaims it. Reduced from 30m to 10m as a safety net: the primary cleanup
// mechanism is event-driven (SetRunStatus deletes on terminal status), and the
// GC ticker serves as a fallback for edge cases.
const awaitChanMaxAge = 10 * time.Minute

// awaitChanGCInterval is the interval at which the background GC ticker scans
// for stale await channel entries.
const awaitChanGCInterval = 5 * time.Minute

// AwaitReplyMsg is the message sent over an await channel when a user replies.
type AwaitReplyMsg struct {
	RunID string
	Reply string
}

// AwaitChannel is the concrete channel type used for await-reply coordination.
type AwaitChannel = chan AwaitReplyMsg

type awaitChanEntry struct {
	ch        AwaitChannel
	done      chan struct{}
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
	PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error
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
	mu         sync.RWMutex
	awaitChans map[string]awaitChanEntry
	bgCancel   context.CancelFunc
	lg         loggateway.Logger
}

func NewChatUsecase(
	runs ChatRunGateway,
	locker ChatSessionLocker,
	pending ChatPendingQueue,
	persist ChatRunStatusPersister,
	publisher ChatEventPublisher,
	lg loggateway.Logger,
) *ChatUsecase {
	return &ChatUsecase{
		runs:       runs,
		locker:     locker,
		pending:    pending,
		persist:    persist,
		publisher:  publisher,
		awaitChans: make(map[string]awaitChanEntry),
		lg:         lg,
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
	if err := uc.persist.PersistRunStatus(ctx, sessionID, runID, status, errMsg); err != nil {
		uc.lg.Error("persist run status failed", loggateway.StepID("chat.persist_run_status"), loggateway.SessionID(sessionID), loggateway.Str("run_id", runID), loggateway.Err(err))
	}
	uc.runs.SetStatus(sessionID, runID, status, errMsg)
	uc.publisher.PublishRunStatus(sessionID, runID, status, errMsg)
	// Proactively clean up await channel when run reaches a terminal status.
	// This prevents memory leaks when runs end without going through the normal
	// await reply flow (e.g., hard budget, cancellation, unexpected failure).
	switch strings.ToLower(strings.TrimSpace(status)) {
	case SessionRunPhaseCompleted, SessionRunPhaseFailed, SessionRunPhaseCancelled:
		uc.DeleteAwaitChannel(sessionID)
	}
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

func (uc *ChatUsecase) RegisterAwaitChannel(sessionID string, ch AwaitChannel) {
	uc.mu.Lock()
	if old, ok := uc.awaitChans[sessionID]; ok {
		close(old.done)
	}
	uc.awaitChans[sessionID] = awaitChanEntry{ch: ch, done: make(chan struct{}), createdAt: time.Now()}
	uc.mu.Unlock()
}

func (uc *ChatUsecase) DeleteAwaitChannel(sessionID string) {
	uc.mu.Lock()
	if entry, ok := uc.awaitChans[sessionID]; ok {
		close(entry.done)
		delete(uc.awaitChans, sessionID)
	}
	uc.mu.Unlock()
}

func (uc *ChatUsecase) LoadAwaitChannel(sessionID string) (AwaitChannel, bool) {
	uc.mu.RLock()
	entry, ok := uc.awaitChans[sessionID]
	uc.mu.RUnlock()
	if !ok {
		return nil, false
	}
	// Entry has been logically deleted (done closed) but not yet removed from map.
	select {
	case <-entry.done:
		return nil, false
	default:
		return entry.ch, true
	}
}

// TrySendAwaitChannel attempts to send msg to the await channel for sessionID.
// It holds a read lock while checking the entry and sending, which prevents
// the GC goroutine from closing the done channel concurrently.
func (uc *ChatUsecase) TrySendAwaitChannel(sessionID string, msg AwaitReplyMsg) bool {
	uc.mu.RLock()
	entry, ok := uc.awaitChans[sessionID]
	if !ok {
		uc.mu.RUnlock()
		return false
	}
	// If done is already closed, the entry has been logically deleted.
	select {
	case <-entry.done:
		uc.mu.RUnlock()
		return false
	default:
	}
	select {
	case entry.ch <- msg:
		uc.mu.RUnlock()
		return true
	default:
		uc.mu.RUnlock()
		return false
	}
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
			ticker := time.NewTicker(awaitChanGCInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					now := time.Now()
					uc.mu.Lock()
					for sid, entry := range uc.awaitChans {
						if strings.TrimSpace(sid) == "" {
							close(entry.done)
							delete(uc.awaitChans, sid)
							continue
						}
						if now.Sub(entry.createdAt) > awaitChanMaxAge {
							uc.lg.Warn("await channel expired, cleaning up", loggateway.StepID("session.compress"), loggateway.SessionID(sid), loggateway.Str("age", now.Sub(entry.createdAt).Round(time.Second).String()))
							close(entry.done)
							delete(uc.awaitChans, sid)
						}
					}
					uc.mu.Unlock()
				}
			}
		})
}
