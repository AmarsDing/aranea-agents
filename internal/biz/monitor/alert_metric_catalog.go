package monitor

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
)

// AlertMetricCatalogEntry pairs catalog metadata with the metric's current
// value, evaluated at request time over its default window. CurrentValue is
// best-effort: evaluation errors are logged and leave the value at zero so a
// single broken metric does not fail the whole directory listing.
type AlertMetricCatalogEntry struct {
	AlertMetricInfo
	CurrentValue float64
	EvaluatedAt  time.Time
}

// ListAlertMetricCatalog returns the alert metric directory for the Alerts
// page: every registered metric with human-readable metadata plus its
// current value.
func (u *Usecase) ListAlertMetricCatalog(ctx context.Context) []AlertMetricCatalogEntry {
	if u == nil || u.registry == nil {
		return nil
	}
	infos := u.registry.ListCatalog()
	out := make([]AlertMetricCatalogEntry, 0, len(infos))
	now := time.Now().UTC()
	for _, info := range infos {
		entry := AlertMetricCatalogEntry{AlertMetricInfo: info, EvaluatedAt: now}
		m, ok := u.registry.Get(info.Key)
		if !ok {
			out = append(out, entry)
			continue
		}
		window := time.Duration(info.DefaultWindowMinutes) * time.Minute
		if window <= 0 {
			window = time.Hour
		}
		if v, err := m.Evaluate(ctx, window); err != nil {
			u.lg.Warn("ListAlertMetricCatalog: evaluate failed",
				loggateway.StepID("monitor.alert_metric_eval_fail"),
				loggateway.Str("metric_key", info.Key),
				loggateway.Err(err),
			)
		} else {
			entry.CurrentValue = v
		}
		out = append(out, entry)
	}
	return out
}
