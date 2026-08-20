// Package alert implements the monitor alert domain: rule state machine,
// metric registry/ring buffer, evaluation engine, and eval worker.
package alert

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

// 保留理由（2026-08-20 P2-2 盘点）：不迁移到 biz/shared.GenericStateMachine——
// 非法转换须返回 apierror.BadRequest(ALERT_STATE/ALERT_TRANSITION) 保证 400 映射；
// current=="" 归一化为 idle（MON-OPT-02 迁移前存量数据）与 Must 测试变体为
// shared 未覆盖特性。AS-FSM-01 合规：显式转换表。

// AlertFiringState is the alert state machine value (MON-OPT-02).
type AlertFiringState string

const (
	AlertFiringStateIdle      AlertFiringState = "idle"
	AlertFiringStateFiring    AlertFiringState = "firing"
	AlertFiringStateRecovered AlertFiringState = "recovered"
)

// AlertFiringEvent represents events that trigger state transitions in the alert firing lifecycle.
type AlertFiringEvent string

const (
	AlertEventThresholdExceeded AlertFiringEvent = "threshold_exceeded"
	AlertEventRecovered         AlertFiringEvent = "recovered"
	AlertEventReset             AlertFiringEvent = "reset"
)

// alertFiringTransitions defines the legal state transitions for the alert firing state machine.
// Key: current state → event → next state.
var alertFiringTransitions = map[AlertFiringState]map[AlertFiringEvent]AlertFiringState{
	AlertFiringStateIdle: {
		AlertEventThresholdExceeded: AlertFiringStateFiring,
	},
	AlertFiringStateFiring: {
		AlertEventRecovered: AlertFiringStateRecovered,
		AlertEventReset:     AlertFiringStateIdle,
	},
	AlertFiringStateRecovered: {
		AlertEventThresholdExceeded: AlertFiringStateFiring,
		AlertEventReset:             AlertFiringStateIdle,
	},
}

// TransitionAlertFiringState validates and returns the next state for a given (current, event) pair.
// Returns an error if the transition is not allowed.
// An empty current state is treated as idle (zero-value default for rules not yet persisted).
func TransitionAlertFiringState(current AlertFiringState, event AlertFiringEvent) (AlertFiringState, error) {
	// Normalize empty string to idle (rules loaded from DB before MON-OPT-02 migration)
	if current == "" {
		current = AlertFiringStateIdle
	}
	events, ok := alertFiringTransitions[current]
	if !ok {
		return current, apierror.BadRequest("ALERT_STATE", "unknown alert firing state: %s", current)
	}
	next, ok := events[event]
	if !ok {
		return current, apierror.BadRequest("ALERT_TRANSITION", "invalid transition: %s + %s", current, event)
	}
	return next, nil
}

// MustTransitionAlertFiringState is like TransitionAlertFiringState but panics on invalid transition.
// Use only in tests.
func MustTransitionAlertFiringState(current AlertFiringState, event AlertFiringEvent) AlertFiringState {
	next, err := TransitionAlertFiringState(current, event)
	if err != nil {
		panic(fmt.Sprintf("invalid alert state transition: %v", err))
	}
	return next
}
