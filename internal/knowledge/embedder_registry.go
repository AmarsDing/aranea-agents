package knowledge

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// searchDenseDims 主维检索；登记了异维 embedder 时逐维再查，按 RRF 合并。
// 某一异维失败只跳过该维，主维错误仍上抛。
func (h *HybridRetriever) searchDenseDims(ctx context.Context, q biz.KnowledgeSearchQuery, primary []float32) ([]biz.KnowledgeChunk, error) {
	hits, err := h.searchDenseLeg(ctx, q, primary)
	if err != nil || len(h.extraEmbedders) == 0 {
		return hits, err
	}
	lists := [][]biz.KnowledgeChunk{hits}
	for _, ex := range h.extraEmbedders {
		if ex == nil {
			continue
		}
		vec, eerr := embedSearchQuery(ctx, ex, q.Query)
		if eerr != nil || len(vec) == 0 || len(vec) == len(primary) {
			if eerr != nil {
				h.lg.Warn("跨维查询嵌入失败，跳过该维",
					loggateway.StepID("knowledge.hybrid.dim_embed"),
					loggateway.Err(eerr),
					loggateway.Str("collection_id", q.CollectionID))
			}
			continue
		}
		more, merr := h.searchDenseLeg(ctx, q, vec)
		if merr != nil {
			h.lg.Warn("跨维 dense 检索失败，跳过该维",
				loggateway.StepID("knowledge.hybrid.dim_dense"),
				loggateway.Err(merr),
				loggateway.Str("collection_id", q.CollectionID))
			continue
		}
		lists = append(lists, more)
	}
	if len(lists) == 1 {
		return hits, nil
	}
	return mergeDenseDims(h.rrfK, lists...), nil
}

// mergeDenseDims 把多维 dense 命中当成同权 RRF 列合并。原始分一律记 DenseScore，
// 不占用 sparse 分解槽。
func mergeDenseDims(k int, lists ...[]biz.KnowledgeChunk) []biz.KnowledgeChunk {
	if k <= 0 {
		k = defaultRRF_K
	}
	scores := map[string]float64{}
	denseRaw := map[string]float32{}
	chunkMap := map[string]biz.KnowledgeChunk{}
	for _, list := range lists {
		for i, ch := range list {
			scores[ch.ID] += 1.0 / float64(k+i+1)
			if ch.Score > denseRaw[ch.ID] {
				denseRaw[ch.ID] = ch.Score
			}
			if _, ok := chunkMap[ch.ID]; !ok {
				chunkMap[ch.ID] = biz.KnowledgeChunk{
					ID:            ch.ID,
					DocID:         ch.DocID,
					CollectionID:  ch.CollectionID,
					Content:       ch.Content,
					MetadataJSON:  ch.MetadataJSON,
					ChunkIndex:    ch.ChunkIndex,
					ParentChunkID: ch.ParentChunkID,
				}
			}
		}
	}
	out := make([]biz.KnowledgeChunk, 0, len(chunkMap))
	for id, score := range scores {
		ch := chunkMap[id]
		fused := float32(score)
		ch.Score = fused
		ch.RRFScore = fused
		ch.DenseScore = denseRaw[id]
		out = append(out, ch)
	}
	sortChunksByRRFScoreDesc(out)
	return out
}
