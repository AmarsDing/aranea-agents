package monitor

import (
	"context"
	"encoding/json"

	"aranea-agents/pkg/apierror"
)

// DiagnoseAndHealResult is the unified result from either observer or legacy usecase.
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
	RootCauseCondition  *RootCauseConditionResult
}

// RootCauseConditionResult describes the root cause condition for a heal result.
type RootCauseConditionResult struct {
	AutoHealed   *AutoHealedResult
	HealAttempts *HealAttemptsResult
	SelfCheck    *SelfCheckResult
}

// AutoHealedResult describes an auto-healed condition.
type AutoHealedResult struct {
	AutoHealed   bool
	HealStrategy string
}

// HealAttemptsResult describes heal attempts condition.
type HealAttemptsResult struct {
	Attempts     int
	MaxAttempts  int
	LastStrategy string
}

// SelfCheckResult describes a self-check condition.
type SelfCheckResult struct {
	CheckName string
	Status    string
	Message   string
}

// DiagnoseAndHeal runs the diagnose-and-heal workflow, preferring the observer
// (new path) and falling back to the legacy SelfHealUsecase (deprecated).
func (u *Usecase) DiagnoseAndHeal(ctx context.Context, observer *SelfHealObserver, legacy *SelfHealUsecase, traceID, sessionID, runID, stepID, triggerType string, contextMinutes int32) (*DiagnoseAndHealResult, error) {
	// Prefer SelfHealObserver (new path)
	if observer != nil {
		rec, err := observer.DiagnoseAndObserve(ctx,
			traceID, sessionID, stepID,
			triggerType, contextMinutes,
		)
		if err != nil {
			return nil, apierror.Internal("MONITOR", err.Error())
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

	// Deprecated fallback: SelfHealUsecase
	if legacy == nil {
		return nil, apierror.Unavailable("MONITOR", "self-heal service not available")
	}
	rec, err := legacy.DiagnoseAndHeal(ctx,
		traceID, sessionID, runID, stepID,
		triggerType, contextMinutes,
	)
	if err != nil {
		return nil, apierror.Internal("MONITOR", err.Error())
	}

	result := &DiagnoseAndHealResult{
		HealID:              rec.ID,
		RuleID:              rec.RuleID,
		Status:              rec.Status,
		Reason:              rec.Reason,
		Confidence:          rec.Confidence,
		FixAction:           rec.FixAction,
		RuntimeAutoHealed:   rec.RuntimeAutoHealed,
		RuntimeHealAttempts: rec.RuntimeHealAttempts,
		CreatedAt:           rec.CreatedAt,
	}

	// Populate RootCauseCondition based on heal result.
	switch rec.Status {
	case string(HealStatusApplied):
		result.RootCauseCondition = &RootCauseConditionResult{
			AutoHealed: &AutoHealedResult{
				AutoHealed:   true,
				HealStrategy: rec.FixAction.Type,
			},
		}
	case string(HealStatusSkippedLowConfidence), string(HealStatusSkippedCooldown), string(HealStatusSkippedNoAction):
		result.RootCauseCondition = &RootCauseConditionResult{
			HealAttempts: &HealAttemptsResult{
				Attempts:     0,
				MaxAttempts:  rec.FixAction.MaxAttempts,
				LastStrategy: rec.FixAction.Type,
			},
		}
	case string(HealStatusFailed):
		result.RootCauseCondition = &RootCauseConditionResult{
			SelfCheck: &SelfCheckResult{
				CheckName: rec.RuleID,
				Status:    "failed",
				Message:   rec.Reason,
			},
		}
	}

	return result, nil
}

// DiagnoseAndHealFixParamsJSON returns the JSON encoding of the fix action params.
func DiagnoseAndHealFixParamsJSON(r *DiagnoseAndHealResult) string {
	if r == nil {
		return ""
	}
	b, _ := json.Marshal(r.FixAction.Params)
	return string(b)
}
