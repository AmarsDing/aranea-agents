package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
)

// DEV-05: the alert firing state machine lives in the alert subpackage.
// These aliases keep the historical monitor.* API surface intact for
// consumers (biz facade, data layer, service layer, tests).

type AlertFiringState = alert.AlertFiringState

const (
	AlertFiringStateIdle      = alert.AlertFiringStateIdle
	AlertFiringStateFiring    = alert.AlertFiringStateFiring
	AlertFiringStateRecovered = alert.AlertFiringStateRecovered
)

type AlertFiringEvent = alert.AlertFiringEvent

const (
	AlertEventThresholdExceeded = alert.AlertEventThresholdExceeded
	AlertEventRecovered         = alert.AlertEventRecovered
	AlertEventReset             = alert.AlertEventReset
)

var (
	TransitionAlertFiringState     = alert.TransitionAlertFiringState
	MustTransitionAlertFiringState = alert.MustTransitionAlertFiringState
)
