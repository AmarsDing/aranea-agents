package server

import (
	"aranea-agents/pkg/auth"
	"encoding/json"
	nethttp "net/http"
)

// readinessFilter returns an HTTP filter that rejects all requests except
// infrastructure routes (/healthz, /metrics) when the server is not ready.
// This allows the HTTP server to start listening immediately while P1
// migrations run in the background, so /healthz can properly report
// "starting" (503) instead of the connection being refused entirely.
func readinessFilter(readiness ReadinessProbe) func(next nethttp.Handler) nethttp.Handler {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			if readiness == nil || readiness.IsReady() {
				next.ServeHTTP(w, r)
				return
			}

			switch r.URL.Path {
			case "/healthz", "/metrics":
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(nethttp.StatusServiceUnavailable)
			status := "starting"
			reason := ""
			if readiness.IsFailed() {
				status = "failed"
				reason = readiness.FailedReason()
			}
			resp := map[string]string{"status": status, "auth_mode": auth.HealthAuthInfo().AuthMode}
			if reason != "" {
				resp["reason"] = reason
			}
			_ = json.NewEncoder(w).Encode(resp)
		})
	}
}
