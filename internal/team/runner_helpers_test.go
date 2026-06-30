package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/activityevent"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

type stepBusRunWriter struct {
	steps []biz.TeamRunStep
}

func (r *stepBusRunWriter) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	r.steps = append(r.steps, step)
	return step, nil
}

func (r *stepBusRunWriter) CreateTeamRun(_ context.Context, run biz.TeamRun) (biz.TeamRun, error) {
	return run, nil
}
func (r *stepBusRunWriter) UpdateTeamRun(_ context.Context, _ biz.TeamRun) error { return nil }
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
	bus := activityevent.New(nil, loggateway.NewNoop())
	ch, unsub := bus.Subscribe(biz.ActivityEventSubscribeOptions{BufferSize: 8, GlobalMode: true})
	defer unsub()

	runner := &Runner{
		runWriter: &stepBusRunWriter{},
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{ActivityBus: bus},
		},
	}
	run := biz.TeamRun{ID: "run-1", SessionID: "sess-1"}
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
	case ev := <-ch:
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
