package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type captureUpwardBus struct {
	mu     sync.Mutex
	events []biz.Event
}

func (b *captureUpwardBus) Publish(_ context.Context, e biz.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
}

func (b *captureUpwardBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func TestDagRunPublishUpwardDoesNotBlockDispatch(t *testing.T) {
	t.Parallel()
	bus := &captureUpwardBus{}
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	pe.SetEventBus(bus)
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	if biz.UpwardIsDispatchBarrier(biz.PipeUpwardHeartbeat) {
		t.Fatal("heartbeat is a barrier")
	}
	r.publishUpward(context.Background(), biz.PipeUpwardHeartbeat, "阶段已开工：设计", map[string]any{"step_id": "st1"})
	r.publishUpward(context.Background(), biz.PipeUpwardException, "阶段例外：后端 timeout", map[string]any{"step_id": "st2"})
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 2 {
		t.Fatalf("events=%d", len(bus.events))
	}
	for _, e := range bus.events {
		n, ok := e.(*biz.SystemNoticeEvent)
		if !ok || n.NoticeType != "orchestration_progress" {
			t.Fatalf("event=%T", e)
		}
		if n.Meta["dispatch_barrier"] != false {
			t.Fatal("upward must not block dispatch")
		}
	}
}
