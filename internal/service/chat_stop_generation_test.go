package service

import (
	"context"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

func TestStopGeneration_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 8})
	defer unsub()

	reg := rt.NewRunRegistry()
	runID := "run-cancel-1"
	reg.SetStatus("sess-1", runID, "running", "")
	reg.StoreCancelable("sess-1", runID, func() {})

	svc := &ChatService{
		orch: &ChatOrchestrator{
			runs: reg,
			td: rt.TurnDeps{
				Pipeline: rt.EventPipeline{Bus: bus},
			},
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
