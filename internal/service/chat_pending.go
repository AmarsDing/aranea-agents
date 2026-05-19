package service

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

const maxPendingPerSession = 32

// PendingMessage is a queued user message while a session run is active.
type PendingMessage struct {
	ID        string
	Content   string
	Status    string
	CreatedAt string
}

// PendingMessageQueue stores per-session FIFO queues of user messages (EP-RT / chat enqueue).
type PendingMessageQueue struct {
	mu     sync.Mutex
	queues map[string][]PendingMessage
}

func NewPendingMessageQueue() *PendingMessageQueue {
	return &PendingMessageQueue{queues: make(map[string][]PendingMessage)}
}

func (q *PendingMessageQueue) List(sessionID string) []PendingMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	out := make([]PendingMessage, len(queue))
	copy(out, queue)
	return out
}

func (q *PendingMessageQueue) Enqueue(sessionID, content string) string {
	id := uuid.NewString()
	entry := PendingMessage{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	if len(queue) >= maxPendingPerSession {
		return ""
	}
	q.queues[sessionID] = append(queue, entry)
	return id
}

func (q *PendingMessageQueue) Dequeue(sessionID string) (PendingMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	if len(queue) == 0 {
		delete(q.queues, sessionID)
		return PendingMessage{}, false
	}
	head := queue[0]
	if len(queue) == 1 {
		delete(q.queues, sessionID)
		return head, true
	}
	q.queues[sessionID] = append([]PendingMessage(nil), queue[1:]...)
	return head, true
}

func (q *PendingMessageQueue) Remove(sessionID, entryID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	for i, e := range queue {
		if e.ID == entryID {
			queue = append(queue[:i], queue[i+1:]...)
			if len(queue) == 0 {
				delete(q.queues, sessionID)
			} else {
				q.queues[sessionID] = append([]PendingMessage(nil), queue...)
			}
			return true
		}
	}
	return false
}

func (q *PendingMessageQueue) Update(sessionID, entryID, newContent string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	for i, e := range queue {
		if e.ID == entryID {
			queue[i] = PendingMessage{
				ID:        e.ID,
				Content:   newContent,
				Status:    e.Status,
				CreatedAt: e.CreatedAt,
			}
			q.queues[sessionID] = append([]PendingMessage(nil), queue...)
			return true
		}
	}
	return false
}
