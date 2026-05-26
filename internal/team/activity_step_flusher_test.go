package team

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

type memStepRepo struct {
	mu    sync.Mutex
	steps []biz.OrchestrationStep
}

func (m *memStepRepo) ListTeams(context.Context) ([]biz.Team, error) { return nil, nil }
func (m *memStepRepo) GetTeamByID(context.Context, string) (biz.Team, error) {
	return biz.Team{}, nil
}
func (m *memStepRepo) CreateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (m *memStepRepo) UpdateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (m *memStepRepo) DeleteTeam(context.Context, string) error { return nil }
func (m *memStepRepo) ListTeamRuns(context.Context, string, int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (m *memStepRepo) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (m *memStepRepo) GetTeamRunByID(context.Context, string) (biz.TeamRun, error) {
	return biz.TeamRun{}, nil
}
func (m *memStepRepo) ListTeamRunSteps(context.Context, string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (m *memStepRepo) CreateTeamRun(context.Context, biz.TeamRun) (biz.TeamRun, error) {
	return biz.TeamRun{}, nil
}
func (m *memStepRepo) UpdateTeamRun(context.Context, biz.TeamRun) error { return nil }
func (m *memStepRepo) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (m *memStepRepo) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (m *memStepRepo) UpdateTeamRunSummaryJSON(context.Context, string, string) error { return nil }
func (m *memStepRepo) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}
func (m *memStepRepo) BatchCreateOrchestrationSteps(_ context.Context, steps []biz.OrchestrationStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steps = append(m.steps, steps...)
	return nil
}
func (m *memStepRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (m *memStepRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (m *memStepRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (m *memStepRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}

func TestActivityStepFlusher_BatchFlush(t *testing.T) {
	t.Setenv("ARANEA_OBS_PERSIST", "1")
	repo := &memStepRepo{}
	flusher := NewActivityStepFlusher(repo, "run-1", "gex-1")
	if flusher == nil {
		t.Fatal("expected flusher")
	}
	for i := 0; i < 12; i++ {
		flusher.Enqueue("member-1", biz.ActivitySnapshot{
			Kind:       "tool",
			ToolName:   "read_file",
			Status:     "success",
			StartedAt:  "2026-05-23T00:00:00Z",
			FinishedAt: "2026-05-23T00:00:01Z",
		})
	}
	flusher.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for {
		repo.mu.Lock()
		n := len(repo.steps)
		repo.mu.Unlock()
		if n >= 12 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected >=12 steps, got %d", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
