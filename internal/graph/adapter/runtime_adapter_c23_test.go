package adapter

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/graph"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// C-23 → ADR-F（G2）演进说明：
//   - ReplanRetry：C-23 一律 fail-closed；ADR-F 起 agent 节点走 Reflexion 智能
//     重试（见 runtime_adapter_g2_test.go），仅无执行载体（非 agent 节点）时保持
//     fail-closed。
//   - ReplanInsertFallback：保持 fail-closed InterruptError（HITL），本文保留
//     详细断言。
//   - ReplanReroute：C-23 传播 (nil,nil)；ADR-F 起退化为 skip（g2 测试覆盖）。
// Soft-recover（把 ControlCommand 当 AfterNode result 返回）仍被禁止。

func TestApplyReplanControl_InsertFallbackFailClosedInterrupt(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{}
	action := &graph.ReplanAction{
		Type: graph.ReplanInsertFallback,
		NewNodes: []biz.NodeDef{
			{ID: "member-1_fallback", Type: biz.NodeTypeAgent, AgentName: "backup-agent"},
		},
	}

	out, err := applyReplanControl(context.Background(), state, "member-1", errors.New("agent incapable"), action, nil)
	if out != nil {
		t.Fatalf("must not soft-recover with result %T", out)
	}
	if err == nil {
		t.Fatal("expected InterruptError for interrupt/resume path")
	}
	if !trpcgraph.IsInterruptError(err) {
		t.Fatalf("want InterruptError, got %T: %v", err, err)
	}
	intr, ok := trpcgraph.GetInterruptError(err)
	if !ok || intr == nil {
		t.Fatal("GetInterruptError failed")
	}
	payload, ok := intr.Value.(map[string]any)
	if !ok {
		t.Fatalf("interrupt value=%T", intr.Value)
	}
	if payload["fallback_agent"] != "backup-agent" {
		t.Fatalf("fallback_agent=%v", payload["fallback_agent"])
	}
	stored, ok := state[graph.StateKeyControlCommand]
	if !ok {
		t.Fatal("expected ControlCommand in state")
	}
	cmd, ok := graph.AsControlCommand(stored)
	if !ok || cmd.FallbackAgent != "backup-agent" {
		t.Fatalf("ControlCommand=%+v", cmd)
	}
	if graph.IsControlCommand(out) {
		t.Fatal("ControlCommand must not be the AfterNode result (would soft-recover)")
	}
}

func TestSanitizeActivityControlCommand_ClearsControlContent(t *testing.T) {
	t.Parallel()
	ev := &biz.ActivityEvent{
		Activity: biz.Activity{
			Content: "ControlCommand{action=retry node=n1 fallback= allowed=true}",
			Meta:    map[string]any{},
		},
	}
	sanitizeActivityControlCommand(ev, nil)
	if ev.Activity.Content != "" {
		t.Fatalf("content should be cleared, got %q", ev.Activity.Content)
	}
	if ev.Activity.Meta["control_command"] != true {
		t.Fatal("expected control_command meta flag")
	}

	ev2 := &biz.ActivityEvent{
		Activity: biz.Activity{
			Content: "user-visible text",
			Meta: map[string]any{
				"interrupt_value": map[string]any{
					"control":        "insert_fallback",
					"fallback_agent": "backup",
				},
			},
		},
	}
	sanitizeActivityControlCommand(ev2, nil)
	if ev2.Activity.Content != "" {
		t.Fatalf("interrupt control should clear content, got %q", ev2.Activity.Content)
	}
	if ev2.Activity.Meta["fallback_agent"] != "backup" {
		t.Fatalf("fallback_agent=%v", ev2.Activity.Meta["fallback_agent"])
	}
}

// S3：RuntimeReplanner.attemptCount（ManagedMap，ttl=0）的 entry 必须在执行流
// 结束时释放——ReleaseExecution 定义后无任何生产调用方，每个发生过节点失败的
// 执行永久泄漏一条 entry。forwardEvents 是 Run/Resume 统一的流结束点（defer
// 必定执行），必须在此释放。
func TestForwardEvents_ReleasesReplannerOnStreamEnd(t *testing.T) {
	t.Parallel()
	spy := &releaseSpyReplanner{}
	rt := &trpcGraphRuntime{
		execID:    "exec-s3",
		lg:        loggateway.NewNoop(),
		replanner: spy,
	}
	eventCh := make(chan *trpcevent.Event)
	close(eventCh)
	out := make(chan biz.GraphRuntimeEvent, 1)
	rt.forwardEvents(eventCh, out, nil)
	if spy.releasedFor != "exec-s3" {
		t.Fatalf("ReleaseExecution not called on stream end (releasedFor=%q)", spy.releasedFor)
	}
}

type releaseSpyReplanner struct{ releasedFor string }

func (s *releaseSpyReplanner) OnNodeFailure(context.Context, *biz.GraphExecution, string, error) (*graph.ReplanAction, error) {
	return nil, nil
}

func (s *releaseSpyReplanner) ReleaseExecution(execID string) { s.releasedFor = execID }
