package trace_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor/trace"
	"aranea-agents/internal/event/contract"
)

func flowEvent(traceID, domain, step, phase string) contract.MonitorEvent {
	return contract.MonitorEvent{
		Type:     contract.MonitorEventTypeFlowLog,
		Metadata: map[string]any{"trace_id": traceID, "domain": domain, "step_id": step, "flow_phase": phase},
	}
}

// 系统日志（自检、WebSocket 断线、定时任务分发）不是运行：不建追踪，也不算运行活动。
func TestTraceProjector_SystemDomainIsNotARun(t *testing.T) {
	inserted := 0
	repo := &mockTraceWriter{insertMonitorTraceFn: func(context.Context, trace.TraceWrite) error { inserted++; return nil }}
	p := newTestTraceProjector(repo)
	p.HandleExposed(context.Background(), flowEvent("t-sys", "system", "monitor.selfcheck.run", "start"))
	if inserted != 0 || p.TraceCount() != 0 {
		t.Fatalf("system event created a trace: inserted=%d count=%d", inserted, p.TraceCount())
	}
	if !p.LastRunnerFlowAt().IsZero() {
		t.Fatal("system event must not count as runner flow activity")
	}
	if p.LastEventAt().IsZero() {
		t.Fatal("projector liveness (LastEventAt) should still advance on system events")
	}
}

// 对话运行算运行活动；不会被空闲收尾（由 runner.completion 收尾）。
func TestTraceProjector_ChatFlowCountsAndIsNotIdleClosed(t *testing.T) {
	closed := 0
	repo := &mockTraceWriter{updateMonitorTraceCompletionFn: func(context.Context, string, trace.TraceCompletion) error { closed++; return nil }}
	p := newTestTraceProjector(repo)
	p.HandleExposed(context.Background(), flowEvent("t-chat", "chat", "chat.turn", "start"))
	if p.LastRunnerFlowAt().IsZero() {
		t.Fatal("chat event should advance LastRunnerFlowAt")
	}
	p.AgeTraceForTest("t-chat", 5*time.Minute)
	p.CloseIdleTracesExposed(context.Background())
	if closed != 0 || p.TraceCount() != 1 {
		t.Fatalf("chat trace must wait for runner completion: closed=%d count=%d", closed, p.TraceCount())
	}
}

// 任务类流程：明确的结束步骤立即收尾，结果照实（完成 / 失败 / 取消）。
func TestTraceProjector_TaskTerminalStepClosesImmediately(t *testing.T) {
	cases := []struct{ step, phase, want string }{
		{"agentbridge.task.done", "done", "ok"},
		{"agentbridge.task.failed", "error", "error"},
		{"agentbridge.task.cancelled", "done", "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			var got []trace.TraceCompletion
			repo := &mockTraceWriter{updateMonitorTraceCompletionFn: func(_ context.Context, _ string, c trace.TraceCompletion) error {
				got = append(got, c)
				return nil
			}}
			p := newTestTraceProjector(repo)
			ctx := context.Background()
			p.HandleExposed(ctx, flowEvent("t-ab", "agentbridge", "agentbridge.task.dispatch", "done"))
			p.HandleExposed(ctx, flowEvent("t-ab", "agentbridge", tc.step, tc.phase))
			if len(got) != 1 || got[0].Status != tc.want || p.TraceCount() != 0 {
				t.Fatalf("completions=%+v count=%d, want one %q", got, p.TraceCount(), tc.want)
			}
			// 收尾后到达的进程退出日志只补 span，不重开、不再改结果
			p.HandleExposed(ctx, flowEvent("t-ab", "agentbridge", "agentbridge.process.exit", "done"))
			p.AgeTraceForTest("t-ab", 5*time.Minute)
			p.CloseIdleTracesExposed(ctx)
			if len(got) != 1 || p.TraceCount() != 0 {
				t.Fatalf("late event reopened or overwrote the trace: completions=%d count=%d", len(got), p.TraceCount())
			}
		})
	}
}

// 派发后没有新日志的长任务不按空闲收尾，等结束步骤。
func TestTraceProjector_OpenTaskNotIdleClosed(t *testing.T) {
	closed := 0
	repo := &mockTraceWriter{updateMonitorTraceCompletionFn: func(context.Context, string, trace.TraceCompletion) error { closed++; return nil }}
	p := newTestTraceProjector(repo)
	p.HandleExposed(context.Background(), flowEvent("t-long", "agentbridge", "agentbridge.task.dispatch", "done"))
	p.AgeTraceForTest("t-long", 5*time.Minute)
	p.CloseIdleTracesExposed(context.Background())
	if closed != 0 || p.TraceCount() != 1 {
		t.Fatalf("open task closed early: closed=%d count=%d", closed, p.TraceCount())
	}
}

// 其他没有结束信号的域（一次性事件、工作流节点等）：空闲 2 分钟后按看到的事件收尾。
func TestTraceProjector_NonRunnerEventClosedWhenIdle(t *testing.T) {
	var got trace.TraceCompletion
	repo := &mockTraceWriter{updateMonitorTraceCompletionFn: func(_ context.Context, _ string, c trace.TraceCompletion) error {
		got = c
		return nil
	}}
	p := newTestTraceProjector(repo)
	ctx := context.Background()
	p.HandleExposed(ctx, flowEvent("t-skill", "skill", "skill.watch.reload", "error"))
	p.CloseIdleTracesExposed(ctx)
	if p.TraceCount() != 1 {
		t.Fatal("a trace that just had activity must not be closed")
	}
	p.AgeTraceForTest("t-skill", 3*time.Minute)
	p.CloseIdleTracesExposed(ctx)
	if p.TraceCount() != 0 || got.Status != "error" {
		t.Fatalf("status=%q count=%d, want closed as error", got.Status, p.TraceCount())
	}
}
