package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// CallbackDuration tracks callback handler latency.
var CallbackDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "aranea_callback_duration_seconds",
	Help:    "Latency of callback invocations by source and lifecycle point.",
	Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
}, []string{"source", "point"})

// CallbackErrorTotal counts callback failures by source, point, and reason.
var CallbackErrorTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aranea_callback_error_total",
	Help: "Number of callback errors by source, point, and reason.",
}, []string{"source", "point", "reason"})

// ObserveCallback records duration and optional error for a callback invocation.
func ObserveCallback(source, point string, start time.Time, err error) {
	CallbackDuration.WithLabelValues(source, point).Observe(time.Since(start).Seconds())
	if err == nil {
		return
	}
	reason := "error"
	if IsBlockedErr(err) {
		reason = "blocked"
	}
	CallbackErrorTotal.WithLabelValues(source, point, reason).Inc()
}

// IsBlockedErr reports hook/policy block errors (best-effort string match).
func IsBlockedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "HOOK_BLOCKED", "blocked", "FORBIDDEN")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
