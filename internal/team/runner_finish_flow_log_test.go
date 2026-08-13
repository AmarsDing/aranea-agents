package team

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// captureFlowBus captures MonitorEvents for flow-log assertions (team-local
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

func (b *captureFlowBus) hasFlowError(stepID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		step, _ := ev.Metadata["step_id"].(string)
		sev, _ := ev.Metadata["severity"].(string)
		if step == stepID && sev == "error" {
			return true
		}
	}
	return false
}

// S1（K2 流程日志覆盖）：finishRunErr 把 run 收敛为 failed，但此前只写进程
// 日志——「流程日志」Tab 上看不到团队任务的失败原因，业务用户无法排障。
// 失败终态必须发射 LogError（复用已注册步骤 team.run.finish）。
func TestFinishRunErr_EmitsFlowLogError(t *testing.T) {
	runRepo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	runner := &Runner{
		runReader:       runRepo,
		runWriter:       runRepo,
		runTransitioner: &gateRunTransitioner{repo: runRepo},
		lg:              loggateway.NewNoop(),
	}

	_, run, _, _ := gateTestFixture()
	runRepo.runs[run.ID] = run

	monBus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: monBus}, event.TraceContext{
		TraceID:   "tr-s1",
		SessionID: run.SessionID,
		RunID:     run.ID,
		Domain:    event.TraceDomainTeam,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	runner.finishRunErr(ctx, &run, time.Now(), "provider boom")

	if !monBus.hasFlowError("team.run.finish") {
		t.Fatalf("expected flow_log error entry for step team.run.finish, got %+v", monBus.evs)
	}
}

// 成功终态不得出现 error 级流程日志（防止 LogError 被无条件发射）。
func TestFinishRunErr_TerminalRunSkipsFlowLogError(t *testing.T) {
	runRepo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	runner := &Runner{
		runReader:       runRepo,
		runWriter:       runRepo,
		runTransitioner: &gateRunTransitioner{repo: runRepo},
		lg:              loggateway.NewNoop(),
	}

	_, run, _, _ := gateTestFixture()
	run.Status = biz.TeamRunStatusSuccess // 已达终态：finishRunErr 直接返回
	runRepo.runs[run.ID] = run

	monBus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: monBus}, event.TraceContext{
		TraceID:   "tr-s1b",
		SessionID: run.SessionID,
		RunID:     run.ID,
		Domain:    event.TraceDomainTeam,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	runner.finishRunErr(ctx, &run, time.Now(), "late error")

	if monBus.hasFlowError("team.run.finish") {
		t.Fatal("terminal run must not emit a flow-log error (guard returned early)")
	}
}
