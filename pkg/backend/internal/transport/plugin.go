package transport

import (
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	page := intQueryParam(r, "page", 1)
	pageSize := intQueryParam(r, "page_size", 20)
	result, err := h.pluginSvc.List(domain.PluginListQuery{
		Search:        r.URL.Query().Get("search"),
		Category:      r.URL.Query().Get("category"),
		Enabled:       r.URL.Query().Get("enabled"),
		CallbackPoint: r.URL.Query().Get("callback_point"),
		Limit:         pageSize,
		Offset:        (page - 1) * pageSize,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, paginatedResponse[domain.Plugin]{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total})
}

func (h *HTTPHandler) handlePluginByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/plugins/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("plugin id and action are required"))
		return
	}
	id := parts[0]
	switch {
	case r.Method == http.MethodPatch && parts[1] == "enabled":
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		updated, err := h.pluginSvc.ToggleEnabled(id, body.Enabled)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case r.Method == http.MethodPut && parts[1] == "config":
		var body domain.PluginConfigUpdate
		if !decodeBody(w, r, &body) {
			return
		}
		updated, err := h.pluginSvc.UpdateConfig(id, body.ConfigJSON)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		methodNotAllowed(w)
	}
}
