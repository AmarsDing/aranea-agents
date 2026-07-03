package service

import (
	"context"
	"testing"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

func TestStopGeneration_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewV2Bus()
	ch, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	reg := rt.NewRunRegistry()
	runID := "run-cancel-1"
	reg.SetStatus("sess-1", runID, "running", "")
	reg.StoreCancelable("sess-1", runID, func() {})

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

	resp, err := svc.StopGeneration(context.Background(), &chatv1.StopGenerationRequest{SessionId: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetStopped() {
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
		if rse.SpiritSessionID() != "sess-1" {
			t.Fatalf("session=%s", rse.SpiritSessionID())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected run_status event")
	}
}
