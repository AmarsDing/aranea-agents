package transport

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		query := parseSessionSearchQuery(r)
		result, err := h.sessionSvc.Search(query)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPost:
		var in domain.Session
		if !decodeBody(w, r, &in) {
			return
		}
		created, err := h.sessionSvc.Create(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "session", created.ID, r.Header.Get("X-Request-Id"), created.Title)
		writeJSON(w, http.StatusCreated, created)
	case http.MethodDelete:
		agentID := r.URL.Query().Get("agent_id")
		if agentID == "" {
			writeErr(w, http.StatusBadRequest, errors.New("agent_id is required"))
			return
		}
		if err := h.sessionSvc.DeleteByAgent(agentID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "sessions", agentID, r.Header.Get("X-Request-Id"), "clear history")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/sessions/")
	if strings.HasSuffix(id, "/timeline") {
		id = strings.TrimSuffix(id, "/timeline")
		if id == "" {
			writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		timeline, err := h.sessionSvc.Timeline(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, timeline)
		return
	}
	if strings.HasSuffix(id, "/archive") {
		id = strings.TrimSuffix(id, "/archive")
		if id == "" {
			writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if err := h.sessionSvc.Archive(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("archive", "session", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.HasSuffix(id, "/l0/snapshots") {
		sessionID := strings.TrimSuffix(id, "/l0/snapshots")
		h.memory.HandleL0Snapshots(w, r, sessionID)
		return
	}
	if strings.Contains(id, "/l1/") || strings.HasSuffix(id, "/l1/tasks") {
		sessionID, suffix := splitSessionPathSuffix(id, "/l1/")
		if sessionID == "" {
			writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
			return
		}
		h.memory.HandleL1Routes(w, r, sessionID, suffix)
		return
	}
	if strings.Contains(id, "/l2/") {
		sessionID, suffix := splitSessionPathSuffix(id, "/l2/")
		if sessionID == "" {
			writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
			return
		}
		h.memory.HandleL2Routes(w, r, sessionID, suffix)
		return
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		session, err := h.sessionSvc.Get(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, session)
	case http.MethodPatch:
		var in struct {
			Title string `json:"title"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if in.Title == "" {
			writeErr(w, http.StatusBadRequest, errors.New("title is required"))
			return
		}
		updated, err := h.sessionSvc.Rename(id, in.Title)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("update", "session", id, r.Header.Get("X-Request-Id"), in.Title)
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.sessionSvc.Delete(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "session", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func parseSessionSearchQuery(r *http.Request) domain.SessionSearchQuery {
	values := r.URL.Query()
	limit := parsePositiveInt(values.Get("limit"), 0)
	offset := parsePositiveInt(values.Get("offset"), 0)
	if pageSize := parsePositiveInt(values.Get("page_size"), 0); pageSize > 0 {
		limit = pageSize
		page := parsePositiveInt(values.Get("page"), 1)
		offset = (page - 1) * pageSize
	}
	return domain.SessionSearchQuery{
		OwnerType:     values.Get("owner_type"),
		AgentID:       values.Get("agent_id"),
		TeamID:        values.Get("team_id"),
		Status:        values.Get("status"),
		ContextStatus: values.Get("context_status"),
		Keyword:       values.Get("keyword"),
		Limit:         limit,
		Offset:        offset,
	}
}

// splitSessionPathSuffix 将形如 `<sessionID><sep><rest>` 的路径拆分为
// (sessionID, rest)。若缺少分隔符则返回 (id, "")。
// rest 会通过去除尾部斜杠规范化。
func splitSessionPathSuffix(id, sep string) (string, string) {
	idx := strings.Index(id, sep)
	if idx < 0 {
		return id, ""
	}
	return id[:idx], strings.Trim(id[idx+len(sep):], "/")
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
