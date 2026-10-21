package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// --- P1 FR-9：outbox (session_id, seq) 冲突 → 重取共享 seq 重试一次 ---

// fr9ConflictOutbox 在既有 fakeOutbox 上叠加可控失败：failNext 计次返回
// biz.ErrEventOutboxSeqConflict 哨兵，alwaysErr 恒返回非哨兵错误。
type fr9ConflictOutbox struct {
	fakeOutbox
	failNext   int
	alwaysErr  error
	insertCalls int
}

func (f *fr9ConflictOutbox) Insert(ctx context.Context, row biz.EventDeliveryOutboxRow) error {
	f.mu.Lock()
	f.insertCalls++
	if f.alwaysErr != nil {
		f.mu.Unlock()
		return f.alwaysErr
	}
	if f.failNext > 0 {
		f.failNext--
		sessionID, seq := row.SessionID, row.Seq
		f.mu.Unlock()
		return fmt.Errorf("%w: session=%s seq=%d", biz.ErrEventOutboxSeqConflict, sessionID, seq)
	}
	f.mu.Unlock()
	return f.fakeOutbox.Insert(ctx, row)
}

func fr9TestSequencer(t *testing.T, outbox *fr9ConflictOutbox) *Sequencer {
	t.Helper()
	s := NewSequencer(&fakeRepoSet{}, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithEventOutbox(outbox),
	)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestSequencer_OutboxSeqConflict_RetrySucceeds 钉死 FR-9 核心语义：首次
// Insert 撞 (session_id, seq) UNIQUE 冲突 ⇒ 共享 SeqAssigner 落后于持久水位；
// sequencer 重取 seq（1→2）、重新 marshal payload（event_id 内嵌）、重试一次
// 成功。event 的 live 帧序号同步抬到 2，MarkPublished 归属重试行。
func TestSequencer_OutboxSeqConflict_RetrySucceeds(t *testing.T) {
	t.Parallel()

	outbox := &fr9ConflictOutbox{failNext: 1}
	s := fr9TestSequencer(t, outbox)

	ctx := context.Background()
	task := biz.Task{ID: "task-fr9", SessionID: "sess-1", Status: biz.TaskStatusCompleted, Seq: 9, Version: 2}
	ev := biz.NewTaskCompletedEvent(task)
	s.Publish(ctx, ev)
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.insertCalls != 2 {
		t.Fatalf("P1 FR-9: Insert calls = %d, want 2 (conflict + one retry)", outbox.insertCalls)
	}
	if len(outbox.inserted) != 1 {
		t.Fatalf("outbox durable rows = %d, want 1", len(outbox.inserted))
	}
	row := outbox.inserted[0]
	if row.Seq != 2 || row.SessionID != "sess-1" {
		t.Fatalf("P1 FR-9: retry row seq/session = %d/%q, want 2/sess-1", row.Seq, row.SessionID)
	}
	wantID := biz.DeliveryEventID(ev, 2)
	if row.EventID != wantID {
		t.Fatalf("P1 FR-9: retry row event_id = %q, want %q", row.EventID, wantID)
	}
	if got := ev.DeliverySeqValue(); got != 2 {
		t.Fatalf("P1 FR-9: event delivery seq = %d, want 2 (live frame follows retry)", got)
	}
	// payload 必须按新 seq 重新 marshal——否则 durable 行与 live 帧 event_id 分叉。
	var env map[string]any
	if err := json.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if env["event_id"] != wantID {
		t.Fatalf("P1 FR-9: envelope event_id = %v, want %s (payload re-marshaled with new seq)", env["event_id"], wantID)
	}
	// MarkPublished 必须打在重试成功的行 ID 上（而非冲突行的孤儿 ID）。
	if len(outbox.markPublished) != 1 || outbox.markPublished[0] != row.ID {
		t.Fatalf("P1 FR-9: MarkPublished = %v, want [%s]", outbox.markPublished, row.ID)
	}
}

// TestSequencer_OutboxSeqConflict_NonConflictNoRetry：非哨兵错误（普通 DB
// 故障）不触发重试——一次 Insert 失败即回落 best-effort 跳过，无 panic。
func TestSequencer_OutboxSeqConflict_NonConflictNoRetry(t *testing.T) {
	t.Parallel()

	outbox := &fr9ConflictOutbox{alwaysErr: errors.New("db down")}
	s := fr9TestSequencer(t, outbox)

	ctx := context.Background()
	task := biz.Task{ID: "task-fr9-nc", SessionID: "sess-1", Status: biz.TaskStatusCompleted, Seq: 1, Version: 1}
	s.Publish(ctx, biz.NewTaskCompletedEvent(task))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.insertCalls != 1 {
		t.Fatalf("P1 FR-9: Insert calls = %d, want 1 (non-conflict must not retry)", outbox.insertCalls)
	}
	if len(outbox.inserted) != 0 {
		t.Fatalf("outbox durable rows = %d, want 0", len(outbox.inserted))
	}
	if len(outbox.markPublished) != 0 {
		t.Fatalf("MarkPublished = %v, want empty (no durable row to mark)", outbox.markPublished)
	}
}

// TestSequencer_OutboxSeqConflict_BothAttemptsFail：重试仍失败 ⇒ 完全回落既
// 有 best-effort 语义（无落库行、无 MarkPublished、无 panic）。
func TestSequencer_OutboxSeqConflict_BothAttemptsFail(t *testing.T) {
	t.Parallel()

	outbox := &fr9ConflictOutbox{failNext: 2}
	s := fr9TestSequencer(t, outbox)

	ctx := context.Background()
	task := biz.Task{ID: "task-fr9-2x", SessionID: "sess-1", Status: biz.TaskStatusCompleted, Seq: 1, Version: 1}
	s.Publish(ctx, biz.NewTaskCompletedEvent(task))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.insertCalls != 2 {
		t.Fatalf("P1 FR-9: Insert calls = %d, want 2 (initial + exactly one retry)", outbox.insertCalls)
	}
	if len(outbox.inserted) != 0 {
		t.Fatalf("outbox durable rows = %d, want 0", len(outbox.inserted))
	}
	if len(outbox.markPublished) != 0 {
		t.Fatalf("MarkPublished = %v, want empty", outbox.markPublished)
	}
}
