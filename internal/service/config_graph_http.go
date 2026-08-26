package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	bizcg "aranea-agents/internal/biz/configgraph"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// ConfigGraphRebuilderPort 是 M81 config-graph HTTP API 需要的窄重建/状态面
// （由 *configgraph.Rebuilder 结构满足；consumer-side port，便于单测）。
type ConfigGraphRebuilderPort interface {
	Current() int64
	Ready() bool
	Running() bool
	RebuildAsync() (gen int64, started bool)
	LastRebuild() (res bizcg.RebuildResult, at time.Time, ok bool)
}

// ConfigGraphService 是 M81 配置资产图谱管理面 API（design §6，knowledge_http.go
// 模式：纯 net/http 手写 JSON）。P0 开放 rebuild / status / nodes 检索；
// P1 增加 impact / dependencies / edges / health 四个查询端点。
//
// 错误体统一 {"error":{"code","message"}}，错误码见 design §6：
// CONFIG_GRAPH.NOT_READY（503）/ CONFIG_GRAPH.BAD_REQUEST（400）/
// CONFIG_GRAPH.NODE_NOT_FOUND（404）。
type ConfigGraphService struct {
	rebuilder ConfigGraphRebuilderPort
	repo      bizcg.Repo
	querier   *bizcg.Querier
	lg        loggateway.Logger
}

// NewConfigGraphService 构造服务。rebuilder/repo 任一为空（图未装配）时返回
// nil，路由侧判空跳过注册。Querier 内部以 rebuilder.Current 为代际来源装配。
func NewConfigGraphService(rebuilder ConfigGraphRebuilderPort, repo bizcg.Repo, lg loggateway.Logger) *ConfigGraphService {
	if rebuilder == nil || repo == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ConfigGraphService{
		rebuilder: rebuilder,
		repo:      repo,
		querier:   bizcg.NewQuerier(repo, rebuilder.Current),
		lg:        lg,
	}
}

func writeConfigGraphJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeConfigGraphError(w http.ResponseWriter, status int, code, msg string) {
	writeConfigGraphJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}

// configGraphAdmin 校验 admin 身份（design §6：走现有 admin 认证，GET 不涉
// HITL；rebuild 只重建派生数据，非高危，无需确认门禁）。
func configGraphAdmin(w http.ResponseWriter, r *http.Request) bool {
	a, ok := auth.FromContext(r.Context())
	if !ok || a == nil {
		writeConfigGraphError(w, http.StatusUnauthorized, "CONFIG_GRAPH.UNAUTHORIZED", "authentication required")
		return false
	}
	if !a.HasAdminAccess() {
		writeConfigGraphError(w, http.StatusForbidden, "CONFIG_GRAPH.FORBIDDEN", "admin access required")
		return false
	}
	return true
}

// ServeRebuild POST /api/v1/config-graph/rebuild — 异步触发全量重建，返回在
// 建代。已在途时 started=false（幂等去重，不报错）。
func (s *ConfigGraphService) ServeRebuild(w http.ResponseWriter, r *http.Request) {
	if !configGraphAdmin(w, r) {
		return
	}
	gen, started := s.rebuilder.RebuildAsync()
	writeConfigGraphJSON(w, http.StatusAccepted, map[string]any{
		"generation": gen,
		"started":    started,
	})
}

// ServeStatus GET /api/v1/config-graph/status — 当前代/计数/在途标志/最近重
// 建摘要。未建图时 ready=false 且不带计数字段（NOT_READY 只约束查询端点）。
func (s *ConfigGraphService) ServeStatus(w http.ResponseWriter, r *http.Request) {
	if !configGraphAdmin(w, r) {
		return
	}
	gen := s.rebuilder.Current()
	resp := map[string]any{
		"ready":      s.rebuilder.Ready(),
		"running":    s.rebuilder.Running(),
		"generation": gen,
	}
	if gen > 0 {
		counts, err := s.repo.Counts(r.Context(), gen)
		if err != nil {
			s.lg.Warn("configgraph status counts failed", loggateway.Err(err))
			writeConfigGraphError(w, http.StatusInternalServerError, "CONFIG_GRAPH.INTERNAL", "internal error")
			return
		}
		resp["nodes"] = counts.Nodes
		resp["edges"] = counts.Edges
		resp["broken"] = counts.Broken
	}
	if res, at, ok := s.rebuilder.LastRebuild(); ok {
		resp["last_rebuild"] = map[string]any{
			"generation": res.Generation,
			"nodes":      res.Nodes,
			"edges":      res.Edges,
			"broken":     res.Broken,
			"elapsed_ms": res.Elapsed.Milliseconds(),
			"at":         at.UTC().Format(time.RFC3339),
		}
	}
	writeConfigGraphJSON(w, http.StatusOK, resp)
}

// configGraphNodeTypes 是 nodes 检索 type 参数的合法值（12 类，design §3.1）。
var configGraphNodeTypes = map[string]bool{
	bizcg.NodeTypeAgent:               true,
	bizcg.NodeTypeTeam:                true,
	bizcg.NodeTypeSkill:               true,
	bizcg.NodeTypeTool:                true,
	bizcg.NodeTypePromptFile:          true,
	bizcg.NodeTypeCronTask:            true,
	bizcg.NodeTypeChannel:             true,
	bizcg.NodeTypeOrganization:        true,
	bizcg.NodeTypeGraph:               true,
	bizcg.NodeTypeKnowledgeCollection: true,
	bizcg.NodeTypeMCPServer:           true,
	bizcg.NodeTypeHook:                true,
}

// ServeNodes GET /api/v1/config-graph/nodes?type=&key=&workspace=&limit= —
// 当前代节点检索（辅助端点；key 为 node_key 子串匹配，limit 走 repo 默认/上限）。
func (s *ConfigGraphService) ServeNodes(w http.ResponseWriter, r *http.Request) {
	if !configGraphAdmin(w, r) {
		return
	}
	gen := s.rebuilder.Current()
	if gen == 0 {
		writeConfigGraphError(w, http.StatusServiceUnavailable, "CONFIG_GRAPH.NOT_READY", "graph not built yet; trigger POST /api/v1/config-graph/rebuild first")
		return
	}
	q := r.URL.Query()
	filter := bizcg.NodeFilter{Generation: gen}
	if t := strings.TrimSpace(q.Get("type")); t != "" {
		if !configGraphNodeTypes[t] {
			writeConfigGraphError(w, http.StatusBadRequest, "CONFIG_GRAPH.BAD_REQUEST", "unknown node type: "+t)
			return
		}
		filter.NodeType = t
	}
	filter.KeyContains = strings.TrimSpace(q.Get("key"))
	filter.WorkspaceID = strings.TrimSpace(q.Get("workspace"))
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeConfigGraphError(w, http.StatusBadRequest, "CONFIG_GRAPH.BAD_REQUEST", "limit must be a non-negative integer")
			return
		}
		filter.Limit = n
	}
	nodes, err := s.repo.ListNodes(r.Context(), filter)
	if err != nil {
		s.lg.Warn("configgraph nodes query failed", loggateway.Err(err))
		writeConfigGraphError(w, http.StatusInternalServerError, "CONFIG_GRAPH.INTERNAL", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, map[string]any{
			"id":           n.ID,
			"node_type":    n.NodeType,
			"ref_id":       n.RefID,
			"node_key":     n.NodeKey,
			"display_name": n.DisplayName,
			"workspace_id": n.WorkspaceID,
			"status":       n.Status,
			"attrs":        n.Attrs,
			"created_at":   n.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":   n.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeConfigGraphJSON(w, http.StatusOK, map[string]any{
		"generation": gen,
		"items":      items,
	})
}

// ── P1 查询端点（design §5/§6）─────────────────────────────────────────────

// writeConfigGraphQueryError 映射查询侧错误：ErrNotReady→503、NotFound→404、
// 其他→500（design §6 错误码）。
func (s *ConfigGraphService) writeConfigGraphQueryError(w http.ResponseWriter, err error) {
	s.lg.Warn("configgraph query failed", loggateway.Err(err))
	switch {
	case errors.Is(err, bizcg.ErrNotReady):
		writeConfigGraphError(w, http.StatusServiceUnavailable, "CONFIG_GRAPH.NOT_READY", "graph not built yet; trigger POST /api/v1/config-graph/rebuild first")
	case apierror.IsCode(err, apierror.CodeNotFound):
		writeConfigGraphError(w, http.StatusNotFound, "CONFIG_GRAPH.NODE_NOT_FOUND", "node not found")
	default:
		writeConfigGraphError(w, http.StatusInternalServerError, "CONFIG_GRAPH.INTERNAL", "internal error")
	}
}

// configGraphWalkTarget 校验 {type}/{ref} 路径参数（type 白名单 + ref 非空），
// 合法时返回 trim 后的值。
func configGraphWalkTarget(w http.ResponseWriter, nodeType, ref string) (string, string, bool) {
	t := strings.TrimSpace(nodeType)
	if !configGraphNodeTypes[t] {
		writeConfigGraphError(w, http.StatusBadRequest, "CONFIG_GRAPH.BAD_REQUEST", "unknown node type: "+t)
		return "", "", false
	}
	r := strings.TrimSpace(ref)
	if r == "" {
		writeConfigGraphError(w, http.StatusBadRequest, "CONFIG_GRAPH.BAD_REQUEST", "ref must not be empty")
		return "", "", false
	}
	return t, r, true
}

// configGraphDepthParam 解析 depth 查询参数（缺省/0 → biz 默认；非整数 → 400）。
func configGraphDepthParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("depth"))
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		writeConfigGraphError(w, http.StatusBadRequest, "CONFIG_GRAPH.BAD_REQUEST", "depth must be an integer")
		return 0, false
	}
	return n, true
}

// ServeImpact GET /api/v1/config-graph/nodes/{type}/{ref}/impact?depth= —
// 反向闭包 + signals + risk 加权（design §5.1）。
func (s *ConfigGraphService) ServeImpact(w http.ResponseWriter, r *http.Request, nodeType, ref string) {
	if !configGraphAdmin(w, r) {
		return
	}
	t, ref, ok := configGraphWalkTarget(w, nodeType, ref)
	if !ok {
		return
	}
	depth, ok := configGraphDepthParam(w, r)
	if !ok {
		return
	}
	res, err := s.querier.Impact(r.Context(), t, ref, depth)
	if err != nil {
		s.writeConfigGraphQueryError(w, err)
		return
	}
	writeConfigGraphJSON(w, http.StatusOK, res)
}

// ServeDependencies GET /api/v1/config-graph/nodes/{type}/{ref}/dependencies?depth=
// — 正向闭包（design §5.2）。
func (s *ConfigGraphService) ServeDependencies(w http.ResponseWriter, r *http.Request, nodeType, ref string) {
	if !configGraphAdmin(w, r) {
		return
	}
	t, ref, ok := configGraphWalkTarget(w, nodeType, ref)
	if !ok {
		return
	}
	depth, ok := configGraphDepthParam(w, r)
	if !ok {
		return
	}
	res, err := s.querier.Dependencies(r.Context(), t, ref, depth)
	if err != nil {
		s.writeConfigGraphQueryError(w, err)
		return
	}
	writeConfigGraphJSON(w, http.StatusOK, res)
}

// ServeNodeEdges GET /api/v1/config-graph/nodes/{type}/{ref}/edges — 单节点
// 邻接边（出/入/断三段 + evidence，design §5.3）。
func (s *ConfigGraphService) ServeNodeEdges(w http.ResponseWriter, r *http.Request, nodeType, ref string) {
	if !configGraphAdmin(w, r) {
		return
	}
	t, ref, ok := configGraphWalkTarget(w, nodeType, ref)
	if !ok {
		return
	}
	res, err := s.querier.NodeEdges(r.Context(), t, ref)
	if err != nil {
		s.writeConfigGraphQueryError(w, err)
		return
	}
	writeConfigGraphJSON(w, http.StatusOK, res)
}

// ServeHealth GET /api/v1/config-graph/health — god node/环/断边/重复
// prompt 四类报告（design §5.4）。
func (s *ConfigGraphService) ServeHealth(w http.ResponseWriter, r *http.Request) {
	if !configGraphAdmin(w, r) {
		return
	}
	rep, err := s.querier.Health(r.Context())
	if err != nil {
		s.writeConfigGraphQueryError(w, err)
		return
	}
	writeConfigGraphJSON(w, http.StatusOK, rep)
}
