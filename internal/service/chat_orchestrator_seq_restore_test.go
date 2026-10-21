package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// --- P1 FR-9：共享计数器恢复折入 outbox 水位 ---

type fr9StepReader struct {
	biz.StepV2Reader
	maxSeq int64
	err    error
}

func (r *fr9StepReader) MaxSeqBySpiritSession(context.Context, string) (int64, error) {
	return r.maxSeq, r.err
}

type fr9TaskReader struct {
	biz.TaskV2Repo
	maxSeq int64
	err    error
}

func (r *fr9TaskReader) MaxSeqBySession(context.Context, string) (int64, error) {
	return r.maxSeq, r.err
}

// fr9Outbox 同时实现基座端口与 FR-9 窄口水位端口。
type fr9Outbox struct {
	maxSeq int64
	err    error
}

func (f *fr9Outbox) Insert(context.Context, biz.EventDeliveryOutboxRow) error { return nil }
func (f *fr9Outbox) MarkPublished(context.Context, string, time.Time) error   { return nil }
func (f *fr9Outbox) MarkPublishedBatch(context.Context, []string, time.Time) error {
	return nil
}
func (f *fr9Outbox) ListAfter(context.Context, string, string, int64, int) ([]biz.EventDeliveryOutboxRow, error) {
	return nil, nil
}
func (f *fr9Outbox) ListPending(context.Context, time.Time, int) ([]biz.EventDeliveryOutboxRow, error) {
	return nil, nil
}
func (f *fr9Outbox) MaxSeqBySession(context.Context, string) (int64, error) { return f.maxSeq, f.err }

// fr9NarrowOutbox 只实现基座端口（无水位窄口）——断言必须静默跳过而非 panic。
type fr9NarrowOutbox struct{ fr9OutboxBase }

type fr9OutboxBase struct{}

func (fr9OutboxBase) Insert(context.Context, biz.EventDeliveryOutboxRow) error { return nil }
func (fr9OutboxBase) MarkPublished(context.Context, string, time.Time) error   { return nil }
func (fr9OutboxBase) MarkPublishedBatch(context.Context, []string, time.Time) error {
	return nil
}
func (fr9OutboxBase) ListAfter(context.Context, string, string, int64, int) ([]biz.EventDeliveryOutboxRow, error) {
	return nil, nil
}
func (fr9OutboxBase) ListPending(context.Context, time.Time, int) ([]biz.EventDeliveryOutboxRow, error) {
	return nil, nil
}

// TestRestoredSharedSeqMax_FoldsOutboxWatermark 钉死 FR-9 核心语义：恢复基
// 线 = max(steps, tasks, outbox)。B-06 仅 steps、H2 折 tasks——最后计数分配
// 落在关键事件时（outbox 独有 seq=13）两位旧水位（8/5）会把 SeqAssigner
// 重启到 9，下条 critical insert 撞 UNIQUE(session_id, seq) 静默丢帧。
func TestRestoredSharedSeqMax_FoldsOutboxWatermark(t *testing.T) {
	o := &ChatOrchestrator{
		core:      chatTurnCoreDeps{StepReader: &fr9StepReader{maxSeq: 8}, TaskV2: &fr9TaskReader{maxSeq: 5}},
		infraDeps: ChatInfraDeps{EventOutbox: &fr9Outbox{maxSeq: 13}},
	}
	maxSeq, ok := o.restoredSharedSeqMax(context.Background(), "sess-1")
	if !ok || maxSeq != 13 {
		t.Fatalf("P1 FR-9: expected outbox watermark 13 to win, got (%d, %v)", maxSeq, ok)
	}
}

// TestRestoredSharedSeqMax_StepsTasksOnly：outbox 缺席（nil 或窄口未实现）
// 退回 B-06/H2 两位水位语义。
func TestRestoredSharedSeqMax_StepsTasksOnly(t *testing.T) {
	o := &ChatOrchestrator{
		core: chatTurnCoreDeps{StepReader: &fr9StepReader{maxSeq: 8}, TaskV2: &fr9TaskReader{maxSeq: 5}},
	}
	maxSeq, ok := o.restoredSharedSeqMax(context.Background(), "sess-1")
	if !ok || maxSeq != 8 {
		t.Fatalf("expected steps watermark 8 without outbox, got (%d, %v)", maxSeq, ok)
	}
	o.infraDeps.EventOutbox = &fr9NarrowOutbox{}
	maxSeq, ok = o.restoredSharedSeqMax(context.Background(), "sess-1")
	if !ok || maxSeq != 8 {
		t.Fatalf("expected narrow outbox silently skipped, got (%d, %v)", maxSeq, ok)
	}
}

// TestRestoredSharedSeqMax_ToleratesPartialFailure：单源失败不拖垮其余——
// steps 读失败时 outbox 水位仍可独立成立（原子 max 语义，区别于 B-06 的嵌
// 套容错：steps 失败则 task 一并跳过）。
func TestRestoredSharedSeqMax_ToleratesPartialFailure(t *testing.T) {
	o := &ChatOrchestrator{
		core: chatTurnCoreDeps{
			StepReader: &fr9StepReader{err: errors.New("db down")},
			TaskV2:     &fr9TaskReader{maxSeq: 5},
		},
		infraDeps: ChatInfraDeps{EventOutbox: &fr9Outbox{maxSeq: 13}},
	}
	maxSeq, ok := o.restoredSharedSeqMax(context.Background(), "sess-1")
	if !ok || maxSeq != 13 {
		t.Fatalf("expected outbox 13 despite steps failure, got (%d, %v)", maxSeq, ok)
	}
}

// TestRestoredSharedSeqMax_AllMissing：全部缺席/全失败 → ok=false（沿用出
// 厂 1 起始，B-06 前既有行为）。
func TestRestoredSharedSeqMax_AllMissing(t *testing.T) {
	o := &ChatOrchestrator{}
	if _, ok := o.restoredSharedSeqMax(context.Background(), "sess-1"); ok {
		t.Fatal("expected ok=false when no watermark source available")
	}
}

// 编译期守卫：fr9Outbox 同时满足基座与窄口；fr9NarrowOutbox 只满足基座。
var (
	_ biz.EventDeliveryOutboxRepo     = (*fr9Outbox)(nil)
	_ biz.EventOutboxWatermarkReader  = (*fr9Outbox)(nil)
	_ biz.EventDeliveryOutboxRepo     = fr9NarrowOutbox{}
	_ biz.EventDeliveryOutboxRepo     = (*fr9Outbox)(nil)
	_ interface{ MaxSeqBySession(context.Context, string) (int64, error) } = (*fr9Outbox)(nil)
)
