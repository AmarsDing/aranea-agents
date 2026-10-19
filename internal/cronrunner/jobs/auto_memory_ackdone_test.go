package jobs

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

func TestHandleDequeued_AckDoneOnCancelledContext(t *testing.T) {
	rc := &conf.Runtime{
		MemoryQueue: &conf.Runtime_MemoryQueue{MaxTenantNormalSlots: 1},
		AutoMemory:  &conf.Runtime_AutoMemory{MaxRetries: 1, DrainBatchSize: 4},
	}
	w := buildRetryExhaustedWorker(t, rc, nil)
	q := w.queue

	req := memtrpc.AutoMemoryJobRequest{
		AppName: "agent-retry", SessionID: "sess-ack-cancel",
		Priority: memtrpc.MemoryJobPriorityNormal, EnqueuedAt: time.Now(),
	}
	q.Enqueue(req)
	got := <-q.Chan()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.handleDequeued(ctx, got)

	req2 := memtrpc.AutoMemoryJobRequest{
		AppName: "agent-retry", SessionID: "sess-ack-cancel-2",
		Priority: memtrpc.MemoryJobPriorityNormal, EnqueuedAt: time.Now(),
	}
	q.Enqueue(req2)
	select {
	case <-q.Chan():
	case <-time.After(2 * time.Second):
		t.Fatal("second job not delivered; tenant in-flight likely leaked")
	}
}

func TestHandleDequeued_AckDoneOnPanic(t *testing.T) {
	rc := &conf.Runtime{
		MemoryQueue: &conf.Runtime_MemoryQueue{MaxTenantNormalSlots: 1},
		AutoMemory:  &conf.Runtime_AutoMemory{MaxRetries: 1, DrainBatchSize: 4},
	}
	base := buildRetryExhaustedWorker(t, rc, nil)
	q := memtrpc.NewMemoryJobQueue(rc, 4, time.Millisecond, loggateway.NewNoop())
	t.Cleanup(q.Close)

	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:  rc,
		Interval:     0,
		Sessions:     base.sessions,
		Agents:       base.agents,
		Writer:       noopConsolidationWriter{},
		Consolidator: &panicConsolidator{},
		Queue:        q,
		Stats:        biz.NewMemoryWorkerStats(),
		Logger:       loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}

	req := memtrpc.AutoMemoryJobRequest{
		AppName: "agent-retry", SessionID: "sess-retry-exhaust",
		Priority: memtrpc.MemoryJobPriorityNormal, EnqueuedAt: time.Now(),
	}
	q.Enqueue(req)
	got := <-q.Chan()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic from consolidator")
			}
		}()
		w.handleDequeued(context.Background(), got)
	}()

	req2 := memtrpc.AutoMemoryJobRequest{
		AppName: "agent-retry", SessionID: "sess-ack-panic-2",
		Priority: memtrpc.MemoryJobPriorityNormal, EnqueuedAt: time.Now(),
	}
	q.Enqueue(req2)
	select {
	case <-q.Chan():
	case <-time.After(2 * time.Second):
		t.Fatal("second job not delivered after panic; in-flight likely leaked")
	}
}

type panicConsolidator struct{}

func (c *panicConsolidator) Extract(_ context.Context, _ biz.ConsolidateInput) ([]biz.MemoryProposal, error) {
	panic("auto_memory consolidator boom")
}
