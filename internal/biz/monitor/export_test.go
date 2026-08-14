package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
)

func RecoveryThreshold(rule AlertRule) float64 {
	return alert.RecoveryThreshold(rule)
}
