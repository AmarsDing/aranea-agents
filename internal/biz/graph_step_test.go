package biz

import "testing"

func TestUpsertGraphStep(t *testing.T) {
	t.Run("append_new", func(t *testing.T) {
		steps := []GraphStepSnapshot{
			{NodeID: "n1", StepIndex: 0, Status: "completed"},
		}
		steps = upsertGraphStep(steps, GraphStepSnapshot{NodeID: "n2", StepIndex: 0, Status: "running"})
		if len(steps) != 2 {
			t.Fatalf("len=%d want 2", len(steps))
		}
		if steps[1].NodeID != "n2" {
			t.Fatalf("step1=%+v", steps[1])
		}
	})

	t.Run("update_existing", func(t *testing.T) {
		steps := []GraphStepSnapshot{
			{NodeID: "n1", StepIndex: 0, Status: "running"},
			{NodeID: "n2", StepIndex: 0, Status: "running"},
		}
		steps = upsertGraphStep(steps, GraphStepSnapshot{NodeID: "n1", StepIndex: 0, Status: "completed"})
		if len(steps) != 2 {
			t.Fatalf("len=%d want 2 (update in place)", len(steps))
		}
		if steps[0].Status != "completed" {
			t.Fatalf("step0 status=%q want completed", steps[0].Status)
		}
	})

	t.Run("same_node_different_step", func(t *testing.T) {
		steps := []GraphStepSnapshot{
			{NodeID: "n1", StepIndex: 0, Status: "completed"},
		}
		steps = upsertGraphStep(steps, GraphStepSnapshot{NodeID: "n1", StepIndex: 1, Status: "running"})
		if len(steps) != 2 {
			t.Fatalf("len=%d want 2 (different step index)", len(steps))
		}
	})

	t.Run("empty_steps", func(t *testing.T) {
		steps := upsertGraphStep(nil, GraphStepSnapshot{NodeID: "n1", StepIndex: 0, Status: "completed"})
		if len(steps) != 1 {
			t.Fatalf("len=%d want 1", len(steps))
		}
	})
}

func TestEvictIfNeeded(t *testing.T) {
	uc := &GraphUsecase{
		executions:       make(map[string]*GraphExecution),
		teamBuildConfigs: make(map[string]GraphBuildConfig),
	}

	for i := 0; i < maxExecutions; i++ {
		uc.executions[string(rune('a'+i%26))+string(rune('0'+i/26))] = &GraphExecution{
			ID:     string(rune('a' + i%26)),
			Status: "completed",
		}
	}
	if len(uc.executions) < maxExecutions {
		t.Fatalf("setup: expected at least %d executions, got %d", maxExecutions, len(uc.executions))
	}

	uc.evictIfNeeded()
	if len(uc.executions) >= maxExecutions {
		t.Fatalf("expected eviction, still have %d executions", len(uc.executions))
	}
}

func TestEvictIfNeeded_skipsRunning(t *testing.T) {
	uc := &GraphUsecase{
		executions:       make(map[string]*GraphExecution),
		teamBuildConfigs: make(map[string]GraphBuildConfig),
	}

	uc.executions["running-1"] = &GraphExecution{ID: "running-1", Status: "running"}
	uc.executions["waiting-1"] = &GraphExecution{ID: "waiting-1", Status: "waiting_human"}

	uc.evictIfNeeded()
	if _, ok := uc.executions["running-1"]; !ok {
		t.Fatal("running execution should not be evicted")
	}
	if _, ok := uc.executions["waiting-1"]; !ok {
		t.Fatal("waiting_human execution should not be evicted")
	}
}
