package chatactivity

import (
	"context"
	"reflect"
	"strings"

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

// isInFlightActivity returns true for statuses that represent an unfinished
// activity which can be cancelled (running / tool_running / tool_blocked).
// Terminal statuses (completed/failed/cancelled/interrupted) are skipped.
func isInFlightActivity(s biz.ActivityStatus) bool {
	switch s {
	case biz.ActivityStatusRunning,
		biz.ActivityStatusToolRunning,
		biz.ActivityStatusToolBlocked:
		return true
	default:
		return false
	}
}

// CancelRunningActivityMessages marks in-flight activity cards (running /
// tool_running / tool_blocked) as cancelled when the user stops generation.
//
// Phase 3b-D Task 7: reads via v2 StepV2Reader.ListStepsBySpiritSession (spirit
// root aggregation for Stop) with ListStepsBySession fallback (member-only
// cancel), and converts each Step to the v1 Activity shape for the v1
// ActivityWriter (writer migration is Tier 3). The v1 ActivityWriter is
// retained because cancel persists status transitions via the v1 write path.
//
// The returned count is the number of activities successfully transitioned
// to cancelled status.
func CancelRunningActivityMessages(
	ctx context.Context,
	stepReader biz.StepV2Reader,
	writer biz.ActivityWriter,
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
	for i := range steps {
		a := biz.StepToActivity(steps[i])
		if !isInFlightActivity(a.Status) {
			continue
		}
		// Validate transition via state machine (AS-FSM-01).
		// Running/ToolRunning/ToolBlocked all support cancel → Cancelled.
		newStatus, err := biz.TransitionActivityStatus(a.Status, biz.ActivityTransitionCancel)
		if err != nil {
			// Illegal transition — skip silently to avoid blocking the cancel loop.
			// This can happen if the activity transitioned to a terminal state
			// between ListStepsBySession and UpdateActivity (race).
			lg.Debug("跳过非法状态转换",
				loggateway.StepID("chat.activity.cancel"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("activity_id", a.ID),
				loggateway.Str("current_status", string(a.Status)),
			)
			continue
		}
		a.Status = newStatus
		if _, err := writer.UpdateActivity(ctx, a); err != nil {
			lg.Warn("取消执行卡片落库失败",
				loggateway.StepID("chat.activity.cancel"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("activity_id", a.ID),
				loggateway.Err(err))
			continue
		}
		cancelled++
	}
	return cancelled, nil
}
