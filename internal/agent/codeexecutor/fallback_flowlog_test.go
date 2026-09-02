package codeexecutor

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// captureFlowBus captures MonitorEvents for flow-log assertions (package-local
// mirror of the graph test helper; event.captureMonitorBus is unexported).
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
		if step, _ := ev.Metadata["step_id"].(string); step == stepID {
			return true
		}
	}
	return false
}

// K3：真实降级点（docker→local）必须发射 codeexec.backend_fallback 流程日志。
func TestFallbackFlowLogEmittedOnDockerDegrade(t *testing.T) {
	stubDockerProbeHooks(t, false)
	t.Setenv("ARANEA_ENV", "")
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "degrade")
	f := NewFactoryWithLogger(loggateway.NewNoop())

	bus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: bus}, event.TraceContext{
		TraceID: "tr-83-fallback",
		Domain:  event.TraceDomainSkill,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	if exec := f.Resolve(ctx, TypeDocker, t.TempDir()); exec == nil {
		t.Fatal("degrade policy must keep docker→local fallback")
	}
	if !bus.hasFlowStep("codeexec.backend_fallback") {
		t.Fatalf("expected flow-log step codeexec.backend_fallback, got %+v", bus.evs)
	}
}

// nil-safe：ctx 无 TraceEmitter（启动期/非会话调用）不得 panic，降级照常。
func TestFallbackFlowLogNilEmitterNoPanic(t *testing.T) {
	stubDockerProbeHooks(t, false)
	t.Setenv("ARANEA_ENV", "")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	if exec := f.Resolve(context.Background(), TypeDocker, t.TempDir()); exec == nil {
		t.Fatal("degrade without emitter must still fall back to local")
	}
}
