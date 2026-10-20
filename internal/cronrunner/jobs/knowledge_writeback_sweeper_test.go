package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

type fakeWriteBackOutbox struct {
	rows       []biz.KnowledgeWriteBackOutboxRow
	enqueued   []string
	attempted  []int64
	published  []int64
	dead       []int64
	enqueueErr error
	listErr    error
}

func (f *fakeWriteBackOutbox) EnqueueWriteBackOutbox(_ context.Context, batchKey string, _ biz.KnowledgeWriteBackInput) error {
	if f.enqueueErr != nil {
		return f.enqueueErr
	}
	f.enqueued = append(f.enqueued, batchKey)
	return nil
}

func (f *fakeWriteBackOutbox) ListPendingWriteBackOutbox(_ context.Context, limit int) ([]biz.KnowledgeWriteBackOutboxRow, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit > 0 && len(f.rows) > limit {
		return f.rows[:limit], nil
	}
	return f.rows, nil
}

func (f *fakeWriteBackOutbox) MarkWriteBackOutboxAttempt(_ context.Context, id int64, _ string) error {
	f.attempted = append(f.attempted, id)
	return nil
}

func (f *fakeWriteBackOutbox) MarkWriteBackOutboxPublished(_ context.Context, ids []int64, _ time.Time) error {
	f.published = append(f.published, ids...)
	return nil
}

func (f *fakeWriteBackOutbox) MarkWriteBackOutboxDead(_ context.Context, ids []int64, _ string) error {
	f.dead = append(f.dead, ids...)
	return nil
}

func TestKnowledgeWriteBackSweeper_ReplaysPendingBatch(t *testing.T) {
	in := biz.KnowledgeWriteBackInput{
		Workspace: "default", SessionID: "sess-1", AgentID: "a-1", UserID: "u-1",
		Facts: []biz.KnowledgeWriteBackFact{{FactID: "f-1", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.95}},
	}
	payload, err := bizknowledge.EncodeWriteBackOutboxPayload(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	outbox := &fakeWriteBackOutbox{rows: []biz.KnowledgeWriteBackOutboxRow{
		{ID: 7, BatchKey: "wb:sess-1:x", Payload: string(payload), Status: bizknowledge.WriteBackOutboxStatusPending},
	}}
	wb := &capturingWriteBack{}
	w := NewKnowledgeWriteBackSweeperWorker(0, outbox, wb, loggateway.NewNoop())
	if w == nil {
		t.Fatal("worker nil with valid deps")
	}
	published, dead := w.RunOnce(context.Background())
	if published != 1 || dead != 0 {
		t.Fatalf("RunOnce = (%d,%d), want (1,0)", published, dead)
	}
	if wb.calls != 1 || wb.got.SessionID != "sess-1" || len(wb.got.Facts) != 1 {
		t.Fatalf("writeback got = %+v calls=%d", wb.got, wb.calls)
	}
	if len(outbox.published) != 1 || outbox.published[0] != 7 {
		t.Fatalf("published marks = %+v", outbox.published)
	}
	if len(outbox.attempted) != 0 {
		t.Fatalf("unexpected attempts = %+v", outbox.attempted)
	}
}

func TestKnowledgeWriteBackSweeper_FailureMarksAttemptPoisonMarksDead(t *testing.T) {
	in := biz.KnowledgeWriteBackInput{SessionID: "sess-2", Facts: []biz.KnowledgeWriteBackFact{{Statement: "x 事实，足够长度一"}}}
	payload, _ := bizknowledge.EncodeWriteBackOutboxPayload(in)
	outbox := &fakeWriteBackOutbox{rows: []biz.KnowledgeWriteBackOutboxRow{
		{ID: 1, Payload: string(payload), Status: "pending"}, // 重放失败 → attempt
		{ID: 2, Payload: `{bad json`, Status: "pending"},     // poison → dead
	}}
	wb := &capturingWriteBack{err: errors.New("kb down")}
	w := NewKnowledgeWriteBackSweeperWorker(0, outbox, wb, loggateway.NewNoop())
	published, dead := w.RunOnce(context.Background())
	if published != 0 || dead != 1 {
		t.Fatalf("RunOnce = (%d,%d), want (0,1)", published, dead)
	}
	if len(outbox.attempted) != 1 || outbox.attempted[0] != 1 {
		t.Fatalf("attempted = %+v", outbox.attempted)
	}
	if len(outbox.dead) != 1 || outbox.dead[0] != 2 {
		t.Fatalf("dead = %+v", outbox.dead)
	}
	if len(outbox.published) != 0 {
		t.Fatalf("unexpected published = %+v", outbox.published)
	}
}

func TestKnowledgeWriteBackSweeper_NilDepsNotConstructed(t *testing.T) {
	if w := NewKnowledgeWriteBackSweeperWorker(0, nil, &capturingWriteBack{}, loggateway.NewNoop()); w != nil {
		t.Fatal("want nil worker when outbox missing")
	}
	if w := NewKnowledgeWriteBackSweeperWorker(0, &fakeWriteBackOutbox{}, nil, loggateway.NewNoop()); w != nil {
		t.Fatal("want nil worker when writeBack missing")
	}
}
