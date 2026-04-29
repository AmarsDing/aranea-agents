package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"arenea/backend/internal/domain"
)

type monitorTraceDetail struct {
	Trace    domain.PlatformResource `json:"trace"`
	Config   map[string]any          `json:"config"`
	Metadata map[string]any          `json:"metadata"`
	Spans    []any                   `json:"spans"`
}

type monitorLogLine struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

func (h *HTTPHandler) handleMonitorAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.auditSvc.List(intQueryParam(r, "limit", 200))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.AuditLog]{Items: items})
}

func (h *HTTPHandler) handleMonitorEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.platformSvc.List("monitor-events")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.PlatformResource]{Items: sanitizePlatformResources(items)})
}

func (h *HTTPHandler) handleMonitorEventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	id := idFromPath(r.URL.Path, "/api/v1/monitor/events/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("event id is required"))
		return
	}
	item, err := h.platformSvc.Get("monitor-events", id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, sanitizePlatformResource(item))
}

func (h *HTTPHandler) handleMonitorTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.platformSvc.List("monitor-traces")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.PlatformResource]{Items: sanitizePlatformResources(items)})
}

func (h *HTTPHandler) handleMonitorTraceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	id := idFromPath(r.URL.Path, "/api/v1/monitor/traces/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("trace id is required"))
		return
	}
	item, err := h.platformSvc.Get("monitor-traces", id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	config := parseJSONMap(item.ConfigJSON)
	metadata := parseJSONMap(item.MetadataJSON)
	writeJSON(w, http.StatusOK, monitorTraceDetail{
		Trace:    sanitizePlatformResource(item),
		Config:   config,
		Metadata: metadata,
		Spans:    traceSpans(config),
	})
}

func (h *HTTPHandler) handleMonitorLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	items := []monitorLogLine{{
		ID:        "logs-not-configured",
		Time:      now,
		Level:     "INFO",
		Message:   "structured monitor APIs are available; process log streaming is not configured",
		Source:    "monitor",
		CreatedAt: now,
	}}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":   items,
		"enabled": false,
		"message": "log stream is not configured",
	})
}

func (h *HTTPHandler) handleMonitorLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	now := time.Now().UTC().Format(time.RFC3339)
	line := monitorLogLine{
		ID:        "logs-not-configured",
		Time:      now,
		Level:     "INFO",
		Message:   "log stream endpoint is reserved; no process log source configured",
		Source:    "monitor",
		CreatedAt: now,
	}
	raw, _ := json.Marshal(line)
	_, _ = fmt.Fprint(w, ": connected\n\n")
	_, _ = fmt.Fprintf(w, "event: log\n")
	_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func sanitizePlatformResources(items []domain.PlatformResource) []domain.PlatformResource {
	out := make([]domain.PlatformResource, 0, len(items))
	for _, item := range items {
		out = append(out, sanitizePlatformResource(item))
	}
	return out
}

func sanitizePlatformResource(item domain.PlatformResource) domain.PlatformResource {
	item.ConfigJSON = sanitizeJSONString(item.ConfigJSON)
	item.MetadataJSON = sanitizeJSONString(item.MetadataJSON)
	return item
}

func sanitizeJSONString(raw string) string {
	parsed := parseJSONMap(raw)
	if len(parsed) == 0 {
		return raw
	}
	sanitized := sanitizeJSONValue(parsed)
	out, err := json.Marshal(sanitized)
	if err != nil {
		return raw
	}
	return string(out)
}

func parseJSONMap(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]any{}
	}
	return sanitizeJSONValue(parsed).(map[string]any)
}

func sanitizeJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if isSensitiveKey(key) {
				out[key] = "******"
				continue
			}
			out[key] = sanitizeJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, sanitizeJSONValue(child))
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, token := range []string{"api_key", "apikey", "token", "secret", "password", "authorization", "cookie"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func traceSpans(config map[string]any) []any {
	if spans, ok := config["spans"].([]any); ok {
		return spans
	}
	if trace, ok := config["trace"].(map[string]any); ok {
		if spans, ok := trace["spans"].([]any); ok {
			return spans
		}
	}
	return []any{}
}
