package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
	"aranea-agents/pkg/loggateway"
)

// DEV-05: AlertEvalWorker lives in the alert subpackage and depends on
// *alert.Engine (not the root Usecase), which eliminates the historical
// circular dependency. The alias keeps the monitor.* API surface intact.
type AlertEvalWorker = alert.AlertEvalWorker

// NewAlertEvalWorker preserves the historical constructor signature: it
// resolves the usecase's alert engine and delegates to the alert subpackage.
// A nil usecase yields nil (legacy contract relied upon by callers/tests).
func NewAlertEvalWorker(uc *Usecase, buffer *MetricRingBuffer, lg loggateway.Logger) *AlertEvalWorker {
	if uc == nil {
		return nil
	}
	return alert.NewAlertEvalWorker(uc.alertEngine(), buffer, lg)
}
