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

func (h *HTTPHandler) handleTeams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.teamSvc.List()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.Team]{Items: items})
	case http.MethodPost:
		var in domain.Team
		if !decodeBody(w, r, &in) {
			return
		}
		created, err := h.teamSvc.Create(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "team", created.ID, r.Header.Get("X-Request-Id"), created.TeamKey)
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleTeamByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(idFromPath(r.URL.Path, "/api/v1/teams/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("team id is required"))
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "duplicate" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		created, err := h.teamSvc.Duplicate(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "team", created.ID, r.Header.Get("X-Request-Id"), "duplicate")
		writeJSON(w, http.StatusCreated, created)
		return
	}
	if len(parts) != 1 {
		methodNotAllowed(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		team, err := h.teamSvc.Get(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, team)
	case http.MethodPatch:
		var in domain.Team
		if !decodeBody(w, r, &in) {
			return
		}
		updated, err := h.teamSvc.Update(id, in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("update", "team", updated.ID, r.Header.Get("X-Request-Id"), updated.TeamKey)
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.teamSvc.Delete(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "team", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleTeamRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.teamSvc.ListRuns(r.URL.Query().Get("team_id"), intQueryParam(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.TeamRun]{Items: items})
}

func (h *HTTPHandler) handleTeamRunByID(w http.ResponseWriter, r *http.Request) {
	runID := strings.Trim(idFromPath(r.URL.Path, "/api/v1/team-runs/"), "/")
	if runID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("team run id is required"))
		return
	}
	parts := strings.Split(runID, "/")
	if len(parts) == 2 && parts[1] == "steps" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		items, err := h.teamSvc.ListRunSteps(parts[0])
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.TeamRunStep]{Items: items})
		return
	}
	methodNotAllowed(w)
}

func (h *HTTPHandler) handleTeamRunEvents(w http.ResponseWriter, r *http.Request) {
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

	events, unsubscribe := h.chatSvc.SubscribeTeamRunEvents(r.URL.Query().Get("team_id"))
	defer unsubscribe()

	_, _ = fmt.Fprint(w, ": connected\n\n")
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
		case event, ok := <-events:
			if !ok {
				return
			}
			raw, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
			flusher.Flush()
		}
	}
}
