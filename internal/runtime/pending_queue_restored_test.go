package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakePendingStore 实现 PendingQueueStore，记录 ReplaceAll 调用。
type fakePendingStore struct {
	rows       map[string][]PendingMessage
	loadErr    error
	replaceCnt int
}

func (s *fakePendingStore) LoadAll(context.Context) (map[string][]PendingMessage, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.rows, nil
}

func (s *fakePendingStore) ReplaceAll(context.Context, map[string][]PendingMessage) error {
	s.replaceCnt++
	return nil
}

func pendingEntry(id, content string) PendingMessage {
	return PendingMessage{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}

// TestPendingQueue_SetStoreMarksRestored 复现 P1 FR-7 前提：重启后
// SetStore.LoadAll 填充的队列条目必须被标记为"恢复集"——否则新消息无法
// 区分「我会话空」与「我有待排空的恢复行」，FIFO 反转无法检测。
func TestPendingQueue_SetStoreMarksRestored(t *testing.T) {
	store := &fakePendingStore{rows: map[string][]PendingMessage{
		"s-restored": {pendingEntry("f1", "F1"), pendingEntry("f2", "F2")},
	}}
	q := NewPendingMessageQueue()
	q.SetStore(store)

	if !q.IsRestored("s-restored") {
		t.Fatal("P1 FR-7: sessions whose queues were loaded from the durable store must be marked restored")
	}
	if q.IsRestored("s-fresh") {
		t.Fatal("session without restored rows must not be marked")
	}
}

// TestPendingQueue_RestoredClearedWhenDrained 恢复标记生命周期终止条件：
// 会话队列完全排空（Dequeue 至空 / Remove 末条）时清除。
func TestPendingQueue_RestoredClearedWhenDrained(t *testing.T) {
	store := &fakePendingStore{rows: map[string][]PendingMessage{
		"s-dequeue": {pendingEntry("f1", "F1"), pendingEntry("f2", "F2")},
		"s-remove":  {pendingEntry("f3", "F3")},
	}}
	q := NewPendingMessageQueue()
	q.SetStore(store)

	if _, ok := q.Dequeue("s-dequeue"); !ok {
		t.Fatal("precondition: first dequeue")
	}
	if !q.IsRestored("s-dequeue") {
		t.Fatal("restored mark must survive while restored rows remain")
	}
	if _, ok := q.Dequeue("s-dequeue"); !ok {
		t.Fatal("precondition: second dequeue")
	}
	if q.IsRestored("s-dequeue") {
		t.Fatal("restored mark must be cleared once the queue is fully drained")
	}

	if !q.Remove("s-remove", "f3") {
		t.Fatal("precondition: remove last entry")
	}
	if q.IsRestored("s-remove") {
		t.Fatal("restored mark must be cleared when Remove empties the queue")
	}
}

// TestPendingQueue_SnapshotRestoreMarksRestored：文件快照恢复路径（无 DB
// store 的部署形态）同样进入恢复集。
func TestPendingQueue_SnapshotRestoreMarksRestored(t *testing.T) {
	dir := t.TempDir()
	snapshot := map[string][]PendingMessage{
		"s-snap": {pendingEntry("f1", "F1")},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pending_queue.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	q := NewPendingMessageQueueWithDir(dir)
	defer q.Close()
	if !q.IsRestored("s-snap") {
		t.Fatal("P1 FR-7: sessions restored from the file snapshot must be marked restored")
	}
	if _, ok := q.Dequeue("s-snap"); !ok {
		t.Fatal("precondition: dequeue restored entry")
	}
	if q.IsRestored("s-snap") {
		t.Fatal("snapshot-restored mark must clear on drain")
	}
}

// TestPendingQueue_SetStoreLoadFailureNoMarks：LoadAll 失败时不得标记任何
// 恢复集（H3 语义：内存态照旧，后续首个真实写入自行对账）。
func TestPendingQueue_SetStoreLoadFailureNoMarks(t *testing.T) {
	q := NewPendingMessageQueue()
	q.SetStore(&fakePendingStore{loadErr: errors.New("db down")})
	if q.IsRestored("s-restored") {
		t.Fatal("failed load must not mark restored sessions")
	}
}

// TestPendingQueue_FlushSession 级联清理锚点（P1 FR-7 删除/归档会话）：
// 清内存队列 + 恢复标记，并调度一次持久层冲刷（防批量 ReplaceAll 把已删
// 会话的条目重新插回造成 FK 违例/幽灵行复活）。
func TestPendingQueue_FlushSession(t *testing.T) {
	store := &fakePendingStore{rows: map[string][]PendingMessage{
		"s-flush": {pendingEntry("f1", "F1")},
	}}
	q := NewPendingMessageQueue()
	q.SetStore(store)
	q.Enqueue("s-flush", "NEW")

	q.FlushSession("s-flush")

	if n := len(q.List("s-flush")); n != 0 {
		t.Fatalf("flush must clear memory queue, got %d entries", n)
	}
	if q.IsRestored("s-flush") {
		t.Fatal("flush must clear the restored mark")
	}
}
