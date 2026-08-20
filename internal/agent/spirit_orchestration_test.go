package agent

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/graph"
)

// ---------------------------------------------------------------------------
// Mock repos for checkpoint restore tests
// ---------------------------------------------------------------------------

type memOrchestrationRepo struct {
	items map[string]*biz.OrchestrationHandle
}

func newMemOrchestrationRepo() *memOrchestrationRepo {
	return &memOrchestrationRepo{items: make(map[string]*biz.OrchestrationHandle)}
}

func (m *memOrchestrationRepo) Create(_ context.Context, h *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	if h.ID == "" {
		h.ID = fmt.Sprintf("orch_%d", len(m.items)+1)
	}
	m.items[h.ID] = h
	return h, nil
}
func (m *memOrchestrationRepo) GetByID(_ context.Context, id string) (*biz.OrchestrationHandle, error) {
	h, ok := m.items[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return h, nil
}
func (m *memOrchestrationRepo) Update(_ context.Context, h *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	m.items[h.ID] = h
	return h, nil
}
func (m *memOrchestrationRepo) ListBySpiritSessionID(_ context.Context, spiritSessionID string) ([]*biz.OrchestrationHandle, error) {
	var out []*biz.OrchestrationHandle
	for _, h := range m.items {
		if h.SpiritSessionID == spiritSessionID {
			out = append(out, h)
		}
	}
	return out, nil
}
func (m *memOrchestrationRepo) ListByStatus(_ context.Context, status biz.OrchestrationStatus) ([]*biz.OrchestrationHandle, error) {
	var out []*biz.OrchestrationHandle
	for _, h := range m.items {
		if h.Status == status {
			out = append(out, h)
		}
	}
	return out, nil
}

// memCheckpointSaver is an in-memory CheckpointSaver for testing.
type memCheckpointSaver struct {
	tuples map[string]*graph.CheckpointTuple // keyed by lineageID
}

func newMemCheckpointSaver() *memCheckpointSaver {
	return &memCheckpointSaver{tuples: make(map[string]*graph.CheckpointTuple)}
}

func (m *memCheckpointSaver) Get(_ context.Context, config map[string]any) (*graph.Checkpoint, error) {
	lineageID := graph.GetLineageID(config)
	t, ok := m.tuples[lineageID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t.Checkpoint, nil
}
func (m *memCheckpointSaver) GetTuple(_ context.Context, config map[string]any) (*graph.CheckpointTuple, error) {
	lineageID := graph.GetLineageID(config)
	checkpointID := graph.GetCheckpointID(config)
	// If checkpoint ID specified, look for exact match
	if checkpointID != "" {
		for k, t := range m.tuples {
			if t.Checkpoint != nil && t.Checkpoint.ID == checkpointID {
				_ = k
				return t, nil
			}
		}
		return nil, fmt.Errorf("not found")
	}
	t, ok := m.tuples[lineageID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t, nil
}
func (m *memCheckpointSaver) List(_ context.Context, config map[string]any, _ *graph.CheckpointFilter) ([]*graph.CheckpointTuple, error) {
	lineageID := graph.GetLineageID(config)
	t, ok := m.tuples[lineageID]
	if !ok {
		return nil, nil
	}
	return []*graph.CheckpointTuple{t}, nil
}
func (m *memCheckpointSaver) Put(_ context.Context, req graph.PutRequest) (map[string]any, error) {
	lineageID := graph.GetLineageID(req.Config)
	m.tuples[lineageID] = &graph.CheckpointTuple{
		Config:     req.Config,
		Checkpoint: req.Checkpoint,
		Metadata:   req.Metadata,
	}
	return req.Config, nil
}
func (m *memCheckpointSaver) PutWrites(_ context.Context, _ graph.PutWritesRequest) error {
	return nil
}
func (m *memCheckpointSaver) PutFull(_ context.Context, req graph.PutFullRequest) (map[string]any, error) {
	lineageID := graph.GetLineageID(req.Config)
	m.tuples[lineageID] = &graph.CheckpointTuple{
		Config:     req.Config,
		Checkpoint: req.Checkpoint,
		Metadata:   req.Metadata,
	}
	return req.Config, nil
}
func (m *memCheckpointSaver) DeleteLineage(_ context.Context, lineageID string) error {
	delete(m.tuples, lineageID)
	return nil
}
func (m *memCheckpointSaver) Close() error { return nil }

// ---------------------------------------------------------------------------
// TestCheckpointRestore: Task 1.2
// ---------------------------------------------------------------------------

func TestCheckpointRestore_RebuildsGraphAgentState(t *testing.T) {
	repo := newMemOrchestrationRepo()
	ckptSaver := newMemCheckpointSaver()
	lg := loggateway.NewNoop()

	orch := &TaskOrchestratorImpl{
		controller:      nil,
		repo:            repo,
		synthesis:       nil,
		checkpointSaver: ckptSaver,
		orchCache:       nil,
		perfRepo:        nil,
		eventBus:        nil,
		lg:              lg,
	}

	ctx := context.Background()

	// Create an interrupted orchestration handle
	handle := &biz.OrchestrationHandle{
		ID:              "orch_test_001",
		TaskPlanID:      "tp_001",
		AllocationID:    "ap_001",
		SpiritSessionID: "spirit_sess_001",
		Strategy:        biz.StrategyDAG,
		TeamIDs:         []string{"team_001"},
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "ckpt_001",
	}
	repo.Create(ctx, handle)

	// Save a checkpoint with matching critical state fields
	lineageID := handle.ID
	channelValues := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": handle.SpiritSessionID,
		"strategy":          string(handle.Strategy),
		"status":            string(handle.Status),
	}
	ckpt := graph.NewCheckpoint(channelValues, nil, nil)
	ckpt.ID = "ckpt_001"
	metadata := graph.NewCheckpointMetadata(graph.CheckpointSourceInput, -1)
	req := graph.PutRequest{
		Config:     graph.CreateCheckpointConfig(lineageID, "ckpt_001", ""),
		Checkpoint: ckpt,
		Metadata:   metadata,
	}
	ckptSaver.Put(ctx, req)

	// Recover the orchestration
	err := orch.Recover(ctx, handle.ID)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	// Verify handle is now running
	recovered, _ := repo.GetByID(ctx, handle.ID)
	if recovered.Status != biz.OrchestrationStatusRunning {
		t.Errorf("expected status running, got %q", recovered.Status)
	}

	// Verify checkpoint ID was updated
	if recovered.CheckpointID == "" {
		t.Error("checkpoint ID should not be empty after recovery")
	}

	// Verify strategy was preserved
	if recovered.Strategy != biz.StrategyDAG {
		t.Errorf("expected strategy dag, got %q", recovered.Strategy)
	}
}

func TestCheckpointRestore_ValidatesCriticalFields(t *testing.T) {
	repo := newMemOrchestrationRepo()
	ckptSaver := newMemCheckpointSaver()
	lg := loggateway.NewNoop()

	orch := &TaskOrchestratorImpl{
		repo:            repo,
		controller:      nil,
		checkpointSaver: ckptSaver,
		lg:              lg,
	}

	ctx := context.Background()

	// Create an interrupted orchestration handle
	handle := &biz.OrchestrationHandle{
		ID:              "orch_test_002",
		SpiritSessionID: "spirit_sess_002",
		Strategy:        biz.StrategyCoordinator,
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "ckpt_002",
	}
	repo.Create(ctx, handle)

	// Save a checkpoint with MISMATCHED orchestration_id
	lineageID := handle.ID
	channelValues := map[string]any{
		"orchestration_id":  "different_orch_id",
		"spirit_session_id": handle.SpiritSessionID,
		"strategy":          string(handle.Strategy),
	}
	ckpt := graph.NewCheckpoint(channelValues, nil, nil)
	ckpt.ID = "ckpt_002"
	metadata := graph.NewCheckpointMetadata(graph.CheckpointSourceInput, -1)
	req := graph.PutRequest{
		Config:     graph.CreateCheckpointConfig(lineageID, "ckpt_002", ""),
		Checkpoint: ckpt,
		Metadata:   metadata,
	}
	ckptSaver.Put(ctx, req)

	// Recovery should still succeed (mismatch is logged as warning, not error)
	err := orch.Recover(ctx, handle.ID)
	if err != nil {
		t.Fatalf("Recover should succeed even with mismatched fields: %v", err)
	}

	recovered, _ := repo.GetByID(ctx, handle.ID)
	if recovered.Status != biz.OrchestrationStatusRunning {
		t.Errorf("expected status running, got %q", recovered.Status)
	}
}

func TestCheckpointRestore_RestoresStrategyFromCheckpoint(t *testing.T) {
	repo := newMemOrchestrationRepo()
	ckptSaver := newMemCheckpointSaver()
	lg := loggateway.NewNoop()

	orch := &TaskOrchestratorImpl{
		repo:            repo,
		controller:      nil,
		checkpointSaver: ckptSaver,
		lg:              lg,
	}

	ctx := context.Background()

	// Create an interrupted orchestration handle with empty strategy
	handle := &biz.OrchestrationHandle{
		ID:              "orch_test_003",
		SpiritSessionID: "spirit_sess_003",
		Strategy:        "", // empty — should be restored from checkpoint
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "ckpt_003",
	}
	repo.Create(ctx, handle)

	// Save a checkpoint with strategy
	lineageID := handle.ID
	channelValues := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": handle.SpiritSessionID,
		"strategy":          "parallel",
	}
	ckpt := graph.NewCheckpoint(channelValues, nil, nil)
	ckpt.ID = "ckpt_003"
	metadata := graph.NewCheckpointMetadata(graph.CheckpointSourceInput, -1)
	req := graph.PutRequest{
		Config:     graph.CreateCheckpointConfig(lineageID, "ckpt_003", ""),
		Checkpoint: ckpt,
		Metadata:   metadata,
	}
	ckptSaver.Put(ctx, req)

	err := orch.Recover(ctx, handle.ID)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	recovered, _ := repo.GetByID(ctx, handle.ID)
	if recovered.Strategy != biz.StrategyParallel {
		t.Errorf("expected strategy to be restored as parallel, got %q", recovered.Strategy)
	}
}

func TestCheckpointRestore_NoCheckpointAvailable(t *testing.T) {
	repo := newMemOrchestrationRepo()
	lg := loggateway.NewNoop()

	orch := &TaskOrchestratorImpl{
		repo:            repo,
		controller:      nil,
		checkpointSaver: nil, // no checkpoint saver
		lg:              lg,
	}

	ctx := context.Background()

	// Create an interrupted orchestration handle without checkpoint ID
	handle := &biz.OrchestrationHandle{
		ID:              "orch_test_004",
		SpiritSessionID: "spirit_sess_004",
		Strategy:        biz.StrategyDirect,
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "", // no checkpoint
	}
	repo.Create(ctx, handle)

	// Recovery should fail because no checkpoint is available
	err := orch.Recover(ctx, handle.ID)
	if err == nil {
		t.Error("expected error when no checkpoint available")
	}

	// Handle should be marked as failed
	recovered, _ := repo.GetByID(ctx, handle.ID)
	if recovered.Status != biz.OrchestrationStatusFailed {
		t.Errorf("expected status failed, got %q", recovered.Status)
	}
}
