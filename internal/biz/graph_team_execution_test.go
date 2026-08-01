package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type memGraphRunRepo struct {
	runs map[string]*GraphExecution
}

func (m *memGraphRunRepo) SaveRun(_ context.Context, exec *GraphExecution) error {
	if m.runs == nil {
		m.runs = map[string]*GraphExecution{}
	}
	m.runs[exec.ID] = exec
	return nil
}

func (m *memGraphRunRepo) GetRun(_ context.Context, id string) (*GraphExecution, error) {
	if exec, ok := m.runs[id]; ok {
		return exec, nil
	}
	return nil, ErrNotFound
}

func (m *memGraphRunRepo) ListRunsByGraph(_ context.Context, _ string, _ int, _ string, _ ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return nil, "", nil
}

func (m *memGraphRunRepo) UpdateRun(_ context.Context, exec *GraphExecution) error {
	if m.runs == nil {
		m.runs = map[string]*GraphExecution{}
	}
	m.runs[exec.ID] = exec
	return nil
}

func TestRegisterTeamGraphExecution_andInterrupt(t *testing.T) {
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{}}
	uc := NewGraphUsecase(GraphUsecaseDeps{RunRepo: repo, Lg: loggateway.NewNoop()})
	cfg := GraphBuildConfig{
		Nodes:      []NodeDef{{ID: "review-1", Type: "review", InterruptAfter: true}},
		EntryPoint: "review-1", FinishPoint: "review-1",
	}
	ct := NewCompiledTeam(cfg, nil, nil, nil)
	// B9：linked_graph_id 非空 → graph_id 用真实图资产 ID。
	if err := uc.RegisterTeamGraphExecution(context.Background(), "exec-1", "sess-1", "sess-1", "team-1", "run-1", "g-asset-1", ct); err != nil {
		t.Fatal(err)
	}
	registered, err := uc.GetExecution(context.Background(), "exec-1")
	if err != nil {
		t.Fatal(err)
	}
	if registered.GraphID != "g-asset-1" {
		t.Fatalf("graph_id = %q, want linked asset id g-asset-1", registered.GraphID)
	}
	// B9：linked 为空（存量未迁移）→ 保留 team: 合成 ID 兜底。
	if err := uc.RegisterTeamGraphExecution(context.Background(), "exec-legacy", "sess-1", "sess-1", "team-1", "run-2", "", ct); err != nil {
		t.Fatal(err)
	}
	legacy, err := uc.GetExecution(context.Background(), "exec-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.GraphID != "team:team-1:run-2" {
		t.Fatalf("legacy graph_id = %q, want team:team-1:run-2", legacy.GraphID)
	}
	gotCt, err := uc.buildConfigForExecution(context.Background(), &GraphExecution{ID: "exec-1", GraphID: "g-asset-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotCt.Nodes) != 1 || gotCt.Nodes[0].ID != "review-1" {
		t.Fatalf("cfg=%+v", gotCt)
	}
	if err := uc.MarkTeamGraphInterrupt(context.Background(), "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	exec, err := uc.GetExecution(context.Background(), "exec-1")
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != "waiting_human" || exec.InterruptNode != "review-1" || exec.LineageID != "lineage-1" {
		t.Fatalf("exec=%+v", exec)
	}
}

// F-B：team 路径绕过 trpcGraphRuntime.Run，无 consumeRuntimeEvents 消费者，
// graph_executions 行不会随运行结束收敛。RecordTeamGraphNodeEnd 增量落 steps_json，
// FinalizeTeamGraphExecution 在 team run 终态时收敛 status/finished_at。
func TestRecordTeamGraphNodeEnd_persistsStepsIncrementally(t *testing.T) {
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{}}
	uc := NewGraphUsecase(GraphUsecaseDeps{RunRepo: repo, Lg: loggateway.NewNoop()})
	ct := NewCompiledTeam(GraphBuildConfig{
		Nodes:      []NodeDef{{ID: "member-1", Type: "agent"}, {ID: "member-2", Type: "agent"}},
		EntryPoint: "member-1", FinishPoint: "member-2",
	}, nil, nil, nil)
	ctx := context.Background()
	if err := uc.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "g-1", ct); err != nil {
		t.Fatal(err)
	}
	if err := uc.RecordTeamGraphNodeEnd(ctx, "exec-1", "member-1", 1, "completed", ""); err != nil {
		t.Fatal(err)
	}
	if err := uc.RecordTeamGraphNodeEnd(ctx, "exec-1", "member-2", 2, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	// 持久化快照（repo 内为 UpdateRun 写入的拷贝）必须携带 steps。
	persisted := repo.runs["exec-1"]
	if persisted == nil {
		t.Fatal("exec not persisted")
	}
	if len(persisted.Steps) != 2 {
		t.Fatalf("persisted steps=%d want 2", len(persisted.Steps))
	}
	if persisted.Steps[0].NodeID != "member-1" || persisted.Steps[0].StepIndex != 1 || persisted.Steps[0].Status != "completed" {
		t.Fatalf("step0=%+v", persisted.Steps[0])
	}
	if persisted.Steps[1].NodeID != "member-2" || persisted.Steps[1].Status != "failed" || persisted.Steps[1].Error != "boom" {
		t.Fatalf("step1=%+v", persisted.Steps[1])
	}
	// 同节点同 step 重复 node_end（事件去重）→ upsert 而非追加。
	if err := uc.RecordTeamGraphNodeEnd(ctx, "exec-1", "member-2", 2, "completed", ""); err != nil {
		t.Fatal(err)
	}
	persisted = repo.runs["exec-1"]
	if len(persisted.Steps) != 2 {
		t.Fatalf("after upsert steps=%d want 2", len(persisted.Steps))
	}
	if persisted.Steps[1].Status != "completed" || persisted.Steps[1].Error != "" {
		t.Fatalf("upserted step1=%+v", persisted.Steps[1])
	}
}

func TestFinalizeTeamGraphExecution_convergesTerminalState(t *testing.T) {
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{}}
	uc := NewGraphUsecase(GraphUsecaseDeps{RunRepo: repo, Lg: loggateway.NewNoop()})
	ct := NewCompiledTeam(GraphBuildConfig{
		Nodes:      []NodeDef{{ID: "member-1", Type: "agent"}},
		EntryPoint: "member-1", FinishPoint: "member-1",
	}, nil, nil, nil)
	ctx := context.Background()
	if err := uc.RegisterTeamGraphExecution(ctx, "exec-ok", "sess-1", "sess-1", "team-1", "run-1", "g-1", ct); err != nil {
		t.Fatal(err)
	}
	if err := uc.FinalizeTeamGraphExecution(ctx, "exec-ok", false, ""); err != nil {
		t.Fatal(err)
	}
	persisted := repo.runs["exec-ok"]
	if persisted.Status != string(GraphExecCompleted) {
		t.Fatalf("status=%q want completed", persisted.Status)
	}
	if persisted.FinishedAt == nil {
		t.Fatal("finished_at not set")
	}
	// 幂等：终态后重复 finalize（含相反结局）不得改变状态。
	if err := uc.FinalizeTeamGraphExecution(ctx, "exec-ok", true, "late failure"); err != nil {
		t.Fatal(err)
	}
	if repo.runs["exec-ok"].Status != string(GraphExecCompleted) {
		t.Fatalf("idempotency broken: status=%q want still completed", repo.runs["exec-ok"].Status)
	}

	// 失败终态：status=failed + error_message 落库。
	if err := uc.RegisterTeamGraphExecution(ctx, "exec-bad", "sess-1", "sess-1", "team-1", "run-2", "g-1", ct); err != nil {
		t.Fatal(err)
	}
	if err := uc.FinalizeTeamGraphExecution(ctx, "exec-bad", true, "node exploded"); err != nil {
		t.Fatal(err)
	}
	bad := repo.runs["exec-bad"]
	if bad.Status != string(GraphExecFailed) || bad.ErrorMessage != "node exploded" || bad.FinishedAt == nil {
		t.Fatalf("bad=%+v", bad)
	}
}

func TestGraphTaskInputFromNode_defaults(t *testing.T) {
	role, mode, strategy, input := GraphTaskInputFromNode(NodeDef{ID: "t1", Type: "task"}, NodeTaskMeta{})
	if role != "" || mode != "static" || strategy != "" || input != "t1" {
		t.Fatalf("role=%q mode=%q strategy=%q input=%q", role, mode, strategy, input)
	}
}

func TestShouldCreateTeamGraphTaskNode_vsStandalone(t *testing.T) {
	agent := NodeDef{ID: "a1", Type: "agent", AgentName: "worker"}
	if !ShouldCreateTaskForNode(&agent, NodeTaskMeta{}) {
		t.Fatal("standalone graph should still create task for agent when policy applies")
	}
	if ShouldCreateTeamGraphTaskNode(&agent) {
		t.Fatal("team graph must not create task for agent node")
	}
	if !ShouldCreateTeamGraphTaskNode(&NodeDef{Type: "task"}) {
		t.Fatal("team task node expected")
	}
}
