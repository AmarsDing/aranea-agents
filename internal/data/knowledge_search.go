package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

// recencyDecayLambda 检索轻时间衰减系数（P1，2026-08-15 评审修订），单位 s⁻¹。
// 取 λ = -ln(0.9) / (180×86400) ≈ 6.774e-9：文档 180 天未更新 ≈ ×0.9，
// 月尺度微调排序，不压倒语义分；5 年旧文 ≈ ×0.34。时间源用
// knowledge_documents.updated_at（人改词条/写回正文才会拨动），不用
// chunks.created_at——整页重插会把旧知识的时间戳刷成「刚才」。
const recencyDecayLambda = 6.774e-9

// recencyScoreSQL 把基础语义分包成「分 × exp(-λ·age) × stale降权」表达式；JOIN 别名 d 固定
// 指向 knowledge_documents。GREATEST 防时钟回拨产生负 age（反而加分）。
// stale 降权（P1-c）：M4 stale 治理置位 d.stale_at 的陈旧词条 ×0.5——降权非排除，
// 陈旧知识仍可命中但让位新鲜知识；内容变更清 NULL 复活。
func recencyScoreSQL(base string) string {
	return fmt.Sprintf(`(%s) * EXP(-%g * GREATEST(EXTRACT(EPOCH FROM (now() - d.updated_at)), 0)) * CASE WHEN d.stale_at IS NULL THEN 1.0 ELSE 0.5 END`, base, recencyDecayLambda)
}

// recencyJoinSQL 是 recencyScoreSQL 配套的 JOIN 子句；主表别名固定 c。
const recencyJoinSQL = `JOIN knowledge_documents d ON d.id = c.doc_id`

// annCandidateLimit 是 IVFFlat 内层窗口：先按向量距离取候选，再在窗口上算
// recency/stale 重排。topK×4，下限 20、上限 80——既让索引可用，又给时间衰减留候选。
func annCandidateLimit(topK int) int {
	if topK <= 0 {
		topK = 5
	}
	n := topK * 4
	if n < 20 {
		n = 20
	}
	if n > 80 {
		n = 80
	}
	return n
}

// denseANNSearchSQL 内层 ORDER BY embedding <=> $1 走 HNSW；外层对候选集
// 套 recency/stale 表达式再 LIMIT topK。clauses 作用于内层 knowledge_chunks
// （无表别名），overfetchPH / topKPH 是 LIMIT 占位序号。
func denseANNSearchSQL(clauses string, overfetchPH, topKPH int) string {
	return fmt.Sprintf(`
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index,
       %s AS score
FROM (
  SELECT id, doc_id, collection_id, content, metadata, chunk_index, embedding
  FROM knowledge_chunks
  WHERE collection_id = $2
    AND embedding IS NOT NULL
    %s
  ORDER BY embedding <=> $1::vector
  LIMIT $%d
) c
%s
ORDER BY score DESC
LIMIT $%d`, recencyScoreSQL(`1 - (c.embedding <=> $1::vector)`), clauses, overfetchPH, recencyJoinSQL, topKPH)
}

func (r *knowledgeRepo) SearchChunks(ctx context.Context, q biz.KnowledgeSearchQuery, queryEmbedding []float32) ([]biz.KnowledgeChunk, error) {
	if len(queryEmbedding) == 0 {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "embedding is empty")
	}
	vec := pgvector.NewVector(queryEmbedding)
	args := []any{vec, q.CollectionID}
	clauses := ""
	if c, a := pathPrefixClause(q.PathPrefix, "$2", len(args)+1); c != "" {
		clauses += "\n  " + c
		args = append(args, a)
	}
	if c, a := pathExcludeClause(q.ExcludePathPrefixes, "$2", len(args)+1); c != "" {
		clauses += "\n  " + c
		args = append(args, a...)
	}
	if q.FilterJSON != "" {
		clauses += fmt.Sprintf("\n  AND metadata @> $%d::jsonb", len(args)+1)
		args = append(args, json.RawMessage(q.FilterJSON))
	}
	if q.MinScore > 0 {

		clauses += fmt.Sprintf("\n  AND (1 - (embedding <=> $1::vector)) >= $%d", len(args)+1)
		args = append(args, q.MinScore)
	}
	if c, a := docChunkVisibilityClause(ctx, "$2", len(args)+1); c != "" {
		clauses += "\n  " + c
		args = append(args, a...)
	}
	overfetch := 0
	if q.TopK > 0 {
		overfetch = annCandidateLimit(q.TopK)
	}
	overfetchPH := len(args) + 1
	args = append(args, overfetch)
	topKPH := len(args) + 1
	args = append(args, q.TopK)
	raw := denseANNSearchSQL(clauses, overfetchPH, topKPH)

	tx, err := r.data.Postgres().BeginTx(ctx, nil)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(hnswEfSearch())); err != nil {
		lg := r.lg
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		lg.Warn("knowledge hnsw.ef_search unavailable",
			loggateway.StepID("data.knowledge.hnsw_ef_search"),
			loggateway.Err(err))
	}
	rows, err := tx.QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()

	var out []biz.KnowledgeChunk
	for rows.Next() {
		var ch biz.KnowledgeChunk
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.CollectionID, &ch.Content, &ch.MetadataJSON, &ch.ChunkIndex, &ch.Score); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	if err := rows.Close(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	if err := tx.Commit(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}

// SearchChunksBM25 词法召回：原查询的 tsvector + trigram（短针另加 substring），
// 并对中文问句抽取内容针后 RRF 融合。Agent 完整问句不得依赖 embedding 才能命中。
func (r *knowledgeRepo) SearchChunksBM25(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	extraClauses := ""
	var extraArgs []any
	if c, a := pathPrefixClause(q.PathPrefix, "$2", 3+len(extraArgs)); c != "" {
		extraClauses += "\n  " + c
		extraArgs = append(extraArgs, a)
	}
	if c, a := pathExcludeClause(q.ExcludePathPrefixes, "$2", 3+len(extraArgs)); c != "" {
		extraClauses += "\n  " + c
		extraArgs = append(extraArgs, a...)
	}
	if q.FilterJSON != "" {
		extraClauses += fmt.Sprintf("\n  AND metadata @> $%d::jsonb", 3+len(extraArgs))
		extraArgs = append(extraArgs, json.RawMessage(q.FilterJSON))
	}
	if c, a := docChunkVisibilityClause(ctx, "$2", 3+len(extraArgs)); c != "" {
		extraClauses += "\n  " + c
		extraArgs = append(extraArgs, a...)
	}

	var searches []func() ([]biz.KnowledgeChunk, error)
	for i, query := range bizknowledge.LexicalSearchQueries(q.Query) {
		query := query
		one := q
		one.Query = query
		searches = append(searches, func() ([]biz.KnowledgeChunk, error) {
			return r.searchChunksTsvector(ctx, one, extraClauses, extraArgs)
		})
		searches = append(searches, func() ([]biz.KnowledgeChunk, error) {
			return r.searchChunksTrigram(ctx, one, extraClauses, extraArgs)
		})

		subLimit := 4
		if i > 0 {
			subLimit = 8
		}
		if utf8.RuneCountInString(query) <= subLimit {
			searches = append(searches, func() ([]biz.KnowledgeChunk, error) {
				return r.searchChunksSubstring(ctx, one, extraClauses, extraArgs)
			})
		}
	}

	if bigrams := bizknowledge.CJKBigrams(q.Query); len(bigrams) >= 2 {
		one := q
		searches = append(searches, func() ([]biz.KnowledgeChunk, error) {
			return r.searchChunksBigramOverlap(ctx, one, bigrams, extraClauses, extraArgs)
		})
	}

	results := make([][]biz.KnowledgeChunk, len(searches))
	errs := make([]error, len(searches))
	var wg sync.WaitGroup
	for i, search := range searches {
		wg.Add(1)
		i, search := i, search
		safego.Go(ctx, "knowledge.search.lexical_branch", func() {
			defer wg.Done()
			results[i], errs[i] = search()
		})
	}
	wg.Wait()

	successful := results[:0]
	var firstErr error
	for i := range results {
		if errs[i] == nil {
			successful = append(successful, results[i])
		} else if firstErr == nil {
			firstErr = errs[i]
		}
	}
	if len(successful) == 0 {
		return nil, firstErr
	}
	return mergeBM25Rankings(successful, q.TopK), nil
}

func (r *knowledgeRepo) searchChunksTsvector(ctx context.Context, q biz.KnowledgeSearchQuery, extraClauses string, extraArgs []any) ([]biz.KnowledgeChunk, error) {
	raw := fmt.Sprintf(`
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND to_tsvector('simple', c.content) @@ plainto_tsquery('simple', $1)
  %s
ORDER BY score DESC
LIMIT $%d`, recencyScoreSQL(`ts_rank(to_tsvector('simple', c.content), plainto_tsquery('simple', $1))`), recencyJoinSQL, extraClauses, 3+len(extraArgs))

	args := []any{q.Query, q.CollectionID}
	args = append(args, extraArgs...)
	args = append(args, q.TopK)
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunks(rows)
}

func (r *knowledgeRepo) searchChunksTrigram(ctx context.Context, q biz.KnowledgeSearchQuery, extraClauses string, extraArgs []any) ([]biz.KnowledgeChunk, error) {

	raw := fmt.Sprintf(`
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND c.content %%> $1
  %s
ORDER BY score DESC
LIMIT $%d`, recencyScoreSQL(`word_similarity($1, c.content)`), recencyJoinSQL, extraClauses, 3+len(extraArgs))

	args := []any{q.Query, q.CollectionID}
	args = append(args, extraArgs...)
	args = append(args, q.TopK)
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunks(rows)
}

// searchChunksBigramOverlap（P1-3）：按命中的 CJK bigram 数计分。
// 只数 bigram 是否出现于正文（position > 0），不做词频——问句检索里
// 「命中多少个不同 2 字词」比单字词频更接近语义重叠。
func (r *knowledgeRepo) searchChunksBigramOverlap(ctx context.Context, q biz.KnowledgeSearchQuery, bigrams []string, extraClauses string, extraArgs []any) ([]biz.KnowledgeChunk, error) {
	raw := fmt.Sprintf(`
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND (SELECT COUNT(*) FROM unnest($1::text[]) bg WHERE position(bg in c.content) > 0) > 0
  %s
ORDER BY score DESC
LIMIT $%d`, recencyScoreSQL(`(SELECT COUNT(*) FROM unnest($1::text[]) bg WHERE position(bg in c.content) > 0)::float4`), recencyJoinSQL, extraClauses, 3+len(extraArgs))

	args := []any{pq.Array(bigrams), q.CollectionID}
	args = append(args, extraArgs...)
	args = append(args, q.TopK)
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunks(rows)
}

func (r *knowledgeRepo) searchChunksSubstring(ctx context.Context, q biz.KnowledgeSearchQuery, extraClauses string, extraArgs []any) ([]biz.KnowledgeChunk, error) {
	raw := fmt.Sprintf(`
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND position(lower($1) in lower(c.content)) > 0
  %s
ORDER BY score DESC
LIMIT $%d`, recencyScoreSQL(`1.0`), recencyJoinSQL, extraClauses, 3+len(extraArgs))

	args := []any{q.Query, q.CollectionID}
	args = append(args, extraArgs...)
	args = append(args, q.TopK)
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunks(rows)
}

func mergeBM25Rankings(rankings [][]biz.KnowledgeChunk, topK int) []biz.KnowledgeChunk {
	const rrfK = 60
	type scoredChunk struct {
		chunk biz.KnowledgeChunk
		score float64
	}
	byID := make(map[string]*scoredChunk)
	for _, ranking := range rankings {

		branchMax := 0.0
		for _, chunk := range ranking {
			if s := float64(chunk.Score); s > branchMax {
				branchMax = s
			}
		}
		for rank, chunk := range ranking {
			item := byID[chunk.ID]
			if item == nil {
				copy := chunk
				item = &scoredChunk{chunk: copy}
				byID[chunk.ID] = item
			}
			weight := 1.0
			if branchMax > 0 {
				weight = float64(chunk.Score) / branchMax
			}
			item.score += weight / float64(rrfK+rank+1)
		}
	}
	merged := make([]biz.KnowledgeChunk, 0, len(byID))
	for _, item := range byID {
		item.chunk.Score = float32(item.score)
		merged = append(merged, item.chunk)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Score == merged[j].Score {
			return merged[i].ID < merged[j].ID
		}
		return merged[i].Score > merged[j].Score
	})
	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}
