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

func TestPersistStep_EmitsStartedAndFinished(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 8})
	defer unsub()

	runner := &Runner{
		runWriter: &stepBusRunWriter{},
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{Bus: bus},
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
	var started, finished bool
	for i := 0; i < 2; i++ {
		select {
		case env := <-ch:
			switch env.Type {
			case event.EnvelopeTypeTeamStepStarted:
				started = true
			case event.EnvelopeTypeTeamStepFinished:
				finished = true
			}
		default:
		}
	}
	if !started || !finished {
		t.Fatalf("started=%v finished=%v", started, finished)
	}
}
