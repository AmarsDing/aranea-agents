package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
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
	Priority  int    `json:"priority,omitempty"` // 0=normal, 1=high priority
	// Kind 是注入级别（P2-3 三级注入语义）："" / "followup" = 追问（turn 结束后
	// 作为新 turn 输入）；"inject" = 静默上下文（不单独唤醒 turn，仅随下一条
	// followup 作为上下文前缀合入）。空值兼容旧快照。
	Kind string `json:"kind,omitempty"`
}

// PendingMessageQueue stores per-session follow-up message queues (Follow-up Queue / Cursor-style).
type PendingMessageQueue struct {
	mu       sync.Mutex
	queues   map[string][]PendingMessage
	dir      string
	stopCh   chan struct{}
	snapshot bool
	lg       loggateway.Logger
}

func NewPendingMessageQueue() *PendingMessageQueue {
	return NewPendingMessageQueueWithDir("")
}

func NewPendingMessageQueueWithDir(dir string) *PendingMessageQueue {
	return NewPendingMessageQueueWithDirAndLogger(dir, nil)
}

func NewPendingMessageQueueWithDirAndLogger(dir string, lg loggateway.Logger) *PendingMessageQueue {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	q := &PendingMessageQueue{
		queues:   make(map[string][]PendingMessage),
		dir:      dir,
		stopCh:   make(chan struct{}),
		snapshot: dir != "",
		lg:       lg,
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
	queue := q.queues[sessionID]
	if len(queue) >= MaxPendingPerSession {
		q.mu.Unlock()
		return ""
	}
	q.queues[sessionID] = append(queue, entry)
	q.mu.Unlock()
	q.writeThrough()
	return id
}

// EnqueueInject appends a silent context entry (P2-3 inject level): it never
// wakes a turn by itself and is only consumed as context merged into the next
// followup dispatch. Otherwise identical to Enqueue (same FIFO position,
// capacity cap, and snapshot persistence).
func (q *PendingMessageQueue) EnqueueInject(sessionID, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	id := uuid.NewString()
	entry := PendingMessage{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
		Kind:      "inject",
	}
	q.mu.Lock()
	queue := q.queues[sessionID]
	if len(queue) >= MaxPendingPerSession {
		q.mu.Unlock()
		return ""
	}
	q.queues[sessionID] = append(queue, entry)
	q.mu.Unlock()
	q.writeThrough()
	return id
}

// EnqueueFollowup merges content into the last pending entry when one exists (CH-BOR-01).
func (q *PendingMessageQueue) EnqueueFollowup(sessionID, content, separator string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if separator == "" {
		separator = "\n"
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	if len(queue) > 0 {
		last := len(queue) - 1
		merged := strings.TrimSpace(queue[last].Content)
		if merged != "" {
			merged += separator
		}
		merged += content
		// 合并后的条目回到 followup 级（Kind 置空）：用户追问文本合入 inject
		// 条目时不得继承其静默语义，否则用户消息会被意外吞掉。
		queue[last] = PendingMessage{
			ID:        queue[last].ID,
			Content:   merged,
			Status:    queue[last].Status,
			CreatedAt: queue[last].CreatedAt,
			Priority:  queue[last].Priority,
		}
		q.queues[sessionID] = append([]PendingMessage(nil), queue...)
		return queue[last].ID
	}
	if len(queue) >= MaxPendingPerSession {
		return ""
	}
	id := uuid.NewString()
	q.queues[sessionID] = append(queue, PendingMessage{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	return id
}

// Peek returns the head of the session queue without removing it.
//
// Used by callers that need to inspect the next pending message before
// deciding whether to dequeue (e.g., processPendingQueue's atomic
// check-and-dequeue under the session lock, which eliminates the TOCTOU
// window between Dequeue and the HasActive admission check).
func (q *PendingMessageQueue) Peek(sessionID string) (PendingMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	if len(queue) == 0 {
		return PendingMessage{}, false
	}
	return queue[0], true
}

func (q *PendingMessageQueue) Dequeue(sessionID string) (PendingMessage, bool) {
	q.mu.Lock()
	queue := q.queues[sessionID]
	if len(queue) == 0 {
		delete(q.queues, sessionID)
		q.mu.Unlock()
		return PendingMessage{}, false
	}
	head := queue[0]
	if len(queue) == 1 {
		delete(q.queues, sessionID)
	} else {
		q.queues[sessionID] = append([]PendingMessage(nil), queue[1:]...)
	}
	q.mu.Unlock()
	q.writeThrough()
	return head, true
}

func (q *PendingMessageQueue) Remove(sessionID, entryID string) bool {
	q.mu.Lock()
	queue := q.queues[sessionID]
	for i, e := range queue {
		if e.ID == entryID {
			queue = append(queue[:i], queue[i+1:]...)
			if len(queue) == 0 {
				delete(q.queues, sessionID)
			} else {
				q.queues[sessionID] = append([]PendingMessage(nil), queue...)
			}
			q.mu.Unlock()
			q.writeThrough()
			return true
		}
	}
	q.mu.Unlock()
	return false
}

// writeThrough synchronously persists the current snapshot (C-12).
// Complements the periodic 10s snapshot so Enqueue/Dequeue/Remove survive crashes.
func (q *PendingMessageQueue) writeThrough() {
	if q == nil || !q.snapshot {
		return
	}
	q.saveSnapshot()
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
				Priority:  e.Priority,
				Kind:      e.Kind,
			}
			q.queues[sessionID] = append([]PendingMessage(nil), queue...)
			return true
		}
	}
	return false
}

// PromoteToFront moves the specified message to the front of the session queue.
func (q *PendingMessageQueue) PromoteToFront(sessionID, pendingID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	for i, e := range queue {
		if e.ID == pendingID {
			if i == 0 {
				return nil
			}
			entry := queue[i]
			queue = append(queue[:i], queue[i+1:]...)
			queue = append([]PendingMessage{entry}, queue...)
			q.queues[sessionID] = queue
			return nil
		}
	}
	return fmt.Errorf("pending message %s not found in session %s", pendingID, sessionID)
}

// SetPriority sets the priority of the specified message (0=normal, 1=high).
func (q *PendingMessageQueue) SetPriority(sessionID, pendingID string, priority int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	queue := q.queues[sessionID]
	for i, e := range queue {
		if e.ID == pendingID {
			queue[i] = PendingMessage{
				ID:        e.ID,
				Content:   e.Content,
				Status:    e.Status,
				CreatedAt: e.CreatedAt,
				Priority:  priority,
				Kind:      e.Kind,
			}
			q.queues[sessionID] = append([]PendingMessage(nil), queue...)
			return nil
		}
	}
	return fmt.Errorf("pending message %s not found in session %s", pendingID, sessionID)
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
		q.lg.Warn("pending queue snapshot marshal failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
		return
	}
	tmp := filepath.Join(q.dir, pendingSnapshotFile+".tmp")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		q.lg.Warn("pending queue snapshot write failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
		return
	}
	dst := filepath.Join(q.dir, pendingSnapshotFile)
	if err := os.Rename(tmp, dst); err != nil {
		q.lg.Warn("pending queue snapshot rename failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
		_ = os.Remove(tmp)
	}
}

func (q *PendingMessageQueue) restore() {
	if q.dir == "" {
		return
	}
	path := filepath.Join(q.dir, pendingSnapshotFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			q.lg.Warn("pending queue snapshot read failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
		}
		return
	}
	var snapshot map[string][]PendingMessage
	if err := json.Unmarshal(data, &snapshot); err != nil {
		q.lg.Warn("pending queue snapshot unmarshal failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
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
