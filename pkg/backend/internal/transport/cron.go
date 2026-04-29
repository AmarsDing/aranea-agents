package transport

import (
	"net/http"
	"strconv"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handleCronTaskRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := domain.CronTaskRunQuery{
		TaskID: r.URL.Query().Get("cron_task_id"),
		Status: r.URL.Query().Get("status"),
	}
	if limit, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		query.Limit = limit
	}
	items, err := h.platformSvc.ListCronTaskRuns(query)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.CronTaskRun]{Items: items})
}
