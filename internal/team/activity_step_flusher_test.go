package team

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type memStepRepo struct {
	mu    sync.Mutex
	steps []biz.OrchestrationStep
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

func TestActivityStepFlusher_BatchFlush(t *testing.T) {
	t.Setenv("ARANEA_OBS_PERSIST", "1")
	repo := &memStepRepo{}
	flusher := NewActivityStepFlusher(repo, "run-1", "gex-1", loggateway.NewNoop())
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
