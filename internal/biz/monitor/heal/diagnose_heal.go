package heal

import (
	"context"
	"encoding/json"

	"aranea-agents/pkg/apierror"
)

// DiagnoseAndHealResult is the result from the observer diagnose-and-observe path.
type DiagnoseAndHealResult struct {
	HealID              string
	RuleID              string
	Status              string
	Reason              string
	Confidence          float64
	FixAction           FixAction
	RuntimeAutoHealed   bool
	RuntimeHealAttempts int
	CreatedAt           string
}

// DiagnoseAndHeal runs the diagnose-and-heal workflow via the observer.
// DEV-05: converted from a monitor.Usecase method to a free function — the
// receiver was never used; the workflow only needs the heal observer.
//
// ADR-4（2026-08-20）：legacy SelfHealUsecase fallback 已删除，observer 为唯一
// 路径（运行时负责真正的修复执行，observer 只观测与告警）。runID 不再下传——
// observer 路径不生成诊断包，无需该参数。
func DiagnoseAndHeal(ctx context.Context, observer *SelfHealObserver, traceID, sessionID, stepID, triggerType string, contextMinutes int32) (*DiagnoseAndHealResult, error) {
	if observer == nil {
		return nil, apierror.Unavailable("MONITOR", "self-heal observer not available")
	}
	rec, err := observer.DiagnoseAndObserve(ctx,
		traceID, sessionID, stepID,
		triggerType, contextMinutes,
	)
	if err != nil {
		return nil, err
	}
	return &DiagnoseAndHealResult{
		HealID:              rec.ID,
		RuleID:              rec.RuleID,
		Status:              rec.Status,
		Reason:              rec.Reason,
		Confidence:          rec.Confidence,
		FixAction:           rec.FixAction,
		RuntimeAutoHealed:   rec.RuntimeAutoHealed,
		RuntimeHealAttempts: rec.RuntimeHealAttempts,
		CreatedAt:           rec.CreatedAt,
	}, nil
}

// DiagnoseAndHealFixParamsJSON returns the JSON encoding of the fix action params.
func DiagnoseAndHealFixParamsJSON(r *DiagnoseAndHealResult) string {
	if r == nil {
		return ""
	}
	b, _ := json.Marshal(r.FixAction.Params)
	return string(b)
}
