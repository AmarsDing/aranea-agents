package http

import (
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

type toolListResponse struct {
	Items    []domain.Tool      `json:"items"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int                `json:"total"`
	Summary  domain.ToolSummary `json:"summary"`
}

type toolRunsPageResponse struct {
	Items    []domain.ToolInvocation `json:"items"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
	Total    int                     `json:"total"`
}

// ToolHTTP 提供 /api/v1/tools 与 /api/v1/tools/runs 的 HTTP 边界（迁移表 #27）。
type ToolHTTP struct {
	tool             *service.ToolService
	audit            *service.AuditService
	writeJSON        func(http.ResponseWriter, int, any)
	writeErr         func(http.ResponseWriter, int, error)
	decodeBody       func(http.ResponseWriter, *http.Request, any) bool
	methodNotAllowed func(http.ResponseWriter)
	pageParams       func(*http.Request) (int, int, int)
	idFromPath       func(path, prefix string) string
}

// NewToolHTTP 由 transport 注入与 catalog EvolutionHTTP 相同风格的辅助函数。
func NewToolHTTP(
	tool *service.ToolService,
	audit *service.AuditService,
	writeJSON func(http.ResponseWriter, int, any),
	writeErr func(http.ResponseWriter, int, error),
	decodeBody func(http.ResponseWriter, *http.Request, any) bool,
	methodNotAllowed func(http.ResponseWriter),
	pageParams func(*http.Request) (int, int, int),
	idFromPath func(path, prefix string) string,
) *ToolHTTP {
	return &ToolHTTP{
		tool:             tool,
		audit:            audit,
		writeJSON:        writeJSON,
		writeErr:         writeErr,
		decodeBody:       decodeBody,
		methodNotAllowed: methodNotAllowed,
		pageParams:       pageParams,
		idFromPath:       idFromPath,
	}
}

// Register 注册 /api/v1/tools 相关路由（顺序与 legacy transport 一致：runs → 集 → 项）。
func (t *ToolHTTP) Register(mux *http.ServeMux) {
	if t == nil {
		return
	}
	mux.HandleFunc("/api/v1/tools/runs", t.handleToolRuns)
	mux.HandleFunc("/api/v1/tools", t.handleTools)
	mux.HandleFunc("/api/v1/tools/", t.handleToolByID)
}

func (t *ToolHTTP) handleTools(w http.ResponseWriter, r *http.Request) {
	if t.tool == nil {
		t.writeErr(w, http.StatusServiceUnavailable, errors.New("tool service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, pageSize, offset := t.pageParams(r)
		result, err := t.tool.Search(domain.ToolListQuery{
			Search:    r.URL.Query().Get("search"),
			Category:  r.URL.Query().Get("category"),
			Source:    r.URL.Query().Get("source"),
			RiskLevel: r.URL.Query().Get("risk_level"),
			Enabled:   r.URL.Query().Get("enabled"),
			Limit:     pageSize,
			Offset:    offset,
		})
		if err != nil {
			t.writeErr(w, http.StatusBadRequest, err)
			return
		}
		t.writeJSON(w, http.StatusOK, toolListResponse{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total, Summary: result.Summary})
	case http.MethodPost:
		var in domain.ToolUpsertInput
		if !t.decodeBody(w, r, &in) {
			return
		}
		created, err := t.tool.Create(in)
		if err != nil {
			t.writeErr(w, http.StatusBadRequest, err)
			return
		}
		if t.audit != nil {
			_ = t.audit.Log("create", "tools", created.ID, r.Header.Get("X-Request-Id"), "tool.create")
		}
		t.writeJSON(w, http.StatusCreated, created)
	default:
		t.methodNotAllowed(w)
	}
}

func (t *ToolHTTP) handleToolByID(w http.ResponseWriter, r *http.Request) {
	if t.tool == nil {
		t.writeErr(w, http.StatusServiceUnavailable, errors.New("tool service is not configured"))
		return
	}
	parts := strings.Split(strings.Trim(t.idFromPath(r.URL.Path, "/api/v1/tools/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		t.writeErr(w, http.StatusBadRequest, errors.New("tool id is required"))
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		tool, err := t.tool.Get(id)
		if err != nil {
			t.writeErr(w, http.StatusBadRequest, err)
			return
		}
		t.writeJSON(w, http.StatusOK, tool)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodPut {
		var in domain.ToolUpsertInput
		if !t.decodeBody(w, r, &in) {
			return
		}
		updated, err := t.tool.Update(id, in)
		if err != nil {
			t.writeErr(w, http.StatusBadRequest, err)
			return
		}
		if t.audit != nil {
			_ = t.audit.Log("update", "tools", updated.ID, r.Header.Get("X-Request-Id"), "tool.update")
		}
		t.writeJSON(w, http.StatusOK, updated)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := t.tool.Delete(id); err != nil {
			t.writeErr(w, http.StatusBadRequest, err)
			return
		}
		if t.audit != nil {
			_ = t.audit.Log("delete", "tools", id, r.Header.Get("X-Request-Id"), "tool.delete")
		}
		t.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) == 2 && parts[1] == "runs" {
		q := r.URL.Query()
		q.Set("tool_key", id)
		r.URL.RawQuery = q.Encode()
		t.handleToolRuns(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "enabled" {
		t.handleToolEnabled(w, r, id)
		return
	}
	t.methodNotAllowed(w)
}

func (t *ToolHTTP) handleToolEnabled(w http.ResponseWriter, r *http.Request, id string) {
	if t.tool == nil {
		t.writeErr(w, http.StatusServiceUnavailable, errors.New("tool service is not configured"))
		return
	}
	if r.Method != http.MethodPatch {
		t.methodNotAllowed(w)
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !t.decodeBody(w, r, &in) {
		return
	}
	updated, err := t.tool.ToggleEnabled(id, in.Enabled)
	if err != nil {
		t.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if t.audit != nil {
		_ = t.audit.Log("update", "tools", updated.ID, r.Header.Get("X-Request-Id"), "enabled")
	}
	t.writeJSON(w, http.StatusOK, updated)
}

func (t *ToolHTTP) handleToolRuns(w http.ResponseWriter, r *http.Request) {
	if t.tool == nil {
		t.writeErr(w, http.StatusServiceUnavailable, errors.New("tool service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		t.methodNotAllowed(w)
		return
	}
	page, pageSize, offset := t.pageParams(r)
	result, err := t.tool.SearchRuns(domain.ToolRunQuery{
		ToolKey:   r.URL.Query().Get("tool_key"),
		AgentID:   r.URL.Query().Get("agent_id"),
		SessionID: r.URL.Query().Get("session_id"),
		Status:    r.URL.Query().Get("status"),
		From:      r.URL.Query().Get("from"),
		To:        r.URL.Query().Get("to"),
		Limit:     pageSize,
		Offset:    offset,
	})
	if err != nil {
		t.writeErr(w, http.StatusBadRequest, err)
		return
	}
	t.writeJSON(w, http.StatusOK, toolRunsPageResponse{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total})
}
