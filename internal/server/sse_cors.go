package server

import (
	"net/http"
	"strings"
)

// prepareSSEAccessControl handles OPTIONS preflight and sets Access-Control-Allow-Origin so browser
// EventSource can connect when the SPA origin differs from the SSE listener (e.g. separate API host).
// Returns false if the response is already complete (OPTIONS).
func prepareSSEAccessControl(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if r.Method == http.MethodOptions {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Cache-Control, Last-Event-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	return true
}
