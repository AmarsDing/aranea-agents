// transport/memory_l4.go 暴露 L4 持久 / 知识图谱 HTTP 接口，见 `aranea/docs/16 memory-L4-persistent.md`
// §6.2（实体）、§6.3（关系）、§6.5（管理 / 抽取）。
// 资源按工作区 / 用户 / 智能体等作用域（非会话作用域），路由在 handler.go 中直接注册于 `/api/v1/memory/l4/...`。
package memoryhttp

import (
	mem "arenea/backend/internal/memory/domain"

	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
)

// registerMemoryL4Routes 挂载面向用户的实体 / 关系 / 邻域端点，
// 以及仅管理端可用的抽取等端点。管理路由位于 /api/v1/admin/memory/l4/。
func (m *MemoryHTTP) registerMemoryL4Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/memory/l4/entities:search", m.handleL4EntitiesSearch)
	mux.HandleFunc("/api/v1/memory/l4/entities", m.handleL4EntitiesCollection)
	mux.HandleFunc("/api/v1/memory/l4/entities/", m.handleL4EntitiesItem)
	mux.HandleFunc("/api/v1/memory/l4/relations", m.handleL4RelationsCollection)
	mux.HandleFunc("/api/v1/memory/l4/relations/", m.handleL4RelationsItem)
	mux.HandleFunc("/api/v1/memory/l4/nodes/", m.handleL4NodeRelations)
	mux.HandleFunc("/api/v1/memory/l4/neighborhood", m.handleL4Neighborhood)
	mux.HandleFunc("/api/v1/memory/l4/search", m.handleL4Search)
	mux.HandleFunc("/api/v1/memory/l4/extract/episode/", m.handleL4ExtractEpisodePath)
	mux.HandleFunc("/api/v1/memory/l4/extract/fact/", m.handleL4ExtractFactPath)

	mux.HandleFunc("/api/v1/admin/memory/l4/extract/episode", m.handleL4ExtractEpisode)
	mux.HandleFunc("/api/v1/admin/memory/l4/extract/fact", m.handleL4ExtractFact)
}

// --- 实体 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL4EntitiesCollection(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := repository.EntityListQuery{
			ScopeType:   mem.ScopeType(r.URL.Query().Get("scope_type")),
			ScopeID:     r.URL.Query().Get("scope_id"),
			WorkspaceID: r.URL.Query().Get("workspace_id"),
			UserID:      r.URL.Query().Get("user_id"),
			EntityType:  mem.EntityType(r.URL.Query().Get("entity_type")),
			Status:      r.URL.Query().Get("status"),
			Keyword:     r.URL.Query().Get("keyword"),
			Limit:       m.parsePositiveInt(r.URL.Query().Get("limit"), 50),
			Offset:      m.parsePositiveInt(r.URL.Query().Get("offset"), 0),
		}
		out, err := svc.ListEntities(r.Context(), q)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		m.writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var in service.EntityUpsertInput
		if !m.decodeBody(w, r, &in) {
			return
		}
		ent, err := svc.UpsertEntity(r.Context(), in)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l4.upsert_entity", "memory_entities", ent.ID, r.Header.Get("X-Request-Id"), string(ent.ScopeType))
		m.writeJSON(w, http.StatusCreated, ent)
	default:
		m.methodNotAllowed(w)
	}
}

// handleL4EntitiesItem 分发 /api/v1/memory/l4/entities/{id}[/...] 路径。
// 子资源：
//   - /versions  (GET)
//   - /facts     (GET)
//   - /rename    (POST)
//   - /merge     (POST)
//   - /archive   (POST)
func (m *MemoryHTTP) handleL4EntitiesItem(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/entities/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("entity id is required"))
		return
	}
	entityID := parts[0]
	if len(parts) == 1 {
		m.handleL4EntityItemRoot(w, r, svc, entityID)
		return
	}
	switch parts[1] {
	case "versions":
		m.handleL4EntityVersions(w, r, svc, entityID)
	case "facts":
		m.handleL4EntityFacts(w, r, svc, entityID)
	case "neighborhood":
		m.handleL4EntityNeighborhood(w, r, svc, entityID)
	case "rename":
		m.handleL4EntityRename(w, r, svc, entityID)
	case "merge":
		m.handleL4EntityMerge(w, r, svc, entityID)
	case "archive":
		m.handleL4EntityArchive(w, r, svc, entityID)
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown entity sub-resource"))
	}
}

func (m *MemoryHTTP) handleL4EntityItemRoot(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	switch r.Method {
	case http.MethodGet:
		ent, err := svc.GetEntity(r.Context(), id)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		m.writeJSON(w, http.StatusOK, ent)
	case http.MethodPatch:
		var in service.EntityUpsertInput
		if !m.decodeBody(w, r, &in) {
			return
		}
		in.ID = id
		ent, err := svc.UpsertEntity(r.Context(), in)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l4.update_entity", "memory_entities", ent.ID, r.Header.Get("X-Request-Id"), in.Reason)
		m.writeJSON(w, http.StatusOK, ent)
	case http.MethodDelete:
		by := r.URL.Query().Get("by")
		reason := r.URL.Query().Get("reason")
		if err := svc.DeleteEntity(r.Context(), id, by, reason); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l4.delete_entity", "memory_entities", id, r.Header.Get("X-Request-Id"), by)
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL4EntityVersions(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
	versions, err := svc.ListEntityVersions(r.Context(), id, limit)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if versions == nil {
		versions = []mem.MemoryEntityVersion{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryEntityVersion]{Items: versions})
}

func (m *MemoryHTTP) handleL4EntityFacts(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
	links, err := svc.ListEntityFacts(r.Context(), id, limit)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if links == nil {
		links = []mem.MemoryEntityFactLink{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryEntityFactLink]{Items: links})
}

func (m *MemoryHTTP) handleL4EntityRename(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		Name   string `json:"name"`
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	if !m.decodeBody(w, r, &in) {
		return
	}
	ent, err := svc.RenameEntity(r.Context(), id, in.Name, in.By, in.Reason)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l4.rename_entity", "memory_entities", id, r.Header.Get("X-Request-Id"), in.Reason)
	m.writeJSON(w, http.StatusOK, ent)
}

func (m *MemoryHTTP) handleL4EntityMerge(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, primaryID string) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		Sources []string `json:"sources"`
		Into    string   `json:"into"`
		By      string   `json:"by"`
		Reason  string   `json:"reason"`
	}
	if !m.decodeBody(w, r, &in) {
		return
	}
	targetID := primaryID
	sources := in.Sources
	if in.Into != "" && len(sources) == 0 {
		targetID = in.Into
		sources = []string{primaryID}
	}
	if err := svc.MergeEntities(r.Context(), targetID, sources, in.By, in.Reason); err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l4.merge_entities", "memory_entities", targetID, r.Header.Get("X-Request-Id"), in.Reason)
	w.WriteHeader(http.StatusNoContent)
}

func (m *MemoryHTTP) handleL4EntityArchive(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	_ = m.decodeBody(w, r, &in) // 请求体可选
	if err := svc.ArchiveEntity(r.Context(), id, in.By, in.Reason); err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l4.archive_entity", "memory_entities", id, r.Header.Get("X-Request-Id"), in.Reason)
	w.WriteHeader(http.StatusNoContent)
}

// --- 关系 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL4RelationsCollection(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		var in service.RelationUpsertInput
		if !m.decodeBody(w, r, &in) {
			return
		}
		rel, err := svc.UpsertRelation(r.Context(), in)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l4.upsert_relation", "memory_relations", rel.ID, r.Header.Get("X-Request-Id"), string(rel.RelationType))
		m.writeJSON(w, http.StatusCreated, rel)
	case http.MethodGet:
		nodeID := r.URL.Query().Get("node_id")
		if nodeID == "" {
			m.writeErr(w, http.StatusBadRequest, errors.New("node_id is required"))
			return
		}
		limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
		rels, err := svc.ListRelationsForNode(r.Context(), nodeID, limit)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		if rels == nil {
			rels = []mem.MemoryRelation{}
		}
		m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryRelation]{Items: rels})
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL4RelationsItem(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/relations/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("relation id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		rel, err := svc.GetRelation(r.Context(), id)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		m.writeJSON(w, http.StatusOK, rel)
	case http.MethodDelete:
		by := r.URL.Query().Get("by")
		reason := r.URL.Query().Get("reason")
		if err := svc.DeleteRelation(r.Context(), id, by, reason); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l4.delete_relation", "memory_relations", id, r.Header.Get("X-Request-Id"), by)
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL4NodeRelations(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/nodes/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "relations" {
		m.writeErr(w, http.StatusNotFound, errors.New("unknown node relation path"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
	rels, err := svc.ListRelationsForNode(r.Context(), parts[0], limit)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if rels == nil {
		rels = []mem.MemoryRelation{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryRelation]{Items: rels})
}

// --- 邻域 / 搜索 --------------------------------------------------------------

func (m *MemoryHTTP) handleL4Neighborhood(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	centerID := r.URL.Query().Get("center_id")
	hops := m.parsePositiveInt(r.URL.Query().Get("hops"), 1)
	maxNodes := m.parsePositiveInt(r.URL.Query().Get("max_nodes"), 12)
	nb, err := svc.Neighborhood(r.Context(), centerID, hops, maxNodes)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, nb)
}

func (m *MemoryHTTP) handleL4EntityNeighborhood(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, entityID string) {
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	hops := m.parsePositiveInt(r.URL.Query().Get("hops"), 1)
	maxNodes := m.parsePositiveInt(firstNonEmptyQuery(r, "max", "max_nodes"), 12)
	nb, err := svc.Neighborhood(r.Context(), entityID, hops, maxNodes)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, nb)
}

func (m *MemoryHTTP) handleL4Search(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	scope := mem.ScopeType(r.URL.Query().Get("scope_type"))
	scopeID := r.URL.Query().Get("scope_id")
	query := r.URL.Query().Get("q")
	topK := m.parsePositiveInt(r.URL.Query().Get("top_k"), 10)
	hits, err := svc.SearchByText(r.Context(), scope, scopeID, query, topK)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if hits == nil {
		hits = []mem.MemoryEntity{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryEntity]{Items: hits})
}

func (m *MemoryHTTP) handleL4EntitiesSearch(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	var in struct {
		ScopeType mem.ScopeType `json:"scope_type"`
		ScopeID   string           `json:"scope_id"`
		Query     string           `json:"query"`
		TopK      int              `json:"top_k"`
	}
	if r.Method == http.MethodPost {
		if !m.decodeBody(w, r, &in) {
			return
		}
	} else if r.Method == http.MethodGet {
		in.ScopeType = mem.ScopeType(r.URL.Query().Get("scope_type"))
		in.ScopeID = r.URL.Query().Get("scope_id")
		in.Query = firstNonEmptyQuery(r, "query", "q")
		in.TopK = m.parsePositiveInt(r.URL.Query().Get("top_k"), 10)
	} else {
		m.methodNotAllowed(w)
		return
	}
	hits, err := svc.SearchByText(r.Context(), in.ScopeType, in.ScopeID, in.Query, in.TopK)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if hits == nil {
		hits = []mem.MemoryEntity{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryEntity]{Items: hits})
}

// --- 管理 / 抽取 --------------------------------------------------------------

func (m *MemoryHTTP) handleL4ExtractEpisode(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in struct {
		EpisodeID string `json:"episode_id"`
	}
	if !m.decodeBody(w, r, &in) {
		return
	}
	report, err := svc.ExtractFromEpisode(r.Context(), in.EpisodeID)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l4.extract_episode", "memory_entities", in.EpisodeID, r.Header.Get("X-Request-Id"), "")
	m.writeJSON(w, http.StatusOK, report)
}

func (m *MemoryHTTP) handleL4ExtractFact(w http.ResponseWriter, r *http.Request) {
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
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
	report, err := svc.ExtractFromFact(r.Context(), in.FactID)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = m.audit.Log("l4.extract_fact", "memory_entities", in.FactID, r.Header.Get("X-Request-Id"), "")
	m.writeJSON(w, http.StatusOK, report)
}

func (m *MemoryHTTP) handleL4ExtractEpisodePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	episodeID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/extract/episode/"), "/")
	report, err := svc.ExtractFromEpisode(r.Context(), episodeID)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, report)
}

func (m *MemoryHTTP) handleL4ExtractFactPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	svc := m.l4()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	factID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/extract/fact/"), "/")
	report, err := svc.ExtractFromFact(r.Context(), factID)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, report)
}

func firstNonEmptyQuery(r *http.Request, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(r.URL.Query().Get(key))
		if value != "" {
			return value
		}
	}
	return ""
}
