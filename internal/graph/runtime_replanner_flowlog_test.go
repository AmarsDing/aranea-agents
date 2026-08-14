package graph

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
)

// captureFlowBus captures MonitorEvents for flow-log assertions (graph-local
// mirror of event.captureMonitorBus, which is unexported).
type captureFlowBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *captureFlowBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
}

func (b *captureFlowBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *captureFlowBus) DropCount() uint64 { return 0 }

func (b *captureFlowBus) hasFlowStep(stepID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		step, _ := ev.Metadata["step_id"].(string)
		if step == stepID {
			return true
		}
	}
	return false
}

// Y7（双轨覆盖）：replan 会动态改写图拓扑（reroute/insert_fallback/
// rebuild_subgraph），此前只有 system.notice（前端时间线）+ 进程日志，
// 「流程日志」Tab 无感知——业务用户看图执行被改道却找不到任何流程记录。
// 决定 replan action 后必须发射流程日志 graph.replan.decided。
func TestRuntimeReplanner_EmitsFlowLogOnReplan(t *testing.T) {
	bus := &recordingReplanBus{}
	r := newTestReplanner(bus)
	monBus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: monBus}, event.TraceContext{
		TraceID: "tr-y7",
		Domain:  event.TraceDomainGraph,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	exec := testReplanExecution()
	_, err := r.OnNodeFailure(ctx, exec, "step1", errors.New("connection timeout"))
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}
	if !monBus.hasFlowStep("graph.replan.decided") {
		t.Fatalf("expected flow-log step graph.replan.decided, got %+v", monBus.evs)
	}
}

// 无 TraceEmitter 的 ctx（如后台恢复路径）不得 panic，且不阻塞 replan 决策。
func TestRuntimeReplanner_NoEmitterStillDecides(t *testing.T) {
	r := newTestReplanner(nil)
	exec := testReplanExecution()
	action, err := r.OnNodeFailure(context.Background(), exec, "step1", errors.New("connection timeout"))
	if err != nil {
		t.Fatalf("OnNodeFailure failed: %v", err)
	}
	if action == nil || action.Type != ReplanRetry {
		t.Fatalf("action=%+v want retry", action)
	}
}
