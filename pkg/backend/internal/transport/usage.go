package transport

import (
	"net/http"
	"strconv"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handleModelUsageOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	out, err := h.usageSvc.Overview(modelUsageQueryFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *HTTPHandler) handleModelUsageTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	out, err := h.usageSvc.Trends(modelUsageQueryFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ModelUsageTrendPoint]{Items: out})
}

func (h *HTTPHandler) handleModelUsageTopModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	out, err := h.usageSvc.TopModels(modelUsageQueryFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ModelUsageBreakdownRow]{Items: out})
}

func (h *HTTPHandler) handleModelUsageTopAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	out, err := h.usageSvc.TopAgents(modelUsageQueryFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ModelUsageBreakdownRow]{Items: out})
}

func (h *HTTPHandler) handleModelUsageEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	out, err := h.usageSvc.Events(modelUsageQueryFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ModelTokenUsageEvent]{Items: out})
}

func modelUsageQueryFromRequest(r *http.Request) domain.ModelUsageQuery {
	values := r.URL.Query()
	limit, _ := strconv.Atoi(values.Get("limit"))
	return domain.ModelUsageQuery{
		Range:        values.Get("range"),
		StartDate:    values.Get("start_date"),
		EndDate:      values.Get("end_date"),
		ProviderCode: values.Get("provider_code"),
		ModelAPIID:   values.Get("model_api_id"),
		AgentID:      values.Get("agent_id"),
		Status:       values.Get("status"),
		Limit:        limit,
	}
}
