// transport/memory_l1.go 暴露 L1 工作记忆 HTTP 接口，见 `aranea/docs/13 memory-L1-working.md` §6.2、§6.3。
// 路由在 sessions.go（handleSessionByID）中，将 L1 路径转发至此。
// 模式管理路由（§6.2）位于 /api/v1/memory/l1/schemas，由 registerMemoryL1Routes 单独挂载。
package memoryhttp

import (
	mem "arenea/backend/internal/memory/domain"

	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/service"
)

// registerMemoryL1Routes 由 registerRoutes 调用，绑定智能体作用域的模式管理端点。
// 会话作用域的任务 / 字段路由由 handleSessionByID 经 splitSessionPathSuffix 分发。
func (m *MemoryHTTP) registerMemoryL1Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/memory/l1/schemas", m.handleL1Schemas)
	mux.HandleFunc("/api/v1/memory/l1/schemas/", m.handleL1SchemaByID)
}

// HandleL1Routes 分发 /api/v1/sessions/{sid}/l1/... 请求。
// suffix 已解析为路径段（例如 "tasks/abc/fields/x"）。
func (m *MemoryHTTP) HandleL1Routes(w http.ResponseWriter, r *http.Request, sessionID, suffix string) {
	svc := m.l1()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L1 service is not configured"))
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || parts[0] != "tasks" {
		m.writeErr(w, http.StatusNotFound, errors.New("unknown l1 path"))
		return
	}
	switch len(parts) {
	case 1:
		m.handleL1TasksCollection(w, r, svc, sessionID)
	case 2:
		m.handleL1TaskItem(w, r, svc, sessionID, parts[1])
	default:
		m.handleL1TaskSubresource(w, r, svc, sessionID, parts[1], parts[2:])
	}
}

func (m *MemoryHTTP) handleL1TasksCollection(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		query := mem.L1TaskListQuery{
			SessionID:    sessionID,
			AgentID:      r.URL.Query().Get("agent_id"),
			Status:       r.URL.Query().Get("status"),
			IncludeEnded: r.URL.Query().Get("include_ended") == "true",
		}
		tasks, err := svc.ListTasks(r.Context(), query)
		if err != nil {
			m.writeErr(w, http.StatusInternalServerError, err)
			return
		}
		m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryL1Task]{Items: tasks})
	case http.MethodPost:
		var in struct {
			RunID        string                `json:"run_id"`
			TeamID       string                `json:"team_id"`
			AgentID      string                `json:"agent_id"`
			TaskKey      string                `json:"task_key"`
			TaskTitle    string                `json:"task_title"`
			TaskGoal     string                `json:"task_goal"`
			ParentTaskID string                `json:"parent_task_id"`
			SchemaID     string                `json:"schema_id"`
			BudgetTokens int                   `json:"budget_tokens"`
			SharedWith   []mem.L1FieldShare `json:"shared_with"`
			Metadata     map[string]any        `json:"metadata"`
		}
		if !m.decodeBody(w, r, &in) {
			return
		}
		task, err := svc.StartTask(r.Context(), service.StartL1TaskInput{
			SessionID:    sessionID,
			RunID:        in.RunID,
			TeamID:       in.TeamID,
			AgentID:      in.AgentID,
			TaskKey:      in.TaskKey,
			TaskTitle:    in.TaskTitle,
			TaskGoal:     in.TaskGoal,
			ParentTaskID: in.ParentTaskID,
			SchemaID:     in.SchemaID,
			BudgetTokens: in.BudgetTokens,
			SharedWith:   in.SharedWith,
			Metadata:     in.Metadata,
		})
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l1.create_task", "memory_l1_task", task.ID, r.Header.Get("X-Request-Id"), task.TaskKey)
		m.writeJSON(w, http.StatusCreated, task)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL1TaskItem(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID, taskID string) {
	if taskID == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("task id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := svc.GetTask(r.Context(), taskID)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		if view.Task.SessionID != sessionID {
			m.writeErr(w, http.StatusNotFound, errors.New("task not found in session"))
			return
		}
		m.writeJSON(w, http.StatusOK, view)
	case http.MethodPatch:
		var in struct {
			Status       string                `json:"status"`
			BudgetTokens int                   `json:"budget_tokens"`
			SharedWith   []mem.L1FieldShare `json:"shared_with"`
		}
		if !m.decodeBody(w, r, &in) {
			return
		}
		if in.BudgetTokens > 0 {
			if err := svc.UpdateTaskBudget(r.Context(), taskID, in.BudgetTokens); err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if in.SharedWith != nil {
			if err := svc.UpdateTaskShared(r.Context(), taskID, in.SharedWith); err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if in.Status != "" {
			if err := svc.EndTask(r.Context(), taskID, mem.L1TaskStatus(in.Status)); err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		view, err := svc.GetTask(r.Context(), taskID)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		m.writeJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		if err := svc.EndTask(r.Context(), taskID, mem.L1TaskArchived); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l1.archive_task", "memory_l1_task", taskID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

// handleL1TaskSubresource 分发 `tasks/{id}/<sub>...` 路径。
// 支持的子资源：
//   - fields                 (GET batch / PATCH batch-patch)
//   - fields/{path}          (GET / PUT / DELETE)
//   - fields/{path}/history  (GET)
//   - fields/{path}/rollback (POST)
//   - render-prompt          (POST)
func (m *MemoryHTTP) handleL1TaskSubresource(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID, taskID string, parts []string) {
	if taskID == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("task id is required"))
		return
	}
	if len(parts) == 0 {
		m.writeErr(w, http.StatusNotFound, errors.New("unknown subresource"))
		return
	}
	switch parts[0] {
	case "render-prompt":
		m.handleL1RenderPrompt(w, r, svc, taskID)
	case "fields":
		m.handleL1Fields(w, r, svc, sessionID, taskID, parts[1:])
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown subresource"))
	}
}

func (m *MemoryHTTP) handleL1RenderPrompt(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, taskID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	viewer := r.URL.Query().Get("viewer_agent_id")
	block, err := svc.RenderForPrompt(r.Context(), taskID, viewer)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, block)
}

func (m *MemoryHTTP) handleL1Fields(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID, taskID string, parts []string) {
	_ = sessionID
	switch len(parts) {
	case 0:
		switch r.Method {
		case http.MethodGet:
			includeInternal := r.URL.Query().Get("include_internal") == "true"
			fields, err := svc.ListFieldsByTask(r.Context(), taskID, includeInternal)
			if err != nil {
				m.writeErr(w, http.StatusInternalServerError, err)
				return
			}
			m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryL1Field]{Items: fields})
		case http.MethodPatch:
			var in struct {
				Patches []mem.L1FieldPatch `json:"patches"`
			}
			if !m.decodeBody(w, r, &in) {
				return
			}
			results, err := svc.PatchFields(r.Context(), taskID, in.Patches)
			if err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
			m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryL1Field]{Items: results})
		default:
			m.methodNotAllowed(w)
		}
		return
	case 1:
		m.handleL1FieldItem(w, r, svc, taskID, parts[0])
		return
	case 2:
		fieldPath := parts[0]
		switch parts[1] {
		case "history":
			if r.Method != http.MethodGet {
				m.methodNotAllowed(w)
				return
			}
			limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 20)
			items, err := svc.ListFieldHistory(r.Context(), taskID, fieldPath, limit)
			if err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
			m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryL1FieldHistory]{Items: items})
		case "rollback":
			if r.Method != http.MethodPost {
				m.methodNotAllowed(w)
				return
			}
			var in struct {
				ToRevision int    `json:"to_revision"`
				ChangedBy  string `json:"changed_by"`
			}
			if !m.decodeBody(w, r, &in) {
				return
			}
			field, err := svc.RollbackField(r.Context(), taskID, fieldPath, in.ToRevision, in.ChangedBy)
			if err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
			m.writeJSON(w, http.StatusOK, field)
		default:
			m.writeErr(w, http.StatusNotFound, errors.New("unknown field action"))
		}
		return
	}
	m.writeErr(w, http.StatusNotFound, errors.New("unknown field path"))
}

func (m *MemoryHTTP) handleL1FieldItem(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, taskID, fieldPath string) {
	if fieldPath == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("field path is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		field, err := svc.GetField(r.Context(), taskID, fieldPath)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		m.writeJSON(w, http.StatusOK, field)
	case http.MethodPut:
		var patch mem.L1FieldPatch
		if !m.decodeBody(w, r, &patch) {
			return
		}
		patch.FieldPath = fieldPath
		stored, err := svc.SetField(r.Context(), taskID, patch)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		m.writeJSON(w, http.StatusOK, stored)
	case http.MethodDelete:
		if err := svc.DeleteField(r.Context(), taskID, fieldPath); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

// handleL1Schemas 处理 /api/v1/memory/l1/schemas。GET 按 scope_type、scope_id 查询参数过滤；
// POST 插入或更新模式行。模式正文保持 JSON 字符串，便于后续第二阶段校验器接入而无需改传输层。
func (m *MemoryHTTP) handleL1Schemas(w http.ResponseWriter, r *http.Request) {
	svc := m.l1()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L1 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		scopeType := r.URL.Query().Get("scope_type")
		scopeID := r.URL.Query().Get("scope_id")
		schemas, err := svc.ListSchemas(r.Context(), scopeType, scopeID)
		if err != nil {
			m.writeErr(w, http.StatusInternalServerError, err)
			return
		}
		m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryL1Schema]{Items: schemas})
	case http.MethodPost:
		var in mem.MemoryL1Schema
		if !m.decodeBody(w, r, &in) {
			return
		}
		if in.ID == "" {
			in.ID = "l1schema_" + strings.ReplaceAll(in.SchemaKey, ".", "_")
		}
		stored, err := svc.UpsertSchema(r.Context(), in)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l1.upsert_schema", "memory_l1_schema", stored.ID, r.Header.Get("X-Request-Id"), stored.SchemaKey)
		m.writeJSON(w, http.StatusCreated, stored)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL1SchemaByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/memory/l1/schemas/")
	if id == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("schema id is required"))
		return
	}
	svc := m.l1()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L1 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		stored, err := svc.GetSchema(r.Context(), id)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		m.writeJSON(w, http.StatusOK, stored)
	case http.MethodPatch:
		stored, err := svc.GetSchema(r.Context(), id)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		var in mem.MemoryL1Schema
		if !m.decodeBody(w, r, &in) {
			return
		}
		if in.SchemaJSON != "" {
			stored.SchemaJSON = in.SchemaJSON
		}
		if in.Description != "" {
			stored.Description = in.Description
		}
		stored.Enabled = in.Enabled
		updated, err := svc.UpsertSchema(r.Context(), stored)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		m.writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := svc.DeleteSchema(r.Context(), id); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

