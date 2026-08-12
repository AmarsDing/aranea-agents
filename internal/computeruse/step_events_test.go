package computeruse

import (
	"context"
	"testing"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/event/contract"
)

// captureMonitorBus 收集 Publish 的 MonitorEvent（contract.MonitorBus 最小实现）。
type captureMonitorBus struct{ evs []contract.MonitorEvent }

func (b *captureMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.evs = append(b.evs, ev)
}

func (b *captureMonitorBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *captureMonitorBus) DropCount() uint64 { return 0 }

func sampleStep(result bizcu.StepResult) bizcu.Step {
	return bizcu.Step{
		SessionID:   "cu-1",
		AgentKey:    "agent__general",
		Index:       3,
		Target:      "保存菜单项",
		Path:        bizcu.PathA11y,
		Action:      bizcu.ActionClick,
		Result:      result,
		DurationMs:  12,
		ConfirmedBy: "user-1",
		Danger:      true,
		CreatedAt:   time.Now(),
	}
}

// 75 M1.4：每步动作发布 computeruse.step MonitorEvent，载荷含审计摘要字段。
func TestStepEventPublisher_PublishesMonitorEvent(t *testing.T) {
	bus := &captureMonitorBus{}
	p := NewStepEventPublisher(bus)

	p.PublishStep(context.Background(), sampleStep(bizcu.StepOK))

	if len(bus.evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.evs))
	}
	ev := bus.evs[0]
	if ev.Type != contract.MonitorEventTypeComputerUseStep {
		t.Errorf("Type = %q, want %q", ev.Type, contract.MonitorEventTypeComputerUseStep)
	}
	if ev.SessionID != "cu-1" {
		t.Errorf("SessionID = %q, want cu-1", ev.SessionID)
	}
	if ev.Level != "info" {
		t.Errorf("Level = %q, want info", ev.Level)
	}
	if ev.Source == "" {
		t.Error("Source must be set")
	}
	md := ev.Metadata
	want := map[string]any{
		"agent_key":    "agent__general",
		"step_index":   3,
		"target":       "保存菜单项",
		"path":         "a11y",
		"action":       "click",
		"result":       "ok",
		"duration_ms":  int64(12),
		"confirmed_by": "user-1",
		"danger":       true,
	}
	for k, v := range want {
		if md[k] != v {
			t.Errorf("Metadata[%q] = %v, want %v", k, md[k], v)
		}
	}
}

// 级别映射：failed→error，retry/cancelled→warn，ok/dry_run→info。
func TestStepEventPublisher_LevelByResult(t *testing.T) {
	cases := map[bizcu.StepResult]string{
		bizcu.StepOK:        "info",
		bizcu.StepDryRun:    "info",
		bizcu.StepRetry:     "warn",
		bizcu.StepCancelled: "warn",
		bizcu.StepFailed:    "error",
	}
	for result, wantLevel := range cases {
		bus := &captureMonitorBus{}
		p := NewStepEventPublisher(bus)
		st := sampleStep(result)
		if result == bizcu.StepFailed {
			st.Error = "boom"
		}
		p.PublishStep(context.Background(), st)
		if len(bus.evs) != 1 {
			t.Fatalf("%s: expected 1 event, got %d", result, len(bus.evs))
		}
		if bus.evs[0].Level != wantLevel {
			t.Errorf("%s: Level = %q, want %q", result, bus.evs[0].Level, wantLevel)
		}
		if result == bizcu.StepFailed && bus.evs[0].Metadata["error"] != "boom" {
			t.Errorf("failed step must carry error metadata, got %v", bus.evs[0].Metadata["error"])
		}
	}
}

// nil bus（测试/装配缺失）不得 panic。
func TestStepEventPublisher_NilBusSafe(t *testing.T) {
	p := NewStepEventPublisher(nil)
	p.PublishStep(context.Background(), sampleStep(bizcu.StepOK))
}
