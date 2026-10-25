package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

// ListChunksByDocumentsForSearch keeps graph expansion/card projection inside
// the same path, metadata and ACL scope as the initial lexical/vector search.
func (r *knowledgeRepo) ListChunksByDocumentsForSearch(ctx context.Context, q biz.KnowledgeSearchQuery, docIDs []string, limitPerDoc int) ([]biz.KnowledgeChunk, error) {
	if strings.TrimSpace(q.CollectionID) == "" || len(docIDs) == 0 || limitPerDoc <= 0 {
		return nil, nil
	}
	args := []any{q.CollectionID, pq.Array(docIDs)}
	clauses := ""
	if c, a := pathPrefixClause(q.PathPrefix, "$1", len(args)+1); c != "" {
		clauses += "\n" + c
		args = append(args, a)
	}
	if c, a := pathExcludeClause(q.ExcludePathPrefixes, "$1", len(args)+1); c != "" {
		clauses += "\n" + c
		args = append(args, a...)
	}
	if q.FilterJSON != "" {
		clauses += fmt.Sprintf("\nAND metadata @> $%d::jsonb", len(args)+1)
		args = append(args, json.RawMessage(q.FilterJSON))
	}
	if c, a := docChunkVisibilityClause(ctx, "$1", len(args)+1); c != "" {
		clauses += "\n" + c
		args = append(args, a...)
	}
	limitPH := len(args) + 1
	args = append(args, limitPerDoc)
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index, parent_chunk_id, 0::float4 AS score
FROM (
  SELECT id, doc_id, collection_id, content, metadata, chunk_index, parent_chunk_id,
         ROW_NUMBER() OVER (PARTITION BY doc_id ORDER BY chunk_index) AS rn
  FROM knowledge_chunks
  WHERE collection_id = $1 AND chunk_role = 'child' AND doc_id = ANY($2)
  %s
) t
WHERE rn <= $%d
ORDER BY doc_id, chunk_index`, clauses, limitPH)
	rows, err := r.data.PostgresRead().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunks(rows)
}

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

// denseANNSearchSQL 内层 JOIN chunk_embeddings 侧表取当前 dim 向量，ORDER BY
// ce.embedding::vector(N) <=> $1 走 per-dim partial HNSW（索引建在 cast 表达式
// 上——pgvector 不接受裸型列建 opclass，查询侧必须用同一表达式方能命中）；
// 外层对候选集套 recency/stale 表达式再 LIMIT topK。clauses 作用于内层（主表
// 别名 c，doc_id 引用必须经 qualifyChunkDocClause 限定——JOIN 后 chunks/
// chunk_embeddings 同名 doc_id 并存，未限定引用 PG 42702），overfetchPH /
// topKPH 是 LIMIT 占位序号。
// G-39（ADR-KN-20261021-G）：dim 以查询向量长度（Go 侧 len）字面值内联
// （注入安全：纯 int），令 partial 谓词对 generic plan 亦可证；`ce.dim = N`
// + cast 使跨维度比较（22P02）结构性不可能——比较只发生在同 dim 桶内。
// G-06：chunk_role='child' 谓词显式排除父块语境行（侧表无父块行本已挡，
// 统一谓词防未来父子检索层演化时破口子）。ce.embedding IS NOT NULL 是对
// 侧表列可空（schema 未加 NOT NULL）的兜底——在树写路径恒非空，但一行越
// 权脏行（手工回填/未来迁移）不应让外层 score Scan NULL 而当掉整条 dense
// 腿（卡片档同款 IS NOT NULL 对称）。
func denseANNSearchSQL(clauses string, dim, overfetchPH, topKPH int) string {
	return fmt.Sprintf(`
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index, c.parent_chunk_id,
       %s AS score
FROM (
  SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata, c.chunk_index, c.parent_chunk_id, ce.embedding
  FROM knowledge_chunks c
  JOIN chunk_embeddings ce ON ce.chunk_id = c.id AND ce.dim = %d
  WHERE c.collection_id = $2
    AND c.chunk_role = 'child'
    AND ce.embedding IS NOT NULL
    %s
  ORDER BY ce.embedding::vector(%d) <=> $1::vector
  LIMIT $%d
) c
%s
ORDER BY score DESC
LIMIT $%d`, recencyScoreSQL(`1 - (c.embedding <=> $1::vector)`), dim, clauses, dim, overfetchPH, recencyJoinSQL, topKPH)
}

// qualifyChunkDocClause 把可见性/路径子句中未限定表的 `AND doc_id ` 前缀改写
// 为 `AND c.doc_id `（G-39 侧表 JOIN 后 chunks c 与 chunk_embeddings ce 同名
// doc_id 列并存，未限定引用 PG 报 42702 ambiguous）。仅 dense 召回构建处
// 使用（子句生成器契约：doc_id 引用恒以 "AND doc_id " 前缀出现）。
func qualifyChunkDocClause(clause string) string {
	return strings.Replace(clause, "AND doc_id ", "AND c.doc_id ", 1)
}

func (r *knowledgeRepo) SearchChunks(ctx context.Context, q biz.KnowledgeSearchQuery, queryEmbedding []float32) ([]biz.KnowledgeChunk, error) {
	if len(queryEmbedding) == 0 {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "embedding is empty")
	}
	vec := pgvector.NewVector(queryEmbedding)
	args := []any{vec, q.CollectionID}
	clauses := ""
	// doc_id 子句必须限定到主表别名 c（G-39 侧表 JOIN 后 doc_id 双名并存）。
	if c, a := pathPrefixClause(q.PathPrefix, "$2", len(args)+1); c != "" {
		clauses += "\n  " + qualifyChunkDocClause(c)
		args = append(args, a)
	}
	if c, a := pathExcludeClause(q.ExcludePathPrefixes, "$2", len(args)+1); c != "" {
		clauses += "\n  " + qualifyChunkDocClause(c)
		args = append(args, a...)
	}
	if q.FilterJSON != "" {
		clauses += fmt.Sprintf("\n  AND metadata @> $%d::jsonb", len(args)+1)
		args = append(args, json.RawMessage(q.FilterJSON))
	}
	if q.MinScore > 0 {

		clauses += fmt.Sprintf("\n  AND (1 - (ce.embedding <=> $1::vector)) >= $%d", len(args)+1)
		args = append(args, q.MinScore)
	}
	if c, a := docChunkVisibilityClause(ctx, "$2", len(args)+1); c != "" {
		clauses += "\n  " + qualifyChunkDocClause(c)
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
	raw := denseANNSearchSQL(clauses, len(queryEmbedding), overfetchPH, topKPH)

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
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.CollectionID, &ch.Content, &ch.MetadataJSON, &ch.ChunkIndex, &ch.ParentChunkID, &ch.Score); err != nil {
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
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index, c.parent_chunk_id,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND c.chunk_role = 'child'
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
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index, c.parent_chunk_id,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND c.chunk_role = 'child'
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
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index, c.parent_chunk_id,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND c.chunk_role = 'child'
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
SELECT c.id, c.doc_id, c.collection_id, c.content, c.metadata::text, c.chunk_index, c.parent_chunk_id,
       %s AS score
FROM knowledge_chunks c
%s
WHERE c.collection_id = $2
  AND c.chunk_role = 'child'
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
