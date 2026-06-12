package service

import (
	"context"
	"testing"

	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

func TestCancelRun_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 8})
	defer unsub()

	reg := rt.NewRunRegistry()
	runID := "run-cancel-ws"
	reg.SetStatus("sess-ws", runID, "running", "")
	reg.StoreCancelable("sess-ws", runID, func() {})

	rStatus := newChatRunStatusTracker(reg, nil, bus, nil)
	svc := &ChatService{
		orch: &ChatOrchestrator{
			runs: reg,
			core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{Bus: bus}}},
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
	case env := <-ch:
		if env.Type != event.EnvelopeTypeRunStatus {
			t.Fatalf("type=%s", env.Type)
		}
		if env.Metadata["status"] != "cancelled" {
			t.Fatalf("status=%v", env.Metadata["status"])
		}
	default:
		t.Fatal("expected run_status envelope")
	}
}
