package trpcmem

import (
	"strings"
	"sync"
	"time"
)

type AutoMemoryJobRequest struct {
	AppName    string
	SessionID  string
	UserID     string
	EnqueuedAt time.Time
	// Feedback-triggered preference extraction (optional).
	FeedbackMessageID string
	FeedbackRating    string
	FeedbackComment   string
}

// AutoMemoryQueue abstracts the job queue consumed by AutoMemoryWorker and sqlite memory service.
type AutoMemoryQueue interface {
	Enqueue(r AutoMemoryJobRequest)
	Chan() <-chan AutoMemoryJobRequest
}

type MemoryJobQueue struct {
	ch       chan AutoMemoryJobRequest
	recent   sync.Map
	debounce time.Duration
}

var _ AutoMemoryQueue = (*MemoryJobQueue)(nil)

func NewMemoryJobQueue(size int, debounce time.Duration) *MemoryJobQueue {
	if size <= 0 {
		size = 256
	}
	if debounce <= 0 {
		debounce = 30 * time.Second
	}
	return &MemoryJobQueue{
		ch:       make(chan AutoMemoryJobRequest, size),
		debounce: debounce,
	}
}

func (q *MemoryJobQueue) Enqueue(r AutoMemoryJobRequest) {
	if q == nil {
		return
	}
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	if sid := strings.TrimSpace(r.SessionID); sid != "" {
		if t, ok := q.recent.Load(sid); ok {
			if time.Since(t.(time.Time)) < q.debounce {
				return
			}
		}
		q.recent.Store(sid, time.Now())
	}
	select {
	case q.ch <- r:
	default:
	}
}

func (q *MemoryJobQueue) Chan() <-chan AutoMemoryJobRequest {
	if q == nil {
		return nil
	}
	return q.ch
}

// NewAutoMemoryEnqueuer adapts a wired queue to biz.AutoMemoryEnqueuer.
func NewAutoMemoryEnqueuer(q AutoMemoryQueue) func(appName, sessionID string, enqueuedAt time.Time) {
	return func(appName, sessionID string, enqueuedAt time.Time) {
		if q == nil {
			return
		}
		q.Enqueue(AutoMemoryJobRequest{
			AppName:    appName,
			SessionID:  sessionID,
			EnqueuedAt: enqueuedAt,
		})
	}
}

// NewFeedbackMemoryEnqueuer adapts a wired queue to biz.FeedbackMemoryEnqueuer.
func NewFeedbackMemoryEnqueuer(q AutoMemoryQueue) func(sessionID, messageID, rating, comment string, enqueuedAt time.Time) {
	return func(sessionID, messageID, rating, comment string, enqueuedAt time.Time) {
		if q == nil {
			return
		}
		q.Enqueue(AutoMemoryJobRequest{
			SessionID:         sessionID,
			EnqueuedAt:        enqueuedAt,
			FeedbackMessageID: messageID,
			FeedbackRating:    rating,
			FeedbackComment:   comment,
		})
	}
}
