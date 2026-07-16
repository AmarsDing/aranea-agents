package chatactivity

import (
	"context"
	"reflect"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// isNilInterface checks whether an interface value is nil, including typed-nil
// pointers (e.g. (*SessionUsecase)(nil) stored in an interface).
func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

// isInFlightStep returns true for statuses that represent an unfinished
// step which can be cancelled (running / tool_running / tool_blocked).
func isInFlightStep(s biz.StepStatus) bool {
	switch s {
	case biz.StepStatusRunning,
		biz.StepStatusToolRunning,
		biz.StepStatusToolBlocked:
		return true
	default:
		return false
	}
}

// CancelRunningActivityMessages marks in-flight steps (running /
// tool_running / tool_blocked) as cancelled when the user stops generation.
//
// Reads and writes go through steps_v2 (StepV2Reader / StepV2Writer). The
// function name is retained for call-site stability.
func CancelRunningActivityMessages(
	ctx context.Context,
	stepReader biz.StepV2Reader,
	writer biz.StepV2Writer,
	sessionID string,
	lg loggateway.Logger,
) (int, error) {
	if isNilInterface(stepReader) || isNilInterface(writer) {
		return 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	steps, err := stepReader.ListStepsBySpiritSession(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	// Member-only cancel: SpiritSessionID filter returns empty when sessionID is
	// a child chat session; fall back to SessionID column.
	if len(steps) == 0 {
		steps, err = stepReader.ListStepsBySession(ctx, sessionID)
		if err != nil {
			return 0, err
		}
	}
	cancelled := 0
	now := time.Now().UTC()
	for i := range steps {
		st := steps[i]
		if !isInFlightStep(st.Status) {
			continue
		}
		// Reuse Activity status machine — StepStatus shares the same string values.
		newStatus, err := biz.TransitionActivityStatus(biz.ActivityStatus(st.Status), biz.ActivityTransitionCancel)
		if err != nil {
			lg.Debug("跳过非法状态转换",
				loggateway.StepID("chat.activity.cancel"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("step_id", st.ID),
				loggateway.Str("current_status", string(st.Status)),
			)
			continue
		}
		st.Status = biz.StepStatus(newStatus)
		st.CompletedAt = &now
		if _, err := writer.UpdateStep(ctx, st); err != nil {
			lg.Warn("取消执行卡片落库失败",
				loggateway.StepID("chat.activity.cancel"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("step_id", st.ID),
				loggateway.Err(err))
			continue
		}
		cancelled++
	}
	return cancelled, nil
}
