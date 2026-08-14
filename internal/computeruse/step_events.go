package computeruse

import (
	"context"
	"fmt"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/event/contract"
)

// stepEventPublisher 实现 bizcu.StepEventPublisher：把每步审计摘要发布为
// computeruse.step MonitorEvent（MonitorBus → WS monitor pump → 前端步骤流）。
// 持久化由 computer_use_audit 表承担，事件仅作实时展示（Informational 级）。
type stepEventPublisher struct {
	bus contract.MonitorBus
}

// NewStepEventPublisher 构造；bus 为 nil 时返回空操作发布器（测试/装配缺失安全）。
func NewStepEventPublisher(bus contract.MonitorBus) bizcu.StepEventPublisher {
	if bus == nil {
		return stepEventPublisher{}
	}
	return stepEventPublisher{bus: bus}
}

// PublishStep 发布一步摘要；尽力而为，不阻断主流程。
func (p stepEventPublisher) PublishStep(ctx context.Context, step bizcu.Step) {
	if p.bus == nil {
		return
	}
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeComputerUseStep, "computeruse")
	ev.SessionID = step.SessionID
	ev.Level = stepEventLevel(step.Result)
	ev.Message = fmt.Sprintf("桌面动作 %s → %s（%s，%s，%dms）",
		step.Action, step.Target, step.Path, step.Result, step.DurationMs)
	ev.Metadata = map[string]any{
		"agent_key":      step.AgentKey,
		"step_index":     step.Index,
		"target":         step.Target,
		"path":           string(step.Path),
		"action":         string(step.Action),
		"result":         string(step.Result),
		"duration_ms":    step.DurationMs,
		"confirmed_by":   step.ConfirmedBy,
		"danger":         step.Danger,
		"degraded":       step.Degraded,
		"screenshot_ref": step.ScreenshotRef,
	}
	if step.Error != "" {
		ev.Metadata["error"] = step.Error
	}
	p.bus.Publish(ctx, ev)
}

// stepEventLevel 结果 → 事件级别：ok/dry_run→info，retry/cancelled→warn，failed→error。
func stepEventLevel(r bizcu.StepResult) string {
	switch r {
	case bizcu.StepFailed:
		return "error"
	case bizcu.StepRetry, bizcu.StepCancelled:
		return "warn"
	default:
		return "info"
	}
}
