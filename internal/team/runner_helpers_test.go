package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

type stepBusRunWriter struct {
	steps []biz.TeamRunStep
}

func (r *stepBusRunWriter) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	r.steps = append(r.steps, step)
	return step, nil
}

func (r *stepBusRunWriter) CreateTeamRun(_ context.Context, run biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return run, nil
}
func (r *stepBusRunWriter) UpdateTeamRun(_ context.Context, _ biz.TeamRunRecord) error { return nil }
func (r *stepBusRunWriter) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *stepBusRunWriter) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (r *stepBusRunWriter) UpdateTeamRunTraceID(_ context.Context, _, _ string) error { return nil }
func (r *stepBusRunWriter) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error {
	return nil
}

func TestPersistStep_EmitsFinished(t *testing.T) {
	v2Bus := event.NewV2Bus()
	ch, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := &Runner{
		runWriter: &stepBusRunWriter{},
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{EventBus: v2Bus},
		},
	}
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a", DisplayName: "Worker A"}
	m := MemberDef{Role: "worker"}
	asst := biz.ChatMessage{Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK, CreatedAt: "2026-01-01T00:00:00Z"}

	runner.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 2)

	repo := runner.runWriter.(*stepBusRunWriter)
	if len(repo.steps) != 1 {
		t.Fatalf("steps=%d", len(repo.steps))
	}
	if repo.steps[0].ToolCallCount != 2 {
		t.Fatalf("tool_call_count=%d", repo.steps[0].ToolCallCount)
	}
	// persistStep now only emits "completed" (finished) event.
	// The "created" (started) event is emitted by PublishTeamStepStarted
	// at node_start time, or by ensureGraphRunStepsFallback for the
	// fallback path.
	var finished bool
	select {
	case e := <-ch:
		bridge, ok := e.(*biz.ActivityBridgeEvent)
		if !ok {
			t.Fatalf("expected *ActivityBridgeEvent, got %T", e)
		}
		ev := bridge.Event
		if ev.Activity.Kind != biz.ActivityKindSession {
			t.Fatalf("kind=%q want session", ev.Activity.Kind)
		}
		if ev.Activity.AgentName != "Worker A" {
			t.Fatalf("agent_name=%q want %q", ev.Activity.AgentName, "Worker A")
		}
		csid, _ := ev.Activity.Meta["child_session_id"].(string)
		if csid != "sess-1" {
			t.Fatalf("child_session_id=%q want %q", csid, "sess-1")
		}
		if ev.Event != biz.ActivityEventCompleted {
			t.Fatalf("event=%q want completed", ev.Event)
		}
		finished = true
	default:
	}
	if !finished {
		t.Fatal("expected completed event, got none")
	}
}
