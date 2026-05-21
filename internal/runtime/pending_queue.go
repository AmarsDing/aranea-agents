package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/safego"
)

const MaxPendingPerSession = 32

const (
	pendingSnapshotPeriod = 10 * time.Second
	pendingSnapshotFile   = "pending_queue.json"
)

// PendingMessage is one FIFO follow-up entry waiting for the current turn to finish.
type PendingMessage struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// PendingMessageQueue stores per-session follow-up message queues (Follow-up Queue / Cursor-style).
type PendingMessageQueue struct {
	mu       sync.Mutex
	queues   map[string][]PendingMessage
	dir      string
	stopCh   chan struct{}
	snapshot bool
}

func NewPendingMessageQueue() *PendingMessageQueue {
	return NewPendingMessageQueueWithDir("")
}

func NewPendingMessageQueueWithDir(dir string) *PendingMessageQueue {
	q := &PendingMessageQueue{
		queues:   make(map[string][]PendingMessage),
		dir:      dir,
		stopCh:   make(chan struct{}),
		snapshot: dir != "",
	}
	if q.snapshot {
		q.restore()
		safego.Go(nil, "pending-snapshot", q.snapshotLoop)
	}
	return q
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
	if len(queue) >= MaxPendingPerSession {
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

func (q *PendingMessageQueue) Close() {
	if q.snapshot {
		close(q.stopCh)
		q.saveSnapshot()
	}
}

func (q *PendingMessageQueue) snapshotLoop() {
	ticker := time.NewTicker(pendingSnapshotPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			q.saveSnapshot()
		case <-q.stopCh:
			return
		}
	}
}

func (q *PendingMessageQueue) saveSnapshot() {
	if q.dir == "" {
		return
	}
	q.mu.Lock()
	snapshot := make(map[string][]PendingMessage, len(q.queues))
	for k, v := range q.queues {
		snapshot[k] = append([]PendingMessage(nil), v...)
	}
	q.mu.Unlock()

	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	tmp := filepath.Join(q.dir, pendingSnapshotFile+".tmp")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(q.dir, pendingSnapshotFile))
}

func (q *PendingMessageQueue) restore() {
	if q.dir == "" {
		return
	}
	path := filepath.Join(q.dir, pendingSnapshotFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var snapshot map[string][]PendingMessage
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return
	}
	cutoff := time.Now().Add(-2 * time.Hour)
	q.mu.Lock()
	defer q.mu.Unlock()
	for sid, entries := range snapshot {
		var kept []PendingMessage
		for _, e := range entries {
			t, err := time.Parse(time.RFC3339, e.CreatedAt)
			if err == nil && t.After(cutoff) {
				kept = append(kept, e)
			}
		}
		if len(kept) > 0 {
			q.queues[sid] = kept
		}
	}
}
