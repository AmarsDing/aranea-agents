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
}

type MemoryJobQueue struct {
	ch      chan AutoMemoryJobRequest
	recent  sync.Map
	debounce time.Duration
}

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

func (q *MemoryJobQueue) Chan() <-chan AutoMemoryJobRequest { return q.ch }

var globalAutoMemoryQueue = NewMemoryJobQueue(256, 30*time.Second)

func EnqueueAutoMemory(r AutoMemoryJobRequest) {
	globalAutoMemoryQueue.Enqueue(r)
}

func GlobalAutoMemoryQueue() *MemoryJobQueue { return globalAutoMemoryQueue }

// SetGlobalAutoMemoryQueueForTest swaps the process-wide queue (tests only).
func SetGlobalAutoMemoryQueueForTest(q *MemoryJobQueue) *MemoryJobQueue {
	prev := globalAutoMemoryQueue
	globalAutoMemoryQueue = q
	return prev
}
