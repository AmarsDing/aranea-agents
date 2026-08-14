package monitor

import (
	"context"

	"aranea-agents/internal/biz/monitor/alert"
)

// DEV-05: the alert metric catalog lives in the alert subpackage; the alias
// keeps the historical monitor.* API surface intact.
type AlertMetricCatalogEntry = alert.AlertMetricCatalogEntry

// ListAlertMetricCatalog returns the alert metric directory for the Alerts
// page: every registered metric with human-readable metadata plus its
// current value. Delegates to the alert engine (DEV-05).
func (u *Usecase) ListAlertMetricCatalog(ctx context.Context) []AlertMetricCatalogEntry {
	if u == nil || u.engine == nil {
		return nil
	}
	return u.engine.ListAlertMetricCatalog(ctx)
}
