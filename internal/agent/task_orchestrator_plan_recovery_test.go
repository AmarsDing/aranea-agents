package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/graph"
)

type memTaskPlanRepo struct {
	items   map[string]*biz.TaskPlan
	listErr error
	getErr  error
}

func newMemTaskPlanRepo(plans ...*biz.TaskPlan) *memTaskPlanRepo {
	m := &memTaskPlanRepo{items: make(map[string]*biz.TaskPlan)}
	for _, p := range plans {
		if p != nil {
			cp := *p
			m.items[p.ID] = &cp
		}
	}
	return m
}

func (m *memTaskPlanRepo) Create(_ context.Context, p *biz.TaskPlan) (*biz.TaskPlan, error) {
	if m.items == nil {
		m.items = make(map[string]*biz.TaskPlan)
	}
	cp := *p
	m.items[p.ID] = &cp
	return p, nil
}
func (m *memTaskPlanRepo) GetByID(_ context.Context, id string) (*biz.TaskPlan, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	p, ok := m.items[id]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainData, "not found")
	}
	return p, nil
}
func (m *memTaskPlanRepo) Update(_ context.Context, p *biz.TaskPlan) (*biz.TaskPlan, error) {
	cp := *p
	m.items[p.ID] = &cp
	return p, nil
}
func (m *memTaskPlanRepo) ListBySpiritSessionID(_ context.Context, sessionID string) ([]*biz.TaskPlan, error) {
	var out []*biz.TaskPlan
	for _, p := range m.items {
		if p.SpiritSessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (m *memTaskPlanRepo) ListByStatuses(_ context.Context, statuses []biz.TaskPlanStatus) ([]*biz.TaskPlan, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	want := make(map[biz.TaskPlanStatus]bool, len(statuses))
	for _, s := range statuses {
		want[s] = true
	}
	var out []*biz.TaskPlan
	for _, p := range m.items {
		if want[p.Status] {
			out = append(out, p)
		}
	}
	return out, nil
}

type memAllocPlanRepo struct {
	items map[string]*biz.AllocationPlan
}

func newMemAllocPlanRepo(plans ...*biz.AllocationPlan) *memAllocPlanRepo {
	m := &memAllocPlanRepo{items: make(map[string]*biz.AllocationPlan)}
	for _, p := range plans {
		if p != nil {
			cp := *p
			m.items[p.ID] = &cp
		}
	}
	return m
}

func (m *memAllocPlanRepo) Create(_ context.Context, p *biz.AllocationPlan) (*biz.AllocationPlan, error) {
	cp := *p
	m.items[p.ID] = &cp
	return p, nil
}
func (m *memAllocPlanRepo) GetByID(_ context.Context, id string) (*biz.AllocationPlan, error) {
	p, ok := m.items[id]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainData, "not found")
	}
	return p, nil
}
func (m *memAllocPlanRepo) Update(_ context.Context, p *biz.AllocationPlan) (*biz.AllocationPlan, error) {
	cp := *p
	m.items[p.ID] = &cp
	return p, nil
}
func (m *memAllocPlanRepo) ListBySpiritSessionID(_ context.Context, sessionID string) ([]*biz.AllocationPlan, error) {
	var out []*biz.AllocationPlan
	for _, p := range m.items {
		if p.SpiritSessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func putCheckpoint(t *testing.T, saver *memCheckpointSaver, handle *biz.OrchestrationHandle) {
	t.Helper()
	channelValues := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": handle.SpiritSessionID,
		"strategy":          string(handle.Strategy),
	}
	ckpt := graph.NewCheckpoint(channelValues, nil, nil)
	ckpt.ID = handle.CheckpointID
	_, err := saver.Put(context.Background(), graph.PutRequest{
		Config:     graph.CreateCheckpointConfig(handle.ID, handle.CheckpointID, ""),
		Checkpoint: ckpt,
		Metadata:   graph.NewCheckpointMetadata(graph.CheckpointSourceInput, -1),
	})
	if err != nil {
		t.Fatalf("Put checkpoint: %v", err)
	}
}

func newRecoveryOrchestrator(repo biz.OrchestrationRepository, plans biz.TaskPlanRepository, allocs biz.AllocationPlanRepository, saver graph.CheckpointSaver) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		repo:            repo,
		plans:           newPlanStore(plans, allocs),
		checkpointSaver: saver,
		lg:              loggateway.NewNoop(),
	}
}

func TestRecover_RestoresDraftTaskPlan(t *testing.T) {
	ctx := context.Background()
	repo := newMemOrchestrationRepo()
	saver := newMemCheckpointSaver()
	draft := &biz.TaskPlan{
		ID:              "tp_draft_1",
		SpiritSessionID: "spirit_sess_1",
		UserMessage:     "写一份巡检报告",
		Strategy:        biz.StrategyDAG,
		Status:          biz.TaskPlanStatusDraft,
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "收集日志"}, {ID: "st_2", Name: "汇总"}},
	}
	alloc := &biz.AllocationPlan{
		ID:              "ap_1",
		TaskPlanID:      draft.ID,
		SpiritSessionID: draft.SpiritSessionID,
		Allocations:     []biz.TaskAllocation{{SubTaskID: "st_1", AssignedKey: "agent_a"}},
	}
	orch := newRecoveryOrchestrator(repo, newMemTaskPlanRepo(draft), newMemAllocPlanRepo(alloc), saver)

	handle := &biz.OrchestrationHandle{
		ID:              "orch_p110_1",
		TaskPlanID:      draft.ID,
		AllocationID:    alloc.ID,
		SpiritSessionID: draft.SpiritSessionID,
		Strategy:        biz.StrategyDAG,
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "ckpt_p110_1",
	}
	repo.Create(ctx, handle)
	putCheckpoint(t, saver, handle)

	if err := orch.Recover(ctx, handle.ID); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	plan, gotAlloc, ok := orch.PeekRecoveredPlan(draft.SpiritSessionID)
	if !ok || plan == nil {
		t.Fatal("expected recovered TaskPlan to be present")
	}
	if plan.ID != draft.ID || len(plan.SubTasks) != 2 {
		t.Fatalf("recovered plan = %+v", plan)
	}
	if gotAlloc == nil || gotAlloc.ID != alloc.ID {
		t.Fatalf("expected AllocationPlan restored, got %+v", gotAlloc)
	}
}

func TestRecover_NoDraftPlan_NotPretendRestored(t *testing.T) {
	ctx := context.Background()
	repo := newMemOrchestrationRepo()
	saver := newMemCheckpointSaver()
	orch := newRecoveryOrchestrator(repo, newMemTaskPlanRepo(), newMemAllocPlanRepo(), saver)

	handle := &biz.OrchestrationHandle{
		ID:              "orch_p110_nodraft",
		TaskPlanID:      "tp_missing",
		SpiritSessionID: "spirit_sess_nodraft",
		Strategy:        biz.StrategyDAG,
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "ckpt_nodraft",
	}
	repo.Create(ctx, handle)
	putCheckpoint(t, saver, handle)

	if err := orch.Recover(ctx, handle.ID); err != nil {
		t.Fatalf("Recover (checkpoint) should succeed: %v", err)
	}
	if _, _, ok := orch.PeekRecoveredPlan(handle.SpiritSessionID); ok {
		t.Fatal("missing draft must not appear as recovered plan")
	}
}

func TestRecover_EmptyTaskPlanID_NotPretendRestored(t *testing.T) {
	ctx := context.Background()
	repo := newMemOrchestrationRepo()
	saver := newMemCheckpointSaver()
	orch := newRecoveryOrchestrator(repo, newMemTaskPlanRepo(), nil, saver)

	handle := &biz.OrchestrationHandle{
		ID:              "orch_p110_empty",
		SpiritSessionID: "spirit_sess_empty",
		Strategy:        biz.StrategyParallel,
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "ckpt_empty",
	}
	repo.Create(ctx, handle)
	putCheckpoint(t, saver, handle)

	if err := orch.Recover(ctx, handle.ID); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if _, _, ok := orch.PeekRecoveredPlan(handle.SpiritSessionID); ok {
		t.Fatal("handle without task_plan_id must not report a recovered plan")
	}
}

func TestRecoverAllInterrupted_RestoresOrphanedDraft(t *testing.T) {
	ctx := context.Background()
	repo := newMemOrchestrationRepo()
	draft := &biz.TaskPlan{
		ID:              "tp_orphan",
		SpiritSessionID: "spirit_orphan",
		UserMessage:     "排查生产告警",
		Strategy:        biz.StrategyParallel,
		Status:          biz.TaskPlanStatusDraft,
		SubTasks:        []biz.SubTask{{ID: "st_a", Name: "定位"}},
	}
	alloc := &biz.AllocationPlan{
		ID:              "ap_orphan",
		TaskPlanID:      draft.ID,
		SpiritSessionID: draft.SpiritSessionID,
	}
	orch := newRecoveryOrchestrator(repo, newMemTaskPlanRepo(draft), newMemAllocPlanRepo(alloc), newMemCheckpointSaver())

	if err := orch.RecoverAllInterrupted(ctx); err != nil {
		t.Fatalf("RecoverAllInterrupted: %v", err)
	}
	plan, gotAlloc, ok := orch.PeekRecoveredPlan(draft.SpiritSessionID)
	if !ok || plan == nil || plan.ID != draft.ID {
		t.Fatalf("orphaned draft not restored: ok=%v plan=%+v", ok, plan)
	}
	if gotAlloc == nil || gotAlloc.ID != alloc.ID {
		t.Fatalf("orphaned AllocationPlan not restored: %+v", gotAlloc)
	}
}

func TestRecoverAllInterrupted_IncompleteDraftSkipped(t *testing.T) {
	ctx := context.Background()
	incomplete := &biz.TaskPlan{
		ID:              "tp_incomplete",
		SpiritSessionID: "spirit_incomplete",
		UserMessage:     "复杂长任务",
		Strategy:        biz.StrategyDAG,
		Status:          biz.TaskPlanStatusDraft,
		SubTasks:        nil, // crash during LLM decompose
	}
	orch := newRecoveryOrchestrator(newMemOrchestrationRepo(), newMemTaskPlanRepo(incomplete), nil, newMemCheckpointSaver())

	if err := orch.RecoverAllInterrupted(ctx); err != nil {
		t.Fatalf("RecoverAllInterrupted: %v", err)
	}
	if _, _, ok := orch.PeekRecoveredPlan(incomplete.SpiritSessionID); ok {
		t.Fatal("incomplete draft (no subtasks) must be skipped, not pretended recovered")
	}
}

func TestRecoverAllInterrupted_CompletedPlanSkipped(t *testing.T) {
	ctx := context.Background()
	done := &biz.TaskPlan{
		ID:              "tp_done",
		SpiritSessionID: "spirit_done",
		Strategy:        biz.StrategyDAG,
		Status:          biz.TaskPlanStatusCompleted,
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "done"}},
	}
	orch := newRecoveryOrchestrator(newMemOrchestrationRepo(), newMemTaskPlanRepo(done), nil, newMemCheckpointSaver())
	if err := orch.RecoverAllInterrupted(ctx); err != nil {
		t.Fatalf("RecoverAllInterrupted: %v", err)
	}
	if _, _, ok := orch.PeekRecoveredPlan(done.SpiritSessionID); ok {
		t.Fatal("completed plan must not be restored as active")
	}
}

func TestConsumeRecoveredPlan_MatchesUserMessage(t *testing.T) {
	draft := &biz.TaskPlan{
		ID:              "tp_consume",
		SpiritSessionID: "spirit_consume",
		UserMessage:     "原任务",
		Strategy:        biz.StrategyDirect,
		Status:          biz.TaskPlanStatusDraft,
	}
	orch := newRecoveryOrchestrator(newMemOrchestrationRepo(), newMemTaskPlanRepo(draft), nil, nil)
	if err := orch.RecoverAllInterrupted(context.Background()); err != nil {
		t.Fatalf("RecoverAllInterrupted: %v", err)
	}

	if _, _, ok := orch.ConsumeRecoveredPlan(draft.SpiritSessionID, "别的任务"); ok {
		t.Fatal("different user message must not consume the recovered plan")
	}
	plan, _, ok := orch.ConsumeRecoveredPlan(draft.SpiritSessionID, "原任务")
	if !ok || plan == nil || plan.ID != draft.ID {
		t.Fatal("matching user message should consume the recovered plan")
	}
	if _, _, ok := orch.PeekRecoveredPlan(draft.SpiritSessionID); ok {
		t.Fatal("consumed plan must leave the cache")
	}
}

func TestRecoverAllInterrupted_HandleRecoverFailsStillRestoresPlan(t *testing.T) {
	ctx := context.Background()
	repo := newMemOrchestrationRepo()
	draft := &biz.TaskPlan{
		ID:              "tp_after_fail",
		SpiritSessionID: "spirit_after_fail",
		UserMessage:     "长任务",
		Strategy:        biz.StrategyCoordinator,
		Status:          biz.TaskPlanStatusExecuting,
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "step"}},
	}
	handle := &biz.OrchestrationHandle{
		ID:              "orch_no_ckpt",
		TaskPlanID:      draft.ID,
		SpiritSessionID: draft.SpiritSessionID,
		Strategy:        biz.StrategyCoordinator,
		Status:          biz.OrchestrationStatusInterrupted,
		CheckpointID:    "", // cannot resume graph
	}
	repo.Create(ctx, handle)
	orch := newRecoveryOrchestrator(repo, newMemTaskPlanRepo(draft), nil, nil)

	if err := orch.RecoverAllInterrupted(ctx); err != nil {
		t.Fatalf("RecoverAllInterrupted: %v", err)
	}
	plan, _, ok := orch.PeekRecoveredPlan(draft.SpiritSessionID)
	if !ok || plan == nil || plan.ID != draft.ID {
		t.Fatal("graph checkpoint failure must not drop a restorable TaskPlan")
	}
}

func TestRestorablePlanReason(t *testing.T) {
	if reason, ok := restorablePlanReason(nil); ok || reason != "plan_nil" {
		t.Fatalf("nil plan: reason=%q ok=%v", reason, ok)
	}
	if _, ok := restorablePlanReason(&biz.TaskPlan{
		SpiritSessionID: "s", Status: biz.TaskPlanStatusDraft, Strategy: biz.StrategyDirect,
	}); !ok {
		t.Fatal("direct draft without subtasks should be restorable")
	}
	if reason, ok := restorablePlanReason(&biz.TaskPlan{
		SpiritSessionID: "s", Status: biz.TaskPlanStatusDraft, Strategy: biz.StrategyDAG,
	}); ok || reason != "incomplete_draft_no_subtasks" {
		t.Fatalf("dag draft without subtasks: reason=%q ok=%v", reason, ok)
	}
}
