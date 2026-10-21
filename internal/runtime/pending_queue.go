package runtime

import (
	"context"
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
	// pendingPersistFlushInterval coalesces hot-path mutations into one durable
	// write (M6, 2026-10-18): each Enqueue/Dequeue/Update used to run a
	// synchronous full ReplaceAll (5s timeout) on the caller's hot path. Memory
	// state is authoritative immediately; the store/snapshot flush happens at
	// most once per interval, plus a final synchronous flush on Close.
	pendingPersistFlushInterval = 100 * time.Millisecond
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

// PendingQueueStore persists the in-memory follow-up queues (Postgres).
type PendingQueueStore interface {
	LoadAll(ctx context.Context) (map[string][]PendingMessage, error)
	ReplaceAll(ctx context.Context, queues map[string][]PendingMessage) error
}

// PendingMessageQueue stores per-session follow-up message queues (Follow-up Queue / Cursor-style).
type PendingMessageQueue struct {
	mu     sync.Mutex
	queues map[string][]PendingMessage
	// restored 记录「启动时从持久层（PG store / 文件快照）恢复且尚未完全排空」
	// 的会话集合（P1 FR-7，89-session-concurrency-hardening）。编排器据此在
	// 会话空闲时把新消息强制入队再整体 FIFO 排空，防止直行 turn 插队到恢复
	// 行之前（重启后 FIFO 反转）。标记在会话队列清空时被移除。
	restored map[string]struct{}
	dir      string
	stopCh   chan struct{}
	snapshot bool
	store    PendingQueueStore
	lg       loggateway.Logger
	// closeOnce makes Close idempotent (M4, 2026-10-18): a second Close used to
	// panic with "close of closed channel" on stopCh.
	closeOnce sync.Once
	// persistMu guards persistTimer (M6 batched flush); flushMu serializes
	// flushPersist invocations so a delayed flush can never overwrite a newer
	// snapshot with an older one (snapshots are taken inside the flush).
	persistMu    sync.Mutex
	persistTimer *time.Timer
	flushMu      sync.Mutex
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
		restored: make(map[string]struct{}),
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

// IsRestored 报告会话队列是否为启动时从持久层恢复且尚未排空的集合（P1
// FR-7）。仅服务于编排器的惰性排空判定；普通入队/出队路径不读此标记。
func (q *PendingMessageQueue) IsRestored(sessionID string) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.restored[sessionID]
	return ok
}

// FlushSession 级联清空会话的内存队列与恢复标记（P1 FR-7 删除/归档会话的
// 级联入口）。调度一次批量持久冲刷：删除会话后若内存仍持有条目，下一次
// ReplaceAll 会把它们重新插回 pending_queue_entries（FK 违例/幽灵行复活）。
// 调用方不得持有会话锁之外的 q.mu——本方法不持锁调用外部代码。
func (q *PendingMessageQueue) FlushSession(sessionID string) {
	if q == nil || sessionID == "" {
		return
	}
	q.mu.Lock()
	_, had := q.queues[sessionID]
	delete(q.queues, sessionID)
	delete(q.restored, sessionID)
	q.mu.Unlock()
	if had {
		q.schedulePersist()
	}
}

// markRestoredLocked 记录恢复会话；调用方须持 q.mu。
func (q *PendingMessageQueue) markRestoredLocked(sessionID string) {
	q.restored[sessionID] = struct{}{}
}

// clearRestoredLocked 在会话队列清空时移除恢复标记；调用方须持 q.mu。
func (q *PendingMessageQueue) clearRestoredLocked(sessionID string) {
	delete(q.restored, sessionID)
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
	q.schedulePersist()
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
	q.schedulePersist()
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
		id := queue[last].ID
		q.mu.Unlock()
		q.schedulePersist()
		return id
	}
	if len(queue) >= MaxPendingPerSession {
		q.mu.Unlock()
		return ""
	}
	id := uuid.NewString()
	q.queues[sessionID] = append(queue, PendingMessage{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	q.mu.Unlock()
	q.schedulePersist()
	return id
}

// FlushLeadingInjects atomically removes and returns all entries when the
// ENTIRE queue for the session consists of inject-kind entries (N2 满队
// 死锁冲刷，session-eval-20260827 S12 / C5-③)：inject 不单独唤醒 turn，只能
// 随 followup 合入；满队全 inject 时新 followup 被容量拒绝、inject 永远等
// 不到合入载体（「已接受但永滞留」）。调用方（enqueue 满员分支）拿到滞留
// inject 后将其内容并入新消息一并入队。队列含任何 followup（既有出队循环
// 会正常冲刷）或为空时返回 nil，不动队列。
func (q *PendingMessageQueue) FlushLeadingInjects(sessionID string) []PendingMessage {
	q.mu.Lock()
	queue := q.queues[sessionID]
	if len(queue) == 0 {
		q.mu.Unlock()
		return nil
	}
	for _, e := range queue {
		if e.Kind != "inject" {
			q.mu.Unlock()
			return nil
		}
	}
	flushed := append([]PendingMessage(nil), queue...)
	delete(q.queues, sessionID)
	q.clearRestoredLocked(sessionID)
	q.mu.Unlock()
	q.schedulePersist()
	return flushed
}

// DequeueLeadingInjects atomically removes and returns the leading run of
// inject-kind entries (N2 上下文合入，新 turn 起步时消费滞留 inject)。
// 与「Peek 判头 + 循环 Dequeue」的两阶段写法不同，全程单锁持有：两阶段
// 写法在并发出队（processPendingQueue 等）下会把非 inject 的头部条目
// （用户 followup）误出队并静默丢弃。首个非 inject 条目就地保留。
func (q *PendingMessageQueue) DequeueLeadingInjects(sessionID string) []PendingMessage {
	q.mu.Lock()
	queue := q.queues[sessionID]
	n := 0
	for n < len(queue) && queue[n].Kind == "inject" {
		n++
	}
	if n == 0 {
		q.mu.Unlock()
		return nil
	}
	out := append([]PendingMessage(nil), queue[:n]...)
	if n == len(queue) {
		delete(q.queues, sessionID)
		q.clearRestoredLocked(sessionID)
	} else {
		q.queues[sessionID] = append([]PendingMessage(nil), queue[n:]...)
	}
	q.mu.Unlock()
	q.schedulePersist()
	return out
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
		q.clearRestoredLocked(sessionID)
		q.mu.Unlock()
		return PendingMessage{}, false
	}
	head := queue[0]
	if len(queue) == 1 {
		delete(q.queues, sessionID)
		q.clearRestoredLocked(sessionID)
	} else {
		q.queues[sessionID] = append([]PendingMessage(nil), queue[1:]...)
	}
	q.mu.Unlock()
	q.schedulePersist()
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
				q.clearRestoredLocked(sessionID)
			} else {
				q.queues[sessionID] = append([]PendingMessage(nil), queue...)
			}
			q.mu.Unlock()
			q.schedulePersist()
			return true
		}
	}
	q.mu.Unlock()
	return false
}

// writeThrough synchronously persists the current snapshot (C-12) and optional store.
// Must not be called while holding q.mu.
func (q *PendingMessageQueue) writeThrough() {
	if q == nil {
		return
	}
	if q.store != nil {
		q.persistStore()
	}
	if q.snapshot {
		q.saveSnapshot()
	}
}

// schedulePersist coalesces hot-path mutations into a batched durable flush
// (M6). Callers must not hold q.mu.
func (q *PendingMessageQueue) schedulePersist() {
	if q == nil {
		return
	}
	q.persistMu.Lock()
	if q.persistTimer == nil {
		q.persistTimer = time.AfterFunc(pendingPersistFlushInterval, q.flushPersist)
	}
	q.persistMu.Unlock()
}

// flushPersist runs the batched durable write. flushMu serializes concurrent
// flushes (timer fire vs Close final flush) and the snapshot is taken inside
// writeThrough at flush time, so writes reach the store in fresh-data order.
func (q *PendingMessageQueue) flushPersist() {
	q.flushMu.Lock()
	defer q.flushMu.Unlock()
	q.persistMu.Lock()
	q.persistTimer = nil
	q.persistMu.Unlock()
	q.writeThrough()
}

func (q *PendingMessageQueue) persistStore() {
	q.mu.Lock()
	snapshot := make(map[string][]PendingMessage, len(q.queues))
	for k, v := range q.queues {
		snapshot[k] = append([]PendingMessage(nil), v...)
	}
	q.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.store.ReplaceAll(ctx, snapshot); err != nil {
		q.lg.Warn("pending queue store write failed", loggateway.StepID("runtime.pending_queue.store"), loggateway.Err(err))
	}
}

// SetStore attaches a durable store and prefers it over the file snapshot on load.
func (q *PendingMessageQueue) SetStore(store PendingQueueStore) {
	if q == nil || store == nil {
		return
	}
	q.store = store
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	loaded, err := store.LoadAll(ctx)
	if err != nil {
		// H3 (2026-10-18): NEVER push in-memory state into the store when the
		// load failed — at startup memory is empty (dir="") or a stale file
		// snapshot, so ReplaceAll would wipe durable rows on a transient DB
		// error. Continue memory-only; the first real mutation reconciles.
		q.lg.Warn("pending queue store load failed, continuing memory-only",
			loggateway.StepID("runtime.pending_queue.store"), loggateway.Err(err))
		return
	}
	if len(loaded) > 0 {
		q.mu.Lock()
		q.queues = loaded
		// FR-7：store 内容权威——恢复集以其键集合整体重建（覆盖文件快照期的
		// 试探性标记），随后队列排空时在出队分支逐会话清除。
		q.restored = make(map[string]struct{}, len(loaded))
		for sid := range loaded {
			q.markRestoredLocked(sid)
		}
		q.mu.Unlock()
		if q.snapshot {
			q.saveSnapshot()
		}
		return
	}
	// Store load succeeded and is empty: push in-memory state only when there
	// is something to migrate (file snapshot → store). Pushing an empty map
	// would be a no-op anyway, but skipping keeps the intent explicit.
	q.mu.Lock()
	pending := len(q.queues)
	q.mu.Unlock()
	if pending > 0 {
		q.writeThrough()
	}
}

func (q *PendingMessageQueue) Update(sessionID, entryID, newContent string) bool {
	q.mu.Lock()
	ok := false
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
			ok = true
			break
		}
	}
	q.mu.Unlock()
	if ok {
		q.schedulePersist()
	}
	return ok
}

// PromoteToFront moves the specified message to the front of the session queue.
func (q *PendingMessageQueue) PromoteToFront(sessionID, pendingID string) error {
	q.mu.Lock()
	var err error
	moved := false
	queue := q.queues[sessionID]
	found := false
	for i, e := range queue {
		if e.ID == pendingID {
			found = true
			if i == 0 {
				break
			}
			entry := queue[i]
			queue = append(queue[:i], queue[i+1:]...)
			queue = append([]PendingMessage{entry}, queue...)
			q.queues[sessionID] = queue
			moved = true
			break
		}
	}
	if !found {
		err = fmt.Errorf("pending message %s not found in session %s", pendingID, sessionID)
	}
	q.mu.Unlock()
	if moved {
		q.schedulePersist()
	}
	return err
}

// SetPriority sets the priority of the specified message (0=normal, 1=high).
func (q *PendingMessageQueue) SetPriority(sessionID, pendingID string, priority int) error {
	q.mu.Lock()
	var err error
	ok := false
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
			ok = true
			break
		}
	}
	if !ok {
		err = fmt.Errorf("pending message %s not found in session %s", pendingID, sessionID)
	}
	q.mu.Unlock()
	if ok {
		q.schedulePersist()
	}
	return err
}

// Close is idempotent (M4): stops the snapshot loop, cancels any pending
// batched flush, and performs one final synchronous durable write.
func (q *PendingMessageQueue) Close() {
	if q == nil {
		return
	}
	q.closeOnce.Do(func() {
		if q.snapshot {
			close(q.stopCh)
		}
		q.persistMu.Lock()
		if q.persistTimer != nil {
			q.persistTimer.Stop()
			q.persistTimer = nil
		}
		q.persistMu.Unlock()
		// flushMu so a timer flush already in flight cannot overwrite this
		// final flush with an older snapshot.
		q.flushMu.Lock()
		defer q.flushMu.Unlock()
		q.writeThrough()
	})
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
