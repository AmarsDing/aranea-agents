package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// F5：定义解析/运行时构建失败的启动也必须落库终态行——此前仅通知
// telemetry/projector，执行历史与孤儿对账完全无痕。

type errGraphDefProvider struct{ err error }

func (p errGraphDefProvider) GetGraph(context.Context, string) (*GraphDefinition, error) {
	return nil, p.err
}

func TestExecuteGraph_FailedStart_PersistsTerminalRow(t *testing.T) {
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{}}
	uc := NewGraphExecutionUsecase(repo, nil, nil,
		NewGraphCacheManager(nil, nil, nil, loggateway.NewNoop()),
		errGraphDefProvider{err: errors.New("graph gone")}, loggateway.NewNoop(), DefaultGraphGCConfig())

	_, err := uc.ExecuteGraph(context.Background(), "missing-graph", "sess-1", "exec-fail-1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	row, ok := repo.runs["exec-fail-1"]
	if !ok {
		t.Fatal("failed start must persist a terminal row (F5)")
	}
	if row.Status != string(GraphExecFailed) {
		t.Fatalf("status=%q, want failed", row.Status)
	}
	if row.ErrorMessage == "" {
		t.Fatal("error_message must record the failure reason")
	}
	if row.FinishedAt == nil {
		t.Fatal("finished_at must be set on the terminal row")
	}
}

func TestExecuteGraph_BuildFailure_PersistsTerminalRow(t *testing.T) {
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{}}
	def := &GraphDefinition{ID: "g-1", Name: "ok"}
	// fakeRecoverFactory.BuildAndRun fails unconditionally — the build-error branch.
	uc := NewGraphExecutionUsecase(repo, &fakeRecoverFactory{}, nil,
		NewGraphCacheManager(nil, nil, nil, loggateway.NewNoop()),
		&stubGraphDefProvider{def: def}, loggateway.NewNoop(), DefaultGraphGCConfig())

	_, err := uc.ExecuteGraph(context.Background(), "g-1", "sess-1", "exec-fail-2", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	row, ok := repo.runs["exec-fail-2"]
	if !ok {
		t.Fatal("build failure must persist a terminal row (F5)")
	}
	if row.Status != string(GraphExecFailed) || row.ErrorMessage == "" {
		t.Fatalf("row=%+v, want failed with message", row)
	}
}
