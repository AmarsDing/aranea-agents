package data

import (
	"context"
	"sort"
	"time"
	"unicode/utf8"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// knowledge_temporal_reader.go — G-32 T4 接线 item 4（ADR-KN-20261022-I 决策
// 7/12 声明 → knowledgeRepo 实现）：窄端口方法直接落 knowledgeRepo，经 biz 根
// 装配的 repo.(Interface) 断言接线（与 SetTemporalGraphRepo 同先例）。
//
//   - ChunkArchiveReader（决策 7）：LatestArchivedChunkContents 委托
//     queryChunkArchiveLatestContents（MAX(revision_no) 最新死亡化身）；
//   - ChunkTemporalReader（决策 12）：ListChunksAsOf / RecallChunksAsOf /
//     SparseRecallChunksAsOf——稠密走 queryChunksAsOfANN（per-leg KNN），
//     sparse 四分支 fan-out 与 SearchChunksBM25 同词素口径（针展开复用
//     bizknowledge.LexicalSearchQueries、bigram 复用 CJKBigrams、短针门槛
//     4/8 runes 同值）后 RRF 融合（mergeChunkAsOfRankings，公式与
//     mergeBM25Rankings 同轨 rrfK=60）。
//
// 契约守卫（与 chunk_temporal_port.go 声明逐条对位）：
//   - span.RecordedAt 非零 → ErrUnsupportedOperation（结构性拒绝，谎报二次迹
//     不如拒绝）；
//   - span.ValidAt 零值 → time.Now 退化（防御性兜底——生产时针注入归
//     Usecase 装配持有（端口声明「T4 接线时由 Usecase 装配持有」），Usecase
//     在调用前解析 ValidAt；data 侧重复退化只为防御裸调用方）；
//   - 存活谓词左闭右开/热度副作用关闭/as-of 语义已在各内核函数侧落实，
//     本文件不改写。

var (
	_ bizknowledge.ChunkArchiveReader  = (*knowledgeRepo)(nil)
	_ bizknowledge.ChunkTemporalReader = (*knowledgeRepo)(nil)
)

// resolveAsOfInstant 契约守卫：RecordedAt 非零即结构性拒绝；ValidAt 零值退化
// time.Now（Usecase 装配时针后的防御性兜底）。统一 UTC（document_versions/
// chunk_archive.created_at 皆为 timestamptz，显式 UTC 免除会话时区漂移）。
func resolveAsOfInstant(span bizknowledge.TemporalSpan) (time.Time, error) {
	if span.RecordedOn() {
		return time.Time{}, bizknowledge.ErrUnsupportedOperation
	}
	at := span.ValidAt
	if at.IsZero() {
		at = time.Now()
	}
	return at.UTC(), nil
}

// LatestArchivedChunkContents 按 chunk_id 精确回落最新死亡化身内容（决策 7
// 「有档回落、无档仍弃」语义钉；授权维持 collection 级当下口径，调用方责任）。
func (r *knowledgeRepo) LatestArchivedChunkContents(ctx context.Context, chunkIDs []string) (map[string]string, error) {
	if len(chunkIDs) == 0 {
		return map[string]string{}, nil
	}
	return queryChunkArchiveLatestContents(ctx, r.data.RWDB().ReadDB(ctx), chunkIDs)
}

// ListChunksAsOf 时点截取列表（child 化身全量投影，(doc_id,chunk_index,chunk_id) 序）。
func (r *knowledgeRepo) ListChunksAsOf(ctx context.Context, collectionID string, span bizknowledge.TemporalSpan, docID string) ([]bizknowledge.KnowledgeChunkIncarnation, error) {
	at, err := resolveAsOfInstant(span)
	if err != nil {
		return nil, err
	}
	items, err := listChunksAsOfCore(ctx, r.data.RWDB().ReadDB(ctx), collectionID, at, docID)
	if err != nil {
		return nil, err
	}
	return chunkAsOfItemsToBiz(items), nil
}

// RecallChunksAsOf 双腿 per-leg KNN 时态稠密召回（纯余弦相似度计分）。
func (r *knowledgeRepo) RecallChunksAsOf(ctx context.Context, collectionID string, queryEmbedding []float32, span bizknowledge.TemporalSpan, topK int, docID string) ([]bizknowledge.KnowledgeChunkIncarnation, error) {
	at, err := resolveAsOfInstant(span)
	if err != nil {
		return nil, err
	}
	if len(queryEmbedding) == 0 {
		return nil, nil
	}
	items, err := queryChunksAsOfANN(ctx, r.data.RWDB().ReadDB(ctx), collectionID, queryEmbedding, at, topK, docID)
	if err != nil {
		return nil, err
	}
	return chunkAsOfItemsToBiz(items), nil
}

// SparseRecallChunksAsOf 词法四分支时态召回（决策 12③ sparse 对型）。fan-out
// 与融合形状镜像 SearchChunksBM25：LexicalSearchQueries 针展开每针跑
// tsvector+trigram（短针 ≤4/8 runes 另加 substring）、原查询 CJK bigrams≥2
// 追加 bigram_overlap；分支级容错（单支失败不拖全量，全败返回首错）、
// RRF 收编（mergeChunkAsOfRankings 同轨 rrfK=60）。串行执行（as-of 属低频
// 调查读，读副本单连接串行省并发放大；current 快报的 safego 并发形态
// 保留在 SearchChunksBM25 原路径不动）。
func (r *knowledgeRepo) SparseRecallChunksAsOf(ctx context.Context, collectionID, query string, span bizknowledge.TemporalSpan, topK int, docID string) ([]bizknowledge.KnowledgeChunkIncarnation, error) {
	at, err := resolveAsOfInstant(span)
	if err != nil {
		return nil, err
	}
	queries := bizknowledge.LexicalSearchQueries(query)
	if len(queries) == 0 {
		return nil, nil
	}
	q := r.data.RWDB().ReadDB(ctx)
	rankings := [][]chunkAsOfItem{}
	var firstErr error
	appendBranch := func(items []chunkAsOfItem, branchErr error) {
		if branchErr != nil {
			if firstErr == nil {
				firstErr = branchErr
			}
			return
		}
		if len(items) > 0 {
			rankings = append(rankings, items)
		}
	}
	for i, needle := range queries {
		items, err := queryChunksAsOfTsvector(ctx, q, collectionID, needle, at, topK, docID)
		appendBranch(items, err)
		items, err = queryChunksAsOfTrigram(ctx, q, collectionID, needle, at, topK, docID)
		appendBranch(items, err)
		subLimit := 4
		if i > 0 {
			subLimit = 8
		}
		if utf8.RuneCountInString(needle) <= subLimit {
			items, err = queryChunksAsOfSubstring(ctx, q, collectionID, needle, at, topK, docID)
			appendBranch(items, err)
		}
	}
	if bigrams := bizknowledge.CJKBigrams(query); len(bigrams) >= 2 {
		items, err := queryChunksAsOfBigramOverlap(ctx, q, collectionID, bigrams, at, topK, docID)
		appendBranch(items, err)
	}
	if len(rankings) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, nil
	}
	return chunkAsOfItemsToBiz(mergeChunkAsOfRankings(rankings, topK)), nil
}

// chunkAsOfItemsToBiz data 投影 → biz 化身（Metadata jsonb []byte → string；
// 其余字段直译——形状与 KnowledgeChunkIncarnation 声明逐字段对位）。
func chunkAsOfItemsToBiz(items []chunkAsOfItem) []bizknowledge.KnowledgeChunkIncarnation {
	out := make([]bizknowledge.KnowledgeChunkIncarnation, 0, len(items))
	for _, it := range items {
		out = append(out, bizknowledge.KnowledgeChunkIncarnation{
			ChunkID:       it.ChunkID,
			DocID:         it.DocID,
			CollectionID:  it.CollectionID,
			Content:       it.Content,
			ContentKey:    it.ContentKey,
			ChunkRole:     it.ChunkRole,
			ParentChunkID: it.ParentChunkID,
			ChunkIndex:    it.ChunkIndex,
			CitedCount:    it.CitedCount,
			Metadata:      string(it.Metadata),
			Archived:      it.Archived,
			Score:         it.Score,
		})
	}
	return out
}

// mergeChunkAsOfRankings RRF 融合（与 mergeBM25Rankings 同轨：rrfK=60、分支内
// 归一 weight=score/branchMax、score+=weight/(rrfK+rank+1)、同分 chunk_id 升序
// 稳定收敛；Score 保持 float64 不降级——mergeBM25Rankings 托 merge 在中间产物
// biz.KnowledgeChunk（float32 Score）上做，as-of 直出化身故保全精度）。
func mergeChunkAsOfRankings(rankings [][]chunkAsOfItem, topK int) []chunkAsOfItem {
	const rrfK = 60
	type scoredItem struct {
		item  chunkAsOfItem
		score float64
	}
	byID := make(map[string]*scoredItem)
	for _, ranking := range rankings {
		branchMax := 0.0
		for _, it := range ranking {
			if it.Score > branchMax {
				branchMax = it.Score
			}
		}
		for rank, it := range ranking {
			acc := byID[it.ChunkID]
			if acc == nil {
				cp := it
				acc = &scoredItem{item: cp}
				byID[it.ChunkID] = acc
			}
			weight := 1.0
			if branchMax > 0 {
				weight = it.Score / branchMax
			}
			acc.score += weight / float64(rrfK+rank+1)
		}
	}
	merged := make([]chunkAsOfItem, 0, len(byID))
	for _, acc := range byID {
		acc.item.Score = acc.score
		merged = append(merged, acc.item)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Score == merged[j].Score {
			return merged[i].ChunkID < merged[j].ChunkID
		}
		return merged[i].Score > merged[j].Score
	})
	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}
