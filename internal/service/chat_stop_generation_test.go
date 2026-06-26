package service

import (
	"context"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/activityevent"
	rt "aranea-agents/internal/runtime"
)

func TestStopGeneration_PublishesCancelledRunStatus(t *testing.T) {
	bus := activityevent.New(nil)
	ch, unsub := bus.Subscribe(biz.ActivityEventSubscribeOptions{BufferSize: 8, GlobalMode: true})
	defer unsub()

	reg := rt.NewRunRegistry()
	runID := "run-cancel-1"
	reg.SetStatus("sess-1", runID, "running", "")
	reg.StoreCancelable("sess-1", runID, func() {})

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

	resp, err := svc.StopGeneration(context.Background(), &chatv1.StopGenerationRequest{SessionId: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetStopped() {
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
