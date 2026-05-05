package legacychat

import (
	"encoding/json"
	"net/http"
)

// WriteUpstreamUnavailableJSON responds when LEGACY_REST_ORIGIN is unset (until fully native chat executes in-process).
func WriteUpstreamUnavailableJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"reason":  "CHAT_UPSTREAM_NOT_CONFIGURED",
		"message": "Set LEGACY_REST_ORIGIN to an HTTP root that serves " + LegacyRoutePrefix + "/* until native chat runs without an upstream (see api/kratos/chat/v1).",
	})
}
