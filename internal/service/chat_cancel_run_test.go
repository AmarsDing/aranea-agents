package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

func TestCancelRun_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewV2Bus()
	ch, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	reg := rt.NewRunRegistry()
	runID := "run-cancel-ws"
	reg.SetStatus("sess-ws", runID, "running", "")
	reg.StoreCancelable("sess-ws", runID, func() {})

	rStatus := newChatRunStatusTracker(reg, nil, bus, nil)
	svc := &ChatService{
		orch: &ChatOrchestrator{
			runs: reg,
			core: chatTurnCoreDeps{TD: rt.TurnDeps{}},
			runMgr: &chatRunManagerImpl{
				runStatusTracker:    rStatus,
				pendingQueueManager: noopPendingQueueManager{},
				awaitCoordinator:    noopAwaitCoordinator{},
				sessionRunLifecycle: noopSessionRunLifecycle{},
			},
			turnLC: newNoopChatTurnLifecycle(),
		},
	}

	if !svc.CancelRun(context.Background(), "sess-ws") {
		t.Fatal("expected stopped=true")
	}

	select {
	case ev := <-ch:
		rse, ok := ev.(*biz.RunStatusEvent)
		if !ok {
			t.Fatalf("expected *RunStatusEvent, got %T", ev)
		}
		if rse.Status != biz.SessionRunPhaseCancelled {
			t.Fatalf("status=%s", rse.Status)
		}
		if rse.SpiritSessionID() != "sess-ws" {
			t.Fatalf("session=%s", rse.SpiritSessionID())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected run_status event")
	}
}
