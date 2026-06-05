package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

type stepBusRepo struct {
	biz.TeamRepository
	steps []biz.TeamRunStep
}

func (r *stepBusRepo) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	r.steps = append(r.steps, step)
	return step, nil
}

func (r *stepBusRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }
func (r *stepBusRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error { return nil }
func (r *stepBusRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error             { return nil }
func (r *stepBusRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (r *stepBusRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (r *stepBusRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (r *stepBusRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *stepBusRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (r *stepBusRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *stepBusRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

func TestPersistStep_EmitsStartedAndFinished(t *testing.T) {
	bus := event.NewBus()
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 8})
	defer unsub()

	runner := &Runner{
		teams: &stepBusRepo{},
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{Bus: bus},
		},
	}
	run := biz.TeamRun{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a", DisplayName: "Worker A"}
	m := MemberDef{Role: "worker"}
	asst := biz.ChatMessage{Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK, CreatedAt: "2026-01-01T00:00:00Z"}

	runner.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 2)

	repo := runner.teams.(*stepBusRepo)
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
