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
// It reads activities via ActivityReader and updates each in-flight entry
// through ActivityWriter, avoiding the legacy ChatMessage adapter layer
// (which is a NoopWriter in the new persistence path).
//
// The returned count is the number of activities successfully transitioned
// to cancelled status.
func CancelRunningActivityMessages(
	ctx context.Context,
	reader biz.ActivityReader,
	writer biz.ActivityWriter,
	sessionID string,
	lg loggateway.Logger,
) (int, error) {
	if isNilInterface(reader) || isNilInterface(writer) {
		return 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	acts, err := reader.ListBySession(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	cancelled := 0
	for i := range acts {
		a := acts[i]
		if !isInFlightActivity(a.Status) {
			continue
		}
		// Validate transition via state machine (AS-FSM-01).
		// Running/ToolRunning/ToolBlocked all support cancel → Cancelled.
		newStatus, err := biz.TransitionActivityStatus(a.Status, biz.ActivityTransitionCancel)
		if err != nil {
			// Illegal transition — skip silently to avoid blocking the cancel loop.
			// This can happen if the activity transitioned to a terminal state
			// between ListBySession and UpdateActivity (race).
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
