package knowledge

import (
	"context"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
)

// temporal_search.go — G-32 T4 item 4 服务/RPC 装配（ADR-KN-20261022-I 决策 12③
// as-of chunk 双腿进检索管线）：SearchQuery.Temporal 非零时 dense/sparse 腿
// 换时态读端口（repo 断言 bizknowledge.ChunkTemporalReader，照 SparseSearcher
// 先例）；span 零值 = current 快报原路径逐字节不动（G-05 条款①）。
//
// 接线语义锚点：
//   - 授权：内核侧已按决策 12③「授权用当下、内容用历史」落行级可见性＋
//     tombstone 文档 collection 级复合谓词（asOfVisibilityClause）；本层不再
//     叠加 ACL；
//   - 副作用：热度读写（access_log/base-level/Hebbian）由既有 q.Temporal.Any()
//     守卫关断（retriever.logAccess、adaptive_router 两枚），本层零横切；
//   - 未接线拒绝：span 非零而 repo 未实现时态端口 → 结构性 Unavailable，
//     不蜕化为 current 快报（谎报时态等价不如拒绝，与 Usecase 侧同原则）；
//   - span.ValidAt 零值而仅 R 轴启用 → 时态端 ErrUnsupportedOperation（chunks
//     无 recorded 轴台账）；span 合法形态走 per-leg KNN / 词法四分支 RRF。
//   - 计时观测：as-of 腿复用既有 dense/sparse stage 探针（调用点原序不动）。

// asOfChunkReader 断言时态读端口（primaries 按偏好序探——dense 池先；两腿
// 恒同一 knowledgeRepo 实现，断言分层只为 fake repo 共存形态宽容）。
func asOfChunkReader(q biz.KnowledgeSearchQuery, primaries ...any) bizknowledge.ChunkTemporalReader {
	if !q.Temporal.Any() {
		return nil
	}
	for _, p := range primaries {
		if tr, ok := p.(bizknowledge.ChunkTemporalReader); ok {
			return tr
		}
	}
	return nil
}

// asOfUnavailable span 非零而端口未接线的结构性拒绝。
var errAsOfNotWired = apierror.Unavailable(apierror.DomainKnowledge, "temporal chunk search not wired")

// searchChunksDenseAsOf dense 时态腿（per-leg KNN，纯余弦计分；topK<=0 退化
// 内核默认 5——retriever 调用前已归一）。
func searchChunksDenseAsOf(ctx context.Context, tr bizknowledge.ChunkTemporalReader, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	items, err := tr.RecallChunksAsOf(ctx, q.CollectionID, vec, q.Temporal, q.TopK, "")
	if err != nil {
		return nil, err
	}
	return chunkIncarnationsToChunks(items), nil
}

// searchChunksSparseAsOf sparse 时态腿（词法四分支 RRF，分支原生分直出）。
func searchChunksSparseAsOf(ctx context.Context, tr bizknowledge.ChunkTemporalReader, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	items, err := tr.SparseRecallChunksAsOf(ctx, q.CollectionID, q.Query, q.Temporal, q.TopK, "")
	if err != nil {
		return nil, err
	}
	return chunkIncarnationsToChunks(items), nil
}

// chunkIncarnationsToChunks 化身 → 检索 Chunk 投影：字段直译，Embedding 恒
// nil（化身读面不回填向量——下游只消费 Content/Score/id 与回填旁路的
// ParentChunkID；向量再嵌属独立决策不含本批）。Score float64→float32 精度
// 收敛与 current 腿 scanChunks 输出同型。
func chunkIncarnationsToChunks(items []bizknowledge.KnowledgeChunkIncarnation) []biz.KnowledgeChunk {
	out := make([]biz.KnowledgeChunk, 0, len(items))
	for _, it := range items {
		out = append(out, biz.KnowledgeChunk{
			ID:            it.ChunkID,
			DocID:         it.DocID,
			CollectionID:  it.CollectionID,
			Content:       it.Content,
			MetadataJSON:  it.Metadata,
			ChunkIndex:    it.ChunkIndex,
			ParentChunkID: it.ParentChunkID,
			Score:         float32(it.Score),
		})
	}
	return out
}

// ── 调用点腿路由（T4 item 4 装配）─────────────────────────────────────────
// HybridRetriever（Router 在役路径）与 Retriever（退化/直搜路径）双腿统一在本
// 文件落路由方法：span 零值分支恒直被调原函数，谓词文本零改动（G-05 条款①）。

// searchDenseLeg dense 腿路由：span 非零 → 时态 per-leg KNN（未接线 → 结构性
// Unavailable）；span 零值 → data SearchChunks 原路径。
func (h *HybridRetriever) searchDenseLeg(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	if q.Temporal.Any() {
		tr := asOfChunkReader(q, h.dense, h.sparse)
		if tr == nil {
			return nil, errAsOfNotWired
		}
		return searchChunksDenseAsOf(ctx, tr, q, vec)
	}
	return h.dense.SearchChunks(ctx, q, vec)
}

// searchSparseLeg sparse 腿路由：span 非零 → 时态词法四分支 RRF；span 零值 →
// 原 BM25。注意调用点（searchSparse / searchRRF goroutine）构图不同但同一实现。
func (h *HybridRetriever) searchSparseLeg(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if q.Temporal.Any() {
		tr := asOfChunkReader(q, h.sparse, h.dense)
		if tr == nil {
			return nil, errAsOfNotWired
		}
		return searchChunksSparseAsOf(ctx, tr, q)
	}
	return h.sparse.SearchChunksBM25(ctx, q)
}

// searchDenseLeg dense 腿路由（Retriever 直搜路径同口径）。
func (r *Retriever) searchDenseLeg(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	if q.Temporal.Any() {
		tr := asOfChunkReader(q, r.repo)
		if tr == nil {
			return nil, errAsOfNotWired
		}
		return searchChunksDenseAsOf(ctx, tr, q, vec)
	}
	return r.repo.SearchChunks(ctx, q, vec)
}

// searchSparseLeg sparse 腿路由（Retriever embed 降级路径同口径；ss 是
// searchSparseFallback 内断言好的 SparseSearcher）。
func (r *Retriever) searchSparseLeg(ctx context.Context, ss SparseSearcher, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if q.Temporal.Any() {
		tr := asOfChunkReader(q, ss, r.repo)
		if tr == nil {
			return nil, errAsOfNotWired
		}
		return searchChunksSparseAsOf(ctx, tr, q)
	}
	return ss.SearchChunksBM25(ctx, q)
}
