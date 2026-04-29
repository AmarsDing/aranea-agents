package transport

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"arenea/backend/internal/domain"
)

type paginatedResponse[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

func (h *HTTPHandler) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	page, pageSize, offset := pageParams(r)
	result, err := h.skillSvc.Search(domain.SkillListQuery{
		Search:  r.URL.Query().Get("search"),
		Tags:    r.URL.Query().Get("tags"),
		Enabled: r.URL.Query().Get("enabled"),
		Status:  r.URL.Query().Get("status"),
		Limit:   pageSize,
		Offset:  offset,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, paginatedResponse[domain.Skill]{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total})
}

func (h *HTTPHandler) handleSkillImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	job, err := h.skillSvc.Import(r.Context(), file, header)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"job_id": job.JobID})
}

func (h *HTTPHandler) handleSkillImportByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(idFromPath(r.URL.Path, "/api/v1/skills/import/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("job id is required"))
		return
	}
	jobID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		job, err := h.skillSvc.GetImportJob(jobID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	if len(parts) == 2 && parts[1] == "apply" {
		h.handleSkillImportApply(w, r, jobID)
		return
	}
	if len(parts) == 4 && parts[1] == "conflict-groups" && parts[3] == "refine" {
		h.handleSkillConflictGroupRefine(w, r, jobID, parts[2])
		return
	}
	methodNotAllowed(w)
}

func (h *HTTPHandler) handleSkillImportApply(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in domain.SkillImportApplyRequest
	if !decodeBody(w, r, &in) {
		return
	}
	result, err := h.skillSvc.ApplyImport(jobID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("import", "skills", jobID, r.Header.Get("X-Request-Id"), result.Message)
	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleSkillConflictGroupRefine(w http.ResponseWriter, r *http.Request, jobID string, groupID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in domain.SkillRefineRequest
	if !decodeBody(w, r, &in) {
		return
	}
	result, err := h.skillSvc.RefineConflictGroup(r.Context(), jobID, groupID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleSkillByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(idFromPath(r.URL.Path, "/api/v1/skills/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("skill id is required"))
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := h.skillSvc.Delete(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "skills", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 2 {
		methodNotAllowed(w)
		return
	}
	switch parts[1] {
	case "enabled":
		h.handleSkillEnabled(w, r, id)
	case "duplicate":
		h.handleSkillDuplicate(w, r, id)
	case "files":
		h.handleSkillFiles(w, r, id)
	case "file":
		h.handleSkillFileContent(w, r, id)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleSkillEnabled(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	updated, err := h.skillSvc.ToggleEnabled(id, in.Enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("update", "skills", updated.ID, r.Header.Get("X-Request-Id"), "enabled")
	writeJSON(w, http.StatusOK, updated)
}

func (h *HTTPHandler) handleSkillDuplicate(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	created, err := h.skillSvc.Duplicate(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("create", "skills", created.ID, r.Header.Get("X-Request-Id"), created.Slug)
	writeJSON(w, http.StatusCreated, created)
}

func (h *HTTPHandler) handleSkillFiles(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	files, err := h.skillSvc.ListFiles(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.SkillFile]{Items: files})
}

func (h *HTTPHandler) handleSkillFileContent(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		content, err := h.skillSvc.ReadFile(id, r.URL.Query().Get("path"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, content)
	case http.MethodPut:
		var in domain.SkillFileUpdateInput
		if !decodeBody(w, r, &in) {
			return
		}
		content, err := h.skillSvc.UpdateFile(id, in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("update", "skills", id, r.Header.Get("X-Request-Id"), "file:"+in.Path)
		writeJSON(w, http.StatusOK, content)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleSkillRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	page, pageSize, offset := pageParams(r)
	result, err := h.skillSvc.SearchRuns(domain.SkillRunQuery{
		SkillID: r.URL.Query().Get("skill_id"),
		AgentID: r.URL.Query().Get("agent_id"),
		Status:  r.URL.Query().Get("status"),
		From:    r.URL.Query().Get("from"),
		To:      r.URL.Query().Get("to"),
		Limit:   pageSize,
		Offset:  offset,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, paginatedResponse[domain.SkillInvocation]{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total})
}

func pageParams(r *http.Request) (int, int, int) {
	page := intQueryParam(r, "page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := intQueryParam(r, "page_size", 20)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

func intQueryParam(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
