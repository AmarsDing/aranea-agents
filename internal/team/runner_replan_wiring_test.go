package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/graph"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// G2（ADR-F D2）：team 域经 run 级 RuntimeState 注入 replanner 全局回调——
// 静态声明（per-node failure_recovery）先跑，未恢复时智能轨兜底。

func TestReplanCallbacksRunOption_InjectsCallbacks(t *testing.T) {
	t.Parallel()
	r := &Runner{
		cfg: RunnerConfig{Replanner: &stubWiringReplanner{}},
		lg:  loggateway.NewNoop(),
	}
	opt := r.replanCallbacksRunOption("s1", "sp1", "g1", "exec-1")
	if opt == nil {
		t.Fatal("graph mode + replanner configured → option must be non-nil")
	}
	var ro trpcagent.RunOptions
	opt(&ro)
	cb, ok := ro.RuntimeState[trpcgraph.StateKeyNodeCallbacks].(*trpcgraph.NodeCallbacks)
	if !ok || cb == nil {
		t.Fatalf("RuntimeState missing NodeCallbacks: %v", ro.RuntimeState)
	}
	if len(cb.AfterNode) == 0 {
		t.Fatal("callbacks must carry the replanner AfterNode decision path")
	}
}

func TestReplanCallbacksRunOption_NilReplannerSkips(t *testing.T) {
	t.Parallel()
	r := &Runner{cfg: RunnerConfig{}, lg: loggateway.NewNoop()}
	if opt := r.replanCallbacksRunOption("s1", "sp1", "g1", "exec-1"); opt != nil {
		t.Fatal("nil replanner → nil option（保持纯静态现状行为）")
	}
}

func TestReplanCallbacksRunOption_NonGraphModeSkips(t *testing.T) {
	t.Parallel()
	r := &Runner{
		cfg: RunnerConfig{Replanner: &stubWiringReplanner{}},
		lg:  loggateway.NewNoop(),
	}
	// DAG 派步团队无 graphExecID——graph executor 之外 callbacks 无消费者。
	if opt := r.replanCallbacksRunOption("s1", "sp1", "g1", ""); opt != nil {
		t.Fatal("empty graphExecID (non-graph mode) → nil option")
	}
}

type stubWiringReplanner struct{}

func (s *stubWiringReplanner) OnNodeFailure(context.Context, *biz.GraphExecution, string, error) (*graph.ReplanAction, error) {
	return nil, nil
}

func (s *stubWiringReplanner) ReleaseExecution(string) {}
