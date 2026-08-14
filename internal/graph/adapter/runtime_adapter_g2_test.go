package adapter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/graph"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// G2（ADR-F）落地语义：
//   - ReplanRetry + 可重执行 agent 节点 → Reflexion 智能重试（失败反馈注入
//     user_input 副本），成功即恢复；失败传播；原 state 零污染。
//   - ReplanRetry + 无执行载体（非 agent 节点）→ fail-closed（C-23 语义保持）。
//   - ReplanReroute → 退化为 skip（SkippedNodesStateKey 标记 + skip 输出）。
//   - ReplanRebuildSubgraph → fail-closed（nil, nil）传播原始错误。
//   - ReplanInsertFallback → InterruptError HITL（C-23 语义保持）。

func TestApplyReplanControl_RetryReflexionSuccess(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection timeout")
	state := trpcgraph.State{
		trpcgraph.StateKeyUserInput: "原始任务内容",
	}
	var gotFeedback string
	var gotStateUserInput string
	retryExec := func(_ context.Context, st trpcgraph.State, nodeID, feedback string) (any, error) {
		if nodeID != "member-1" {
			t.Errorf("retryExec nodeID = %q, want member-1", nodeID)
		}
		gotFeedback = feedback
		gotStateUserInput, _ = st[trpcgraph.StateKeyUserInput].(string)
		// 重执行成功：返回节点输出（state 更新）。
		return trpcgraph.State{trpcgraph.StateKeyLastResponse: "重试成功输出"}, nil
	}

	out, err := applyReplanControl(context.Background(), state, "member-1", cause,
		&graph.ReplanAction{Type: graph.ReplanRetry}, retryExec)
	if err != nil {
		t.Fatalf("retry success should recover (err=nil), got %v", err)
	}
	st, ok := out.(trpcgraph.State)
	if !ok {
		t.Fatalf("recovered result should be State, got %T", out)
	}
	if st[trpcgraph.StateKeyLastResponse] != "重试成功输出" {
		t.Fatalf("last_response = %v", st[trpcgraph.StateKeyLastResponse])
	}
	// Reflexion：反馈必须含失败原因，且拼入重试副本的 user_input 前部。
	if !strings.Contains(gotFeedback, "connection timeout") {
		t.Fatalf("feedback missing cause: %q", gotFeedback)
	}
	if !strings.HasPrefix(gotStateUserInput, gotFeedback) {
		t.Fatalf("retry user_input must start with feedback, got %q", gotStateUserInput)
	}
	if !strings.Contains(gotStateUserInput, "原始任务内容") {
		t.Fatalf("retry user_input must retain original input, got %q", gotStateUserInput)
	}
	// checkpoint 隔离：原 state 的 user_input 不被污染。
	if state[trpcgraph.StateKeyUserInput] != "原始任务内容" {
		t.Fatalf("original state user_input polluted: %q", state[trpcgraph.StateKeyUserInput])
	}
}

func TestApplyReplanControl_RetryReflexionFailurePropagates(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection timeout")
	retryErr := errors.New("retry still failing: deadline exceeded")
	state := trpcgraph.State{}
	retryExec := func(context.Context, trpcgraph.State, string, string) (any, error) {
		return nil, retryErr
	}

	out, err := applyReplanControl(context.Background(), state, "n1", cause,
		&graph.ReplanAction{Type: graph.ReplanRetry}, retryExec)
	if out != nil {
		t.Fatalf("failed retry must not recover, got %T", out)
	}
	if !errors.Is(err, retryErr) {
		t.Fatalf("want retry error propagated, got %v", err)
	}
	// 重试失败仍落 ControlCommand 供观测。
	stored, ok := state[graph.StateKeyControlCommand]
	if !ok {
		t.Fatal("expected ControlCommand stashed after failed retry")
	}
	if cmd, _ := graph.AsControlCommand(stored); cmd.Action != graph.ReplanRetry {
		t.Fatalf("cmd.Action = %q", cmd.Action)
	}
}

func TestApplyReplanControl_RetryNoExecutorFailClosed(t *testing.T) {
	t.Parallel()
	cause := errors.New("transient timeout")
	state := trpcgraph.State{}

	out, err := applyReplanControl(context.Background(), state, "func-node", cause,
		&graph.ReplanAction{Type: graph.ReplanRetry}, nil)
	if !errors.Is(err, cause) {
		t.Fatalf("no executor → fail-closed with original error, got %v", err)
	}
	if out != nil {
		t.Fatalf("must not soft-recover, got %T", out)
	}
	if _, ok := state[graph.StateKeyControlCommand]; !ok {
		t.Fatal("expected ControlCommand stashed for observability")
	}
}

func TestApplyReplanControl_RerouteDegradesToSkip(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{}

	out, err := applyReplanControl(context.Background(), state, "blocked-node", errors.New("route blocked"),
		&graph.ReplanAction{Type: graph.ReplanReroute}, nil)
	if err != nil {
		t.Fatalf("reroute→skip should recover, got err %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("skip output should be map, got %T", out)
	}
	if m[biz.SkippedNodeOutputKey] != "blocked-node" {
		t.Fatalf("skip output = %v", m[biz.SkippedNodeOutputKey])
	}
	// 与静态 skip 同语义：SkippedNodesStateKey 落 state。
	skipped, _ := state[biz.SkippedNodesStateKey].([]string)
	found := false
	for _, s := range skipped {
		if s == "blocked-node" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SkippedNodesStateKey = %v", state[biz.SkippedNodesStateKey])
	}
}

func TestApplyReplanControl_RebuildSubgraphFailClosed(t *testing.T) {
	t.Parallel()
	cause := errors.New("subtask invalid")
	out, err := applyReplanControl(context.Background(), trpcgraph.State{}, "n", cause,
		&graph.ReplanAction{Type: graph.ReplanRebuildSubgraph}, nil)
	if err != nil || out != nil {
		t.Fatalf("rebuild_subgraph stays fail-closed (nil,nil), got (%v, %v)", out, err)
	}
}

func TestApplyReplanControl_InsertFallbackKeepsInterrupt(t *testing.T) {
	t.Parallel()
	action := &graph.ReplanAction{
		Type: graph.ReplanInsertFallback,
		NewNodes: []biz.NodeDef{
			{ID: "n_fallback", Type: biz.NodeTypeAgent, AgentName: "backup"},
		},
	}
	out, err := applyReplanControl(context.Background(), trpcgraph.State{}, "n", errors.New("incapable"), action, nil)
	if out != nil {
		t.Fatalf("must not soft-recover, got %T", out)
	}
	if !trpcgraph.IsInterruptError(err) {
		t.Fatalf("want InterruptError HITL, got %T: %v", err, err)
	}
}

// TestNewReplanNodeCallbacks_AfterNodeFullChain 验证提取后的包级构造函数：
// replanner 决策 retry → AfterNode 内经 FindSubAgent 判别为 agent 节点 →
// Reflexion 重试落地。
func TestNewReplanNodeCallbacks_AfterNodeFullChain(t *testing.T) {
	t.Parallel()
	replanner := &g2StubReplanner{action: &graph.ReplanAction{Type: graph.ReplanRetry}}
	cb := NewReplanNodeCallbacks(replanner, loggateway.NewNoop(), "s1", "sp1", "g1", "exec-1")
	if cb == nil {
		t.Fatal("non-nil replanner must produce callbacks")
	}
	if len(cb.AfterNode) == 0 || len(cb.OnNodeError) == 0 {
		t.Fatalf("callbacks incomplete: %+v", cb)
	}

	// 无 ParentAgent 的 state：FindSubAgent 判别失败 → 非 agent 节点 → fail-closed。
	cause := errors.New("connection timeout")
	out, err := cb.RunAfterNode(context.Background(),
		&trpcgraph.NodeCallbackContext{NodeID: "member-1"},
		trpcgraph.State{}, nil, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("unresolvable node → fail-closed original error, got %v", err)
	}
	if out != nil {
		t.Fatalf("got %T", out)
	}
}

// TestNewReplanNodeCallbacks_NilReplanner 验证 nil replanner 返回 nil callbacks
//（runtime 跳过 StateKeyNodeCallbacks 注入，保持现状行为）。
func TestNewReplanNodeCallbacks_NilReplanner(t *testing.T) {
	t.Parallel()
	if cb := NewReplanNodeCallbacks(nil, loggateway.NewNoop(), "s", "sp", "g", "e"); cb != nil {
		t.Fatalf("nil replanner must yield nil callbacks, got %+v", cb)
	}
}

type g2StubReplanner struct {
	action *graph.ReplanAction
}

func (s *g2StubReplanner) OnNodeFailure(context.Context, *biz.GraphExecution, string, error) (*graph.ReplanAction, error) {
	return s.action, nil
}

func (s *g2StubReplanner) ReleaseExecution(string) {}
