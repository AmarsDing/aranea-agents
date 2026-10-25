package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// ── G-05-asof（ADR-KN-20261020-K）：as-of 时态读面 custom HTTP ────────────────
// 双路由：POST /v1/knowledge/search/asof（检索快照）+ GET /v1/knowledge/vaults/
// {id}/graph/asof（图快照）。custom JSON 避免 make api；鉴权 = 默认 JWT Filter +
// assertCollectionAccess（C-01 404 反 IDOR）；错误映射复用 writeKnowledgeJSONError
// 单点（Unavailable 落 default 500，与既有 knowledge custom 路由一致）。
// 时态语义见 ADR-KN-20261020-J：T 轴存在性过滤 + R 轴“R 时刻已知”，weight/context
// 返回当前值；零 span 字节级委托现路径。

// parseTemporalSpanRFC3339 解析双时态轴（空串 = 轴未设；两段 RFC3339 严格校验）。
// 语法错误先于任何存储/装配校验报 400——客户端先修请求，再谈资源存在性。
func parseTemporalSpanRFC3339(validAt, recordedAt string) (bizknowledge.TemporalSpan, error) {
	var span bizknowledge.TemporalSpan
	if raw := strings.TrimSpace(validAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return span, apierror.BadRequest("KNOWLEDGE", "valid_at must be RFC3339")
		}
		span.ValidAt = t
	}
	if raw := strings.TrimSpace(recordedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return span, apierror.BadRequest("KNOWLEDGE", "recorded_at must be RFC3339")
		}
		span.RecordedAt = t
	}
	return span, nil
}

// asOfSearchBody POST /v1/knowledge/search/asof 请求体：复用 search 现字段集
// （ADR-K；snake_case 与前端 searchKnowledge 现 payload 对齐，免改客户端组装）。
type asOfSearchBody struct {
	CollectionID     string  `json:"collection_id"`
	Query            string  `json:"query"`
	TopK             int     `json:"top_k"`
	MinScore         float32 `json:"min_score"` // 与 proto SearchRequest float 及 biz.MinScore(float32) 对齐
	FilterJSON       string  `json:"filter_json"`
	UseRerank        *bool   `json:"use_rerank"`
	RerankCandidates int     `json:"rerank_candidates"`
	HybridSearch     string  `json:"hybrid_search"`
	PathPrefix       string  `json:"path_prefix"`
	ValidAt          string  `json:"valid_at"`
	RecordedAt       string  `json:"recorded_at"`
}

// ServeSearchAsOf POST /v1/knowledge/search/asof——as-of 时态检索。链路与 Search
// RPC 一致（ACL → query 必填 → 装配检查 → Federated/Router/Retriever 分支），
// 差异仅入参为 JSON 且携带 Temporal；ADR-K body 字段集枚举排除 proto 顶层
// use_eval/rewrite_strategy（strategy 固定空串，等价 RPC 空策略——复杂查询
// 自动 MultiQuery 仍走 adaptive_router.go 内部 pickAutoRewriteStrategy），
// SearchQuery 侧 ExcludePathPrefixes/SkipAccessLog/SessionID/AgentID/UserID 与
// RPC 一致均不复制。
func (s *KnowledgeService) ServeSearchAsOf(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body asOfSearchBody
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeKnowledgeJSONError(w, apierror.BadRequest("KNOWLEDGE", "invalid JSON body"))
				return
			}
		}
	}
	span, err := parseTemporalSpanRFC3339(body.ValidAt, body.RecordedAt)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	colID := strings.TrimSpace(body.CollectionID)
	if colID != "" {
		// C-01：显式单库校验存在性 + 读权限（404 反 IDOR，与 Search RPC 同义）。
		col, err := s.uc.GetCollection(ctx, colID)
		if err != nil {
			writeKnowledgeJSONError(w, err)
			return
		}
		if err := s.assertCollectionAccess(ctx, col); err != nil {
			writeKnowledgeJSONError(w, err)
			return
		}
	}
	query := strings.TrimSpace(body.Query)
	if query == "" {
		writeKnowledgeJSONError(w, apierror.BadRequest("KNOWLEDGE", "query is required"))
		return
	}
	if s.search.Retriever == nil {
		writeKnowledgeJSONError(w, apierror.Unavailable("KNOWLEDGE", "knowledge retriever not configured"))
		return
	}
	q := biz.KnowledgeSearchQuery{
		CollectionID:     colID,
		Query:            query,
		TopK:             body.TopK,
		MinScore:         body.MinScore,
		FilterJSON:       body.FilterJSON,
		RerankCandidates: body.RerankCandidates,
		PathPrefix:       strings.TrimSpace(body.PathPrefix),
		// G-05-asof：双时态窗口（零 span 时 data 层字节级委托现路径）。
		Temporal: span,
	}
	if body.UseRerank != nil {
		v := *body.UseRerank
		q.UseRerank = &v
	}

	var chunks []biz.KnowledgeChunk
	rewriteResult := s.rewriteSearchQuery(ctx, query, "")
	modeOverride := knowledge.ParseHybridSearchMode(body.HybridSearch)

	if q.CollectionID == "" {
		// 与 Search RPC 同分支约定：collection_id 留空走全库智能路由；
		// as-of 全库面允许（边时态在各库内部裁剪）。
		if s.search.Federated == nil {
			writeKnowledgeJSONError(w, apierror.Unavailable("KNOWLEDGE", "federated retriever not configured for collection-free search"))
			return
		}
		chunks, err = s.search.Federated.SearchAll(ctx, q, rewriteResult, modeOverride, workspace.ReadableFilterID(ctx))
	} else if s.search.Router != nil {
		chunks, err = s.search.Router.Search(ctx, q, rewriteResult, modeOverride)
	} else {
		chunks, err = s.search.Retriever.Search(ctx, q)
	}
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}

	out := make([]map[string]any, 0, len(chunks)) // 空集出 "chunks":[]（前端 .map 契约）
	for _, ch := range chunks {
		out = append(out, map[string]any{
			"id":             ch.ID,
			"doc_id":         ch.DocID,
			"collection_id":  ch.CollectionID,
			"content":        ch.Content,
			"metadata_json":  ch.MetadataJSON,
			"chunk_index":    ch.ChunkIndex,
			"score":          ch.Score,
			"dense_score":    ch.DenseScore,
			"sparse_score":   ch.SparseScore,
			"rrf_score":      ch.RRFScore,
			"rerank_score":   ch.RerankScore,
			"parent_content": ch.ParentContent,
		})
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"chunks": out})
}

// ServeCollectionGraphAsOf GET /v1/knowledge/vaults/{id}/graph/asof——as-of 图快照。
// 节点集天然当前（ADR-J「边时态×当前节点集×当前权限」）；边源零 span 时字节级
// 回退当前快照。JSON 形状与 proto ListCollectionGraph 对齐（snake_case，前端
// pickStr 双制式复用现有 mapGraphNode）。
func (s *KnowledgeService) ServeCollectionGraphAsOf(w http.ResponseWriter, r *http.Request, collectionID string) {
	ctx := r.Context()
	qs := r.URL.Query()
	span, err := parseTemporalSpanRFC3339(qs.Get("valid_at"), qs.Get("recorded_at"))
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	col, err := s.uc.GetCollection(ctx, collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	var linkTypes []string
	if raw := strings.TrimSpace(qs.Get("link_types")); raw != "" {
		linkTypes = strings.Split(raw, ",")
	}
	g, err := s.uc.ListCollectionGraphAsOf(ctx, collectionID, linkTypes, strings.TrimSpace(qs.Get("path_prefix")), span)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	nodes := make([]map[string]any, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, map[string]any{
			"doc_id":   n.DocID,
			"name":     n.Name,
			"rel_path": n.RelPath,
			"doc_type": n.DocType,
			"degree":   n.Degree,
		})
	}
	edges := make([]map[string]any, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, map[string]any{
			"source": e.Source,
			"target": e.Target,
			"type":   e.Type,
		})
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges})
}
