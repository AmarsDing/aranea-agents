package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/activityevent"
	rt "aranea-agents/internal/runtime"
)

func TestCancelRun_PublishesCancelledRunStatus(t *testing.T) {
	bus := activityevent.New(nil)
	ch, unsub := bus.Subscribe(biz.ActivityEventSubscribeOptions{BufferSize: 8, GlobalMode: true})
	defer unsub()

	reg := rt.NewRunRegistry()
	runID := "run-cancel-ws"
	reg.SetStatus("sess-ws", runID, "running", "")
	reg.StoreCancelable("sess-ws", runID, func() {})

	rStatus := newChatRunStatusTracker(reg, nil, bus, nil)
	svc := &ChatService{
		orch: &ChatOrchestrator{
			runs: reg,
			core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{ActivityBus: bus}}},
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
		if ev.Activity.Stage != "run_status" {
			t.Fatalf("stage=%s", ev.Activity.Stage)
		}
		if ev.Activity.Meta["status"] != "cancelled" {
			t.Fatalf("status=%v", ev.Activity.Meta["status"])
		}
	default:
		t.Fatal("expected run_status activity event")
	}
}
