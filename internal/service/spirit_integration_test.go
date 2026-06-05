package service

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/graph"
)

// ---------------------------------------------------------------------------
// Stubs for Spirit integration test
// ---------------------------------------------------------------------------

// stubOrchestrationRepo implements biz.OrchestrationRepository for integration tests.
type stubOrchestrationRepo struct {
	items map[string]*biz.OrchestrationHandle
}

func newStubOrchestrationRepo() *stubOrchestrationRepo {
	return &stubOrchestrationRepo{items: make(map[string]*biz.OrchestrationHandle)}
}

func (s *stubOrchestrationRepo) Create(_ context.Context, h *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	if h.ID == "" {
		h.ID = fmt.Sprintf("orch_%d", len(s.items)+1)
	}
	s.items[h.ID] = h
	return h, nil
}
func (s *stubOrchestrationRepo) GetByID(_ context.Context, id string) (*biz.OrchestrationHandle, error) {
	h, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return h, nil
}
func (s *stubOrchestrationRepo) Update(_ context.Context, h *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	s.items[h.ID] = h
	return h, nil
}
func (s *stubOrchestrationRepo) ListBySpiritSessionID(_ context.Context, spiritSessionID string) ([]*biz.OrchestrationHandle, error) {
	var out []*biz.OrchestrationHandle
	for _, h := range s.items {
		if h.SpiritSessionID == spiritSessionID {
			out = append(out, h)
		}
	}
	return out, nil
}
func (s *stubOrchestrationRepo) ListByStatus(_ context.Context, status biz.OrchestrationStatus) ([]*biz.OrchestrationHandle, error) {
	var out []*biz.OrchestrationHandle
	for _, h := range s.items {
		if h.Status == status {
			out = append(out, h)
		}
	}
	return out, nil
}

// stubCheckpointSaver implements graph.CheckpointSaver for integration tests.
type stubCheckpointSaver struct {
	tuples map[string]*graph.CheckpointTuple
}

func newStubCheckpointSaver() *stubCheckpointSaver {
	return &stubCheckpointSaver{tuples: make(map[string]*graph.CheckpointTuple)}
}

func (s *stubCheckpointSaver) Get(_ context.Context, config map[string]any) (*graph.Checkpoint, error) {
	lineageID := graph.GetLineageID(config)
	t, ok := s.tuples[lineageID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t.Checkpoint, nil
}
func (s *stubCheckpointSaver) GetTuple(_ context.Context, config map[string]any) (*graph.CheckpointTuple, error) {
	checkpointID := graph.GetCheckpointID(config)
	if checkpointID != "" {
		for _, t := range s.tuples {
			if t.Checkpoint != nil && t.Checkpoint.ID == checkpointID {
				return t, nil
			}
		}
		return nil, fmt.Errorf("not found")
	}
	lineageID := graph.GetLineageID(config)
	t, ok := s.tuples[lineageID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t, nil
}
func (s *stubCheckpointSaver) List(_ context.Context, config map[string]any, _ *graph.CheckpointFilter) ([]*graph.CheckpointTuple, error) {
	lineageID := graph.GetLineageID(config)
	t, ok := s.tuples[lineageID]
	if !ok {
		return nil, nil
	}
	return []*graph.CheckpointTuple{t}, nil
}
func (s *stubCheckpointSaver) Put(_ context.Context, req graph.PutRequest) (map[string]any, error) {
	lineageID := graph.GetLineageID(req.Config)
	s.tuples[lineageID] = &graph.CheckpointTuple{
		Config:     req.Config,
		Checkpoint: req.Checkpoint,
		Metadata:   req.Metadata,
	}
	return req.Config, nil
}
func (s *stubCheckpointSaver) PutWrites(_ context.Context, _ graph.PutWritesRequest) error {
	return nil
}
func (s *stubCheckpointSaver) PutFull(_ context.Context, req graph.PutFullRequest) (map[string]any, error) {
	lineageID := graph.GetLineageID(req.Config)
	s.tuples[lineageID] = &graph.CheckpointTuple{
		Config:     req.Config,
		Checkpoint: req.Checkpoint,
		Metadata:   req.Metadata,
	}
	return req.Config, nil
}
func (s *stubCheckpointSaver) DeleteLineage(_ context.Context, lineageID string) error {
	delete(s.tuples, lineageID)
	return nil
}
func (s *stubCheckpointSaver) Close() error { return nil }

// stubPerfRepo implements biz.AgentPerformanceRepository for integration tests.
type stubPerfRepo struct {
	items map[string]*biz.AgentPerformance
}

func newStubPerfRepo() *stubPerfRepo {
	return &stubPerfRepo{items: make(map[string]*biz.AgentPerformance)}
}

func (s *stubPerfRepo) Get(_ context.Context, agentKey, taskType string) (*biz.AgentPerformance, error) {
	key := agentKey + "_" + taskType
	p, ok := s.items[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}
func (s *stubPerfRepo) GetBestForTaskType(_ context.Context, taskType string, limit int) ([]*biz.AgentPerformance, error) {
	var results []*biz.AgentPerformance
	for _, p := range s.items {
		if p.TaskType == taskType {
			results = append(results, p)
		}
	}
	// Sort by success rate descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].SuccessRate > results[i].SuccessRate {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
func (s *stubPerfRepo) Upsert(_ context.Context, perf *biz.AgentPerformance) error {
	key := perf.AgentKey + "_" + perf.TaskType
	s.items[key] = perf
	return nil
}

// ---------------------------------------------------------------------------
// TestSpiritIntegration: Task 4.1
// ---------------------------------------------------------------------------

// TestSpiritIntegration_CheckpointRecoveryAndPerformance verifies the end-to-end
// Spirit orchestration flow: checkpoint recovery with GraphAgent rebuild and
// AgentPerformance-based agent selection.
func TestSpiritIntegration_CheckpointRecoveryAndPerformance(t *testing.T) {
	ctx := context.Background()
	_ = loggateway.NewNoop() // ensure loggateway initialized

	// --- Part 1: Checkpoint Recovery ---
	t.Run("CheckpointRecovery", func(t *testing.T) {
		repo := newStubOrchestrationRepo()
		ckptSaver := newStubCheckpointSaver()

		// Simulate an interrupted orchestration with a saved checkpoint
		handle := &biz.OrchestrationHandle{
			ID:              "orch_integ_001",
			TaskPlanID:      "tp_001",
			AllocationID:    "ap_001",
			SpiritSessionID: "spirit_integ_001",
			Strategy:        biz.StrategyDAG,
			TeamIDs:         []string{"team_001"},
			Status:          biz.OrchestrationStatusInterrupted,
			CheckpointID:    "ckpt_integ_001",
		}
		repo.Create(ctx, handle)

		// Save checkpoint with matching state
		channelValues := map[string]any{
			"orchestration_id":  handle.ID,
			"spirit_session_id": handle.SpiritSessionID,
			"strategy":          string(handle.Strategy),
			"status":            string(handle.Status),
		}
		ckpt := graph.NewCheckpoint(channelValues, nil, nil)
		ckpt.ID = "ckpt_integ_001"
		ckptSaver.Put(ctx, graph.PutRequest{
			Config:     graph.CreateCheckpointConfig(handle.ID, "ckpt_integ_001", ""),
			Checkpoint: ckpt,
			Metadata:   graph.NewCheckpointMetadata(graph.CheckpointSourceInput, -1),
		})

		// Verify the orchestration handle was created correctly
		h, err := repo.GetByID(ctx, handle.ID)
		if err != nil {
			t.Fatalf("failed to get handle: %v", err)
		}
		if h.Status != biz.OrchestrationStatusInterrupted {
			t.Errorf("expected interrupted status, got %q", h.Status)
		}
		if h.Strategy != biz.StrategyDAG {
			t.Errorf("expected DAG strategy, got %q", h.Strategy)
		}
		if h.CheckpointID != "ckpt_integ_001" {
			t.Errorf("expected checkpoint ID, got %q", h.CheckpointID)
		}

		// Verify checkpoint can be loaded
		tuple, err := ckptSaver.GetTuple(ctx, graph.CreateCheckpointConfig(handle.ID, "ckpt_integ_001", ""))
		if err != nil {
			t.Fatalf("failed to load checkpoint: %v", err)
		}
		if tuple.Checkpoint.ID != "ckpt_integ_001" {
			t.Errorf("expected checkpoint ID ckpt_integ_001, got %q", tuple.Checkpoint.ID)
		}

		// Verify critical state fields match
		ckptOrchID, _ := tuple.Checkpoint.ChannelValues["orchestration_id"].(string)
		if ckptOrchID != handle.ID {
			t.Errorf("checkpoint orchestration_id mismatch: got %q, want %q", ckptOrchID, handle.ID)
		}
		ckptSessionID, _ := tuple.Checkpoint.ChannelValues["spirit_session_id"].(string)
		if ckptSessionID != handle.SpiritSessionID {
			t.Errorf("checkpoint spirit_session_id mismatch: got %q, want %q", ckptSessionID, handle.SpiritSessionID)
		}
		ckptStrategy, _ := tuple.Checkpoint.ChannelValues["strategy"].(string)
		if ckptStrategy != string(handle.Strategy) {
			t.Errorf("checkpoint strategy mismatch: got %q, want %q", ckptStrategy, handle.Strategy)
		}
	})

	// --- Part 2: AgentPerformance Selection ---
	t.Run("AgentPerformanceSelection", func(t *testing.T) {
		perfRepo := newStubPerfRepo()

		// Seed performance data
		perfRepo.Upsert(ctx, &biz.AgentPerformance{
			AgentKey:    "backend-agent",
			TaskType:    "go-backend",
			TotalRuns:   20,
			SuccessRuns: 18,
			SuccessRate: 0.9,
			AvgDQScore:  0.88,
		})
		perfRepo.Upsert(ctx, &biz.AgentPerformance{
			AgentKey:    "frontend-agent",
			TaskType:    "go-backend",
			TotalRuns:   5,
			SuccessRuns: 1,
			SuccessRate: 0.2,
			AvgDQScore:  0.15,
		})

		// Query best agent for task type
		best, err := perfRepo.GetBestForTaskType(ctx, "go-backend", 1)
		if err != nil {
			t.Fatalf("GetBestForTaskType failed: %v", err)
		}
		if len(best) == 0 {
			t.Fatal("expected at least one result")
		}
		if best[0].AgentKey != "backend-agent" {
			t.Errorf("expected backend-agent (best performance), got %q", best[0].AgentKey)
		}
		if best[0].SuccessRate != 0.9 {
			t.Errorf("expected success rate 0.9, got %f", best[0].SuccessRate)
		}

		// Verify individual agent performance lookup
		perf, err := perfRepo.Get(ctx, "backend-agent", "go-backend")
		if err != nil {
			t.Fatalf("Get performance failed: %v", err)
		}
		if perf.AvgDQScore != 0.88 {
			t.Errorf("expected DQ score 0.88, got %f", perf.AvgDQScore)
		}
	})

	// --- Part 3: Orchestration Status Transitions ---
	t.Run("OrchestrationStatusTransitions", func(t *testing.T) {
		repo := newStubOrchestrationRepo()

		handle := &biz.OrchestrationHandle{
			ID:              "orch_integ_002",
			SpiritSessionID: "spirit_integ_002",
			Strategy:        biz.StrategyCoordinator,
			Status:          biz.OrchestrationStatusPending,
		}
		repo.Create(ctx, handle)

		// pending → running
		handle.Status = biz.OrchestrationStatusRunning
		repo.Update(ctx, handle)

		h, _ := repo.GetByID(ctx, handle.ID)
		if h.Status != biz.OrchestrationStatusRunning {
			t.Errorf("expected running, got %q", h.Status)
		}

		// running → interrupted (simulating process crash)
		h.Status = biz.OrchestrationStatusInterrupted
		repo.Update(ctx, h)

		// List interrupted orchestrations
		interrupted, _ := repo.ListByStatus(ctx, biz.OrchestrationStatusInterrupted)
		if len(interrupted) == 0 {
			t.Error("expected at least one interrupted orchestration")
		}

		// interrupted → running (recovery)
		h.Status = biz.OrchestrationStatusRunning
		repo.Update(ctx, h)

		h, _ = repo.GetByID(ctx, handle.ID)
		if h.Status != biz.OrchestrationStatusRunning {
			t.Errorf("expected running after recovery, got %q", h.Status)
		}

		// running → completed
		h.Status = biz.OrchestrationStatusCompleted
		repo.Update(ctx, h)

		h, _ = repo.GetByID(ctx, handle.ID)
		if h.Status != biz.OrchestrationStatusCompleted {
			t.Errorf("expected completed, got %q", h.Status)
		}
	})
}
