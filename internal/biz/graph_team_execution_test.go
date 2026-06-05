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
	uc := NewGraphUsecase(nil, repo, nil, nil, nil, loggateway.NewNoop())
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{{ID: "review-1", Type: "review", InterruptAfter: true}},
		EntryPoint: "review-1", FinishPoint: "review-1",
	}
	ct := NewCompiledTeam(cfg, nil, nil, nil)
	if err := uc.RegisterTeamGraphExecution(context.Background(), "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
		t.Fatal(err)
	}
	gotCt, err := uc.buildConfigForExecution(context.Background(), &GraphExecution{ID: "exec-1", GraphID: "team:team-1:run-1"})
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
