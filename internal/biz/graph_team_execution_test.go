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
