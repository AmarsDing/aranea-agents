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

// C-23 policy under test:
//   - ReplanRetry → fail-closed (nil, originalErr); ControlCommand stashed in state only.
//   - ReplanInsertFallback → fail-closed via InterruptError (not fake "[fallback]" text).
// Soft-recover (returning ControlCommand as AfterNode result) is explicitly rejected.

func TestApplyReplanControl_RetryFailClosedNotSoftRecover(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{}
	cause := errors.New("transient timeout")
	action := &graph.ReplanAction{Type: graph.ReplanRetry}

	out, err := applyReplanControl(state, "node-a", cause, action)
	if err == nil {
		t.Fatal("retry should return (nil, originalErr), got err=nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("want originalErr wrapped/returned, got %v", err)
	}
	if out != nil {
		t.Fatalf("retry must not soft-recover with result %T (%v)", out, out)
	}
	stored, ok := state[graph.StateKeyControlCommand]
	if !ok {
		t.Fatal("expected ControlCommand stashed in state for observability")
	}
	cmd, ok := graph.AsControlCommand(stored)
	if !ok {
		t.Fatalf("state value not ControlCommand: %T", stored)
	}
	if cmd.Action != graph.ReplanRetry || cmd.NodeID != "node-a" {
		t.Fatalf("unexpected cmd: %+v", cmd)
	}
	if !cmd.AttemptAllowed {
		t.Fatal("AttemptAllowed should be true when replanner returned an action")
	}
}

func TestApplyReplanControl_InsertFallbackFailClosedInterrupt(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{}
	action := &graph.ReplanAction{
		Type: graph.ReplanInsertFallback,
		NewNodes: []biz.NodeDef{
			{ID: "member-1_fallback", Type: biz.NodeTypeAgent, AgentName: "backup-agent"},
		},
	}

	out, err := applyReplanControl(state, "member-1", errors.New("agent incapable"), action)
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

func TestApplyReplanControl_ReroutePropagatesNil(t *testing.T) {
	t.Parallel()
	out, err := applyReplanControl(nil, "n", errors.New("blocked"), &graph.ReplanAction{Type: graph.ReplanReroute})
	if err != nil || out != nil {
		t.Fatalf("want (nil,nil), got (%v, %v)", out, err)
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
