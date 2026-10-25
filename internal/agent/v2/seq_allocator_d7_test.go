package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// --- D7-1：ARANEA_SEQ_ALLOCATOR=pg 事务内取号路径钉板（deferred 战役） ---

// TestResolveSeqAllocatorMode 钉死 flag 解析矩阵：off 缺省/非法值回落，
// 大小写/空白容忍，布尔习惯值（"true"/"1"）回落 off（枚举不接受布尔真值，
// 与 ARANEA_EXECUTOR_WAITER 解析同律）。
func TestResolveSeqAllocatorMode(t *testing.T) {
	t.Parallel()
	if got := ResolveSeqAllocatorMode(nil); got != SeqAllocatorModeOff {
		t.Fatalf("nil getenv = %q, want off", got)
	}
	for _, c := range []struct{ in, want string }{
		{"", SeqAllocatorModeOff},
		{"off", SeqAllocatorModeOff},
		{"OFF", SeqAllocatorModeOff},
		{"pg", SeqAllocatorModePG},
		{"PG", SeqAllocatorModePG},
		{" pg ", SeqAllocatorModePG},
		{"true", SeqAllocatorModeOff},
		{"1", SeqAllocatorModeOff},
		{"mem", SeqAllocatorModeOff},
	} {
		getenv := func(string) string { return c.in }
		if got := ResolveSeqAllocatorMode(getenv); got != c.want {
			t.Errorf("ResolveSeqAllocatorMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// d7AllocatingOutbox 在 fakeOutbox 上叠加分配端口（biz.EventOutboxSeqAllocatingInserter）：
// 逐次发号 → build(seq) 建行 → 计入 inserted；allocErr 注入事务级故障。
type d7AllocatingOutbox struct {
	fakeOutbox
	allocCalls int
	allocErr   error
	nextSeq    int64
}

func (f *d7AllocatingOutbox) InsertAllocatingSeq(ctx context.Context, sessionID string, build func(seq int64) (biz.EventDeliveryOutboxRow, error)) error {
	f.mu.Lock()
	f.allocCalls++
	f.nextSeq++
	seq := f.nextSeq
	allocErr := f.allocErr
	f.mu.Unlock()
	if allocErr != nil {
		return allocErr
	}
	row, err := build(seq)
	if err != nil {
		return err
	}
	return f.Insert(ctx, row)
}

func d7AllocSequencer(t *testing.T, outbox *d7AllocatingOutbox) *Sequencer {
	t.Helper()
	s := NewSequencer(&fakeRepoSet{}, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithEventOutbox(outbox),
		WithEventOutboxSeqAllocator(outbox),
	)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func d7CriticalEvent(sessionID, taskID string) biz.Event {
	return biz.NewTaskCompletedEvent(biz.Task{
		ID: taskID, SessionID: sessionID,
		Status: biz.TaskStatusCompleted, Seq: 1, Version: 1,
	})
}

// TestInsertCriticalOutbox_PGAllocatesViaPort：pg 档关键事件 seq 全部来自
// 事务内分配端口（逐次 1,2…），事件被打戳、durable 行与 live 帧 event_id
// 一致；进程内 Go 计数器零消耗（关键事件序完全脱离 Go 号段）。
func TestInsertCriticalOutbox_PGAllocatesViaPort(t *testing.T) {
	t.Parallel()

	outbox := &d7AllocatingOutbox{}
	s := d7AllocSequencer(t, outbox)
	ctx := context.Background()

	ev1 := d7CriticalEvent("sess-1", "task-a")
	id1 := s.insertCriticalOutbox(ctx, ev1)
	if id1 == "" {
		t.Fatal("pg 档 insert 应返回行 ID")
	}
	de1 := ev1.(biz.DeliverySeqEvent)
	if got := de1.DeliverySeqValue(); got != 1 {
		t.Fatalf("event1 delivery seq = %d, want 1", got)
	}

	ev2 := d7CriticalEvent("sess-1", "task-b")
	if id2 := s.insertCriticalOutbox(ctx, ev2); id2 == "" {
		t.Fatal("pg 档第二次 insert 应返回行 ID")
	}
	if got := ev2.(biz.DeliverySeqEvent).DeliverySeqValue(); got != 2 {
		t.Fatalf("event2 delivery seq = %d, want 2", got)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.allocCalls != 2 {
		t.Fatalf("allocator calls = %d, want 2", outbox.allocCalls)
	}
	if len(outbox.inserted) != 2 {
		t.Fatalf("durable rows = %d, want 2", len(outbox.inserted))
	}
	for i, row := range outbox.inserted {
		wantSeq := int64(i + 1)
		if row.Seq != wantSeq || row.SessionID != "sess-1" {
			t.Fatalf("row[%d] seq/session = %d/%q, want %d/sess-1", i, row.Seq, row.SessionID, wantSeq)
		}
	}
	// durable 行 event_id 必须内嵌分配 seq（与 live 帧同式）。
	if want := biz.DeliveryEventID(ev1, 1); outbox.inserted[0].EventID != want {
		t.Fatalf("row[0] event_id = %q, want %q", outbox.inserted[0].EventID, want)
	}
	// Go 计数器零消耗：此时直接取号必须仍得 1（pg 档关键事件不碰 Go 号段）。
	if got := s.seqAssigner.NextSeq("sess-1"); got != 1 {
		t.Fatalf("Go counter consumed by pg path: NextSeq = %d, want 1", got)
	}
}

// TestInsertCriticalOutbox_PGErrorStripsSeq：分配事务失败 ⇒ E2E-C1 同律剥
// seq（live 帧不带幻影游标）、无落库行、无 panic。
func TestInsertCriticalOutbox_PGErrorStripsSeq(t *testing.T) {
	t.Parallel()

	outbox := &d7AllocatingOutbox{allocErr: errors.New("pg tx failed")}
	s := d7AllocSequencer(t, outbox)

	ev := d7CriticalEvent("sess-1", "task-x")
	if id := s.insertCriticalOutbox(context.Background(), ev); id != "" {
		t.Fatalf("失败路径应返回空 ID，got %q", id)
	}
	if got := ev.(biz.DeliverySeqEvent).DeliverySeqValue(); got != 0 {
		t.Fatalf("E2E-C1: 失败后 delivery seq = %d, want 0（剥戳）", got)
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.allocCalls != 1 {
		t.Fatalf("allocator calls = %d, want 1", outbox.allocCalls)
	}
	if len(outbox.inserted) != 0 {
		t.Fatalf("durable rows = %d, want 0", len(outbox.inserted))
	}
}

// TestInsertCriticalOutbox_PGPreStampedSeqBypassesAllocator：已打戳事件
// （同一事件对象重发布/崩溃重试窗口）走直插幂等径，分配端口零调用——
// 同 event_id 幂等语义与 off 档一致。
func TestInsertCriticalOutbox_PGPreStampedSeqBypassesAllocator(t *testing.T) {
	t.Parallel()

	outbox := &d7AllocatingOutbox{}
	s := d7AllocSequencer(t, outbox)

	ev := d7CriticalEvent("sess-1", "task-prestamped")
	ev.(biz.DeliverySeqEvent).SetDeliverySeqValue(7)
	if id := s.insertCriticalOutbox(context.Background(), ev); id == "" {
		t.Fatal("预打戳直插应返回行 ID")
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.allocCalls != 0 {
		t.Fatalf("预打戳事件不得触碰分配端口，calls = %d", outbox.allocCalls)
	}
	if len(outbox.inserted) != 1 || outbox.inserted[0].Seq != 7 {
		t.Fatalf("直插行 = %+v, want 单行 seq=7", outbox.inserted)
	}
}

// TestInsertCriticalOutbox_OffModeByteLevelUnchanged：不挂分配端口（off 档
// 字节级旧行为）——seq 来自进程内 Go 计数器，行照常落库。
func TestInsertCriticalOutbox_OffModeByteLevelUnchanged(t *testing.T) {
	t.Parallel()

	outbox := &fakeOutbox{}
	s := NewSequencer(&fakeRepoSet{}, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithEventOutbox(outbox),
	)
	t.Cleanup(func() { _ = s.Close() })

	ev := d7CriticalEvent("sess-1", "task-off")
	if id := s.insertCriticalOutbox(context.Background(), ev); id == "" {
		t.Fatal("off 档 insert 应返回行 ID")
	}
	if got := ev.(biz.DeliverySeqEvent).DeliverySeqValue(); got != 1 {
		t.Fatalf("off 档 Go 计数器首发 = %d, want 1", got)
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if len(outbox.inserted) != 1 || outbox.inserted[0].Seq != 1 {
		t.Fatalf("off 档落库行 = %+v, want 单行 seq=1", outbox.inserted)
	}
}
