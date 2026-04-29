// transport/memory_l3.go 暴露 L3 语义记忆 HTTP 接口，见 `aranea/docs/15 memory-L3-semantic.md` §6.2–§6.6。
// 资源按工作区 / 用户 / 智能体等作用域（非会话作用域），路由在 handler.go 中直接注册于
// `/api/v1/memory/l3/...`，不由 sessions.go 分发。
package memoryhttp

import (
	mem "arenea/backend/internal/memory/domain"

	"errors"
	"net/http"
	"strconv"
	"strings"

	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
)

// registerMemoryL3Routes 同时挂载面向用户的事实 / 回忆 / 反馈端点，
// 以及仅管理端可用的衰减 / 嵌入 / 统计端点。管理路由位于 /api/v1/admin/memory/l3/。
func (m *MemoryHTTP) registerMemoryL3Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/memory/l3/facts", m.handleL3FactsCollection)
	mux.HandleFunc("/api/v1/memory/l3/facts/", m.handleL3FactsItem)
	mux.HandleFunc("/api/v1/memory/l3/facts:bulk-upsert", m.handleL3FactsBulkUpsert)
	mux.HandleFunc("/api/v1/memory/l3/recall", m.handleL3Recall)
	mux.HandleFunc("/api/v1/memory/l3/conflicts", m.handleL3Conflicts)
	mux.HandleFunc("/api/v1/memory/l3/conflicts/", m.handleL3ConflictItem)

	mux.HandleFunc("/api/v1/admin/memory/l3/decay/run", m.handleL3DecayRun)
	mux.HandleFunc("/api/v1/admin/memory/l3/embedding/rebuild", m.handleL3EmbeddingRebuild)
	mux.HandleFunc("/api/v1/admin/memory/l3/stats", m.handleL3Stats)
}

// --- 事实 CRUD --------------------------------------------------------------

func (m *MemoryHTTP) handleL3FactsCollection(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := repository.FactListQuery{
			ScopeType:   mem.ScopeType(r.URL.Query().Get("scope_type")),
			ScopeID:     r.URL.Query().Get("scope_id"),
			WorkspaceID: r.URL.Query().Get("workspace_id"),
			UserID:      r.URL.Query().Get("user_id"),
			TeamID:      r.URL.Query().Get("team_id"),
			AgentID:     r.URL.Query().Get("agent_id"),
			Status:      r.URL.Query().Get("status"),
			Kind:        mem.FactKind(r.URL.Query().Get("kind")),
			Tags:        splitCSV(r.URL.Query().Get("tags")),
			Keyword:     r.URL.Query().Get("keyword"),
			Limit:       m.parsePositiveInt(r.URL.Query().Get("limit"), 20),
			Offset:      m.parsePositiveInt(r.URL.Query().Get("offset"), 0),
		}
		out, err := svc.List(r.Context(), q)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		m.writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var in mem.FactUpsertInput
		if !m.decodeBody(w, r, &in) {
			return
		}
		fact, err := svc.UpsertFact(r.Context(), in)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l3.upsert_fact", "memory_facts", fact.ID, r.Header.Get("X-Request-Id"), string(fact.ScopeType))
		m.writeJSON(w, http.StatusCreated, fact)
	default:
		m.methodNotAllowed(w)
	}
}

// handleL3FactsItem 分发 /api/v1/memory/l3/facts/{id}[/...] 路径。
// 子资源：
//   - /versions  (GET)
//   - /feedback  (GET, POST)
//   - /rollback  (POST)
func (m *MemoryHTTP) handleL3FactsItem(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l3/facts/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("fact id is required"))
		return
	}
	factID := parts[0]
	if len(parts) == 1 {
		m.handleL3FactItemRoot(w, r, svc, factID)
		return
	}
	switch parts[1] {
	case "versions":
		m.handleL3FactVersions(w, r, svc, factID)
	case "feedback":
		m.handleL3FactFeedback(w, r, svc, factID)
	case "rollback":
		m.handleL3FactRollback(w, r, svc, factID)
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown fact sub-resource"))
	}
}

func (m *MemoryHTTP) handleL3FactItemRoot(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	switch r.Method {
	case http.MethodGet:
		fact, err := svc.Get(r.Context(), factID)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		m.writeJSON(w, http.StatusOK, fact)
	case http.MethodPatch:
		var patch service.FactPatch
		if !m.decodeBody(w, r, &patch) {
			return
		}
		updated, err := svc.UpdateFact(r.Context(), factID, patch)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l3.update_fact", "memory_facts", factID, r.Header.Get("X-Request-Id"), patch.Reason)
		m.writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		by := r.URL.Query().Get("by")
		if err := svc.DeleteFact(r.Context(), factID, by); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l3.delete_fact", "memory_facts", factID, r.Header.Get("X-Request-Id"), by)
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL3FactVersions(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
	versions, err := svc.ListVersions(r.Context(), factID, limit)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if versions == nil {
		versions = []mem.FactVersion{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.FactVersion]{Items: versions})
}

func (m *MemoryHTTP) handleL3FactFeedback(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	switch r.Method {
	case http.MethodGet:
		limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
		fbs, err := svc.ListFeedback(r.Context(), factID, limit)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		if fbs == nil {
			fbs = []mem.FactFeedback{}
		}
		m.writeJSON(w, http.StatusOK, listResponse[mem.FactFeedback]{Items: fbs})
	case http.MethodPost:
		var in mem.FactFeedback
		if !m.decodeBody(w, r, &in) {
			return
		}
		in.FactID = factID
		if err := svc.Feedback(r.Context(), in); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l3.feedback", "memory_facts", factID, r.Header.Get("X-Request-Id"), in.Type)
		w.WriteHeader(http.StatusAccepted)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL3FactRollback(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		ToVersion int    `json:"to_version"`
		By        string `json:"by"`
	}
	if !m.decodeBody(w, r, &in) {
		return
	}
	updated, err := svc.RollbackFact(r.Context(), factID, in.ToVersion, in.By)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l3.rollback_fact", "memory_facts", factID, r.Header.Get("X-Request-Id"), strconv.Itoa(in.ToVersion))
	m.writeJSON(w, http.StatusOK, updated)
}

func (m *MemoryHTTP) handleL3FactsBulkUpsert(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		Items []mem.FactUpsertInput `json:"items"`
	}
	if !m.decodeBody(w, r, &in) {
		return
	}
	if len(in.Items) == 0 {
		m.writeErr(w, http.StatusBadRequest, errors.New("items is required"))
		return
	}
	report, err := svc.BulkUpsert(r.Context(), in.Items)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l3.bulk_upsert", "memory_facts", "", r.Header.Get("X-Request-Id"), strconv.Itoa(len(in.Items)))
	m.writeJSON(w, http.StatusOK, report)
}

// --- 回忆 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL3Recall(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in mem.FactRecallQuery
	if !m.decodeBody(w, r, &in) {
		return
	}
	hits, err := svc.Recall(r.Context(), in)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if hits == nil {
		hits = []mem.FactRecallHit{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.FactRecallHit]{Items: hits})
}

// --- 冲突 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL3Conflicts(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	scope := mem.ScopeType(r.URL.Query().Get("scope_type"))
	scopeID := r.URL.Query().Get("scope_id")
	conflicts, err := svc.ListOpenConflicts(r.Context(), scope, scopeID)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if conflicts == nil {
		conflicts = []mem.FactConflict{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.FactConflict]{Items: conflicts})
}

func (m *MemoryHTTP) handleL3ConflictItem(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l3/conflicts/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("conflict id is required"))
		return
	}
	conflictID := parts[0]
	if len(parts) == 2 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			m.methodNotAllowed(w)
			return
		}
		var in struct {
			Resolution string `json:"resolution"`
			By         string `json:"by"`
		}
		if !m.decodeBody(w, r, &in) {
			return
		}
		if err := svc.ResolveConflict(r.Context(), conflictID, in.Resolution, in.By); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l3.resolve_conflict", "memory_fact_conflicts", conflictID, r.Header.Get("X-Request-Id"), in.Resolution)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	m.writeErr(w, http.StatusNotFound, errors.New("unknown conflict path"))
}

// --- 管理 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL3DecayRun(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	report, err := svc.RunDecayBatch(r.Context())
	if err != nil {
		m.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = m.audit.Log("l3.decay_run", "memory_facts", "", r.Header.Get("X-Request-Id"), "")
	m.writeJSON(w, http.StatusOK, report)
}

func (m *MemoryHTTP) handleL3EmbeddingRebuild(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		FactID string `json:"fact_id"`
	}
	if !m.decodeBody(w, r, &in) {
		return
	}
	if err := svc.BuildEmbedding(r.Context(), in.FactID); err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l3.embedding_rebuild", "memory_facts", in.FactID, r.Header.Get("X-Request-Id"), "")
	w.WriteHeader(http.StatusAccepted)
}

func (m *MemoryHTTP) handleL3Stats(w http.ResponseWriter, r *http.Request) {
	svc := m.l3()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	scope := mem.ScopeType(r.URL.Query().Get("scope_type"))
	scopeID := r.URL.Query().Get("scope_id")
	report, err := svc.Stats(r.Context(), scope, scopeID)
	if err != nil {
		m.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	m.writeJSON(w, http.StatusOK, report)
}
