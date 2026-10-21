package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// ──────────────────────────────────────────────────────────
// L3 FTS candidate search + RRF fusion (P2-3)
//
// Design (report §6.4 读取路径): the L3 recall pool is now fed by THREE
// complementary retrieval signals — pgvector semantic search, PostgreSQL
// FTS (to_tsvector over statement + details_markdown), and pg_trgm
// word_similarity on statement — fused with Reciprocal Rank Fusion at the
// CANDIDATE-SET level. The final ranking still uses the calibrated hybrid
// score (keyword/vector/importance/recency/quality), so minScore semantics
// are unchanged; RRF decides WHICH facts get scored, not their final order.
// The fused RRF score is annotated into the score breakdown for observability
// only.
//
// FTS 'simple' does not segment CJK runs, so continuous Chinese text forms a
// single token. CJK keyword matching uses the Go substring channel
// (keywordOverlapScore) plus the trigram channel (searchL3Trigram). FTS
// covers alphanumeric tokens (codes, names, IDs) that both vector similarity
// and substring keyword channels handle poorly.
// ──────────────────────────────────────────────────────────

const (
	// l3FTSCandidateLimit caps how many FTS-ranked candidates enter the
	// recall pool. FTS is a complementary signal; the hybrid score re-ranks.
	l3FTSCandidateLimit = 40
	// l3FTSLargeLimit is the FTS pool when the no-embedding path cannot
	// full-scan (fact count above the brute-force threshold).
	l3FTSLargeLimit = 200
	// l3RRFK is the Reciprocal Rank Fusion constant (k=60, same as the
	// knowledge module's hybrid retriever) — dampens head-of-list dominance.
	l3RRFK = 60
)

// searchL3FTS returns FTS-ranked fact IDs (best first) for the query within
// the given scope, using ts_rank over statement + details_markdown.
// Blank queries return nil — callers degrade to vector-only / Go-keyword
// scoring.
func (r *l3FactRepo) searchL3FTS(ctx context.Context, scopeType, scopeID, userID, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if r == nil || r.data == nil || query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	tsv := `to_tsvector('simple', statement || ' ' || COALESCE(details_markdown, ''))`
	tsq := buildL3FTSQuery(query)
	clauses := []string{
		"status = 'active'", "deleted_at = ''", "valid_until = ''",
		tsv + ` @@ to_tsquery('simple', ?)`,
	}
	args := []any{tsq}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	q := `SELECT id FROM memory_facts WHERE ` + strings.Join(clauses, " AND ") +
		` ORDER BY ts_rank(` + tsv + `, to_tsquery('simple', ?)) DESC, COALESCE(NULLIF(valid_from, ''), created_at) DESC LIMIT ?`
	args = append(args, tsq, limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "MEMORY_L3")
		}
		ids = append(ids, id)
	}
	return ids, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// ftsCandidateIDs is the best-effort wrapper around searchL3FTS for the
// recall hot path: FTS errors degrade to no FTS candidates (vector /
// Go-keyword scoring continues) rather than failing the recall.
// AMC C3-B: the ranked English list is merged with the CJK bigram channel
// (searchL3FTSCJK, ranked first-list-wins dedup) so 中文部分词面查询也进
// 候选池；英文 FTS 行为不削弱（英文通道仍是主列表）。
func (r *l3FactRepo) ftsCandidateIDs(ctx context.Context, scopeType, scopeID, userID, query string, limit int) []string {
	ids, err := r.searchL3FTS(ctx, scopeType, scopeID, userID, query, limit)
	if err != nil {
		r.data.lg.Warn("L3 FTS candidate search failed, degrading to non-FTS recall",
			loggateway.StepID("memory.l3_fts_search"),
			loggateway.Err(err))
		ids = nil
	}
	return mergeRankedIDLists(ids, r.ftsCJKCandidateIDs(ctx, scopeType, scopeID, userID, query, limit))
}

// ──────────────────────────────────────────────────────────
// L3 CJK bigram FTS channel (AMC C3-B, migration 20261292)
//
// to_tsvector('simple') 把连续中文整段当单一 lexeme，英文 FTS 通道只能命中
// 整段子串；20261292 的函数式 GIN 索引对 bigram_text(statement) 的滑窗
// 二元组建 tsvector（修复 #9 后查询侧以同一表达式
// to_tsvector('simple', bigram_text(statement)) 命中索引，不再依赖生成列），
// 使「喜欢什么颜色」可经共享二元组命中「喜欢蓝色」，与 trgm 通道
// 互补。查询词用库内同一 bigram_text() 分词，词表永不与索引漂移。
// 老库未迁移 20261292（bigram_text 42883；r1 遗留列时代 42703）时
// 记 debug 并跳过本通道，英文 FTS 与原召回路径不受影响。
// ──────────────────────────────────────────────────────────

// hasCJKRune reports whether s contains a CJK Unified Ideograph — the gate
// for the CJK bigram channel (pure-ASCII queries stay on searchL3FTS).
func hasCJKRune(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// isMissingCJKFTS reports err means the 20261292 objects are absent on an
// unmigrated DB: 42883 undefined_function (bigram_text)。对 r1 库曾按
// STORED 列设计建过 statement_fts_cjk 的环境，42703 undefined_column
// 也视为可降级缺席（查询侧已改走函数表达式，不再引用任何生成列）。
func isMissingCJKFTS(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case "42703", "42883":
			return true
		}
	}
	return false
}

// searchL3FTSCJK returns CJK bigram FTS-ranked fact IDs (best first) via the
// 20261292 functional GIN index on to_tsvector('simple', bigram_text(statement))
// (修复 #9：索引表达式与查询表达式逐字一致，PG 才能匹配函数式索引；
// 原 STORED 生成列方案因全表重写在 #9 改为函数式)。Queries without CJK
// runes return nil (callers keep the English channel only). Missing-migration
// errors degrade to nil (debug-logged); other errors keep entErrToBizErr
// semantics.
func (r *l3FactRepo) searchL3FTSCJK(ctx context.Context, scopeType, scopeID, userID, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if r == nil || r.data == nil || query == "" || !hasCJKRune(query) {
		return nil, nil
	}
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	var bigrams string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT bigram_text(?)`), []any{query}, &bigrams)
	if err != nil {
		if isMissingCJKFTS(err) {
			r.data.lg.Debug("L3 CJK FTS channel unavailable (migration 20261292 not applied), skipping",
				loggateway.StepID("memory.l3_fts_cjk_tokenize"),
				loggateway.Err(err))
			return nil, nil
		}
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	tsq := buildCJKBigramTSQuery(bigrams)
	if tsq == "" {
		return nil, nil
	}
	// user_id 是唯一检索/写入隔离边界：scope 过滤始终强制（'' = 该维不限）。
	// cjkFTSExpr 必须与 20261292 函数式 GIN 索引表达式逐字一致（含括号内
	// 中间空格归一由 PG 解析器规整），否则 planner 无法匹配索引。
	cjkFTSExpr := `to_tsvector('simple', bigram_text(statement))`
	clauses := []string{
		"status = 'active'", "deleted_at = ''", "valid_until = ''",
		`(? = '' OR scope_type = ?)`,
		`(? = '' OR scope_id = ?)`,
		`(? = '' OR user_id = ?)`,
		cjkFTSExpr + ` @@ to_tsquery('simple', ?)`,
	}
	args := []any{scopeType, scopeType, scopeID, scopeID, userID, userID, tsq}
	q := `SELECT id FROM memory_facts WHERE ` + strings.Join(clauses, " AND ") +
		` ORDER BY ts_rank(` + cjkFTSExpr + `, to_tsquery('simple', ?)) DESC, COALESCE(NULLIF(valid_from, ''), created_at) DESC LIMIT ?`
	args = append(args, tsq, limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		if isMissingCJKFTS(err) {
			r.data.lg.Debug("L3 CJK FTS column unavailable (migration 20261292 not applied), skipping",
				loggateway.StepID("memory.l3_fts_cjk_search"),
				loggateway.Err(err))
			return nil, nil
		}
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "MEMORY_L3")
		}
		ids = append(ids, id)
	}
	return ids, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// ftsCJKCandidateIDs is the best-effort wrapper around searchL3FTSCJK: CJK
// channel errors degrade to no CJK candidates rather than failing recall.
func (r *l3FactRepo) ftsCJKCandidateIDs(ctx context.Context, scopeType, scopeID, userID, query string, limit int) []string {
	ids, err := r.searchL3FTSCJK(ctx, scopeType, scopeID, userID, query, limit)
	if err != nil {
		r.data.lg.Warn("L3 CJK FTS candidate search failed, degrading to non-CJK-FTS recall",
			loggateway.StepID("memory.l3_fts_cjk_search"),
			loggateway.Err(err))
		return nil
	}
	return ids
}

// mergeRankedIDLists concatenates ranked ID lists, dropping blanks and
// duplicates while preserving first-seen order — pass the primary list first
// so its ranks dominate downstream RRF fusion.
func mergeRankedIDLists(lists ...[]string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, ids := range lists {
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildCJKBigramTSQuery turns bigram_text() output (space-separated CJK
// bigrams + lowercase ASCII words) into an OR tsquery, mirroring
// buildL3FTSQuery's question-form tolerance: any shared bigram ranks, ts_rank
// plus the hybrid score does the precision work downstream.
func buildCJKBigramTSQuery(bigrams string) string {
	seen := map[string]struct{}{}
	parts := []string{}
	for _, tok := range strings.Fields(bigrams) {
		term := escapeTSQueryTerm(tok)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		parts = append(parts, term)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// rrfFuseRanked merges ranked ID lists via Reciprocal Rank Fusion:
// fused(id) = Σ_lists 1/(k + rank). Returns the fused score per ID and the
// union of IDs ordered by fused score descending (ties keep first-seen
// order, so pass the primary list first).
func rrfFuseRanked(k int, lists ...[]string) (map[string]float64, []string) {
	if k <= 0 {
		k = l3RRFK
	}
	scores := map[string]float64{}
	order := []string{}
	seen := map[string]struct{}{}
	for _, ids := range lists {
		for rank, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			scores[id] += 1.0 / float64(k+rank+1)
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				order = append(order, id)
			}
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	return scores, order
}

// queryFactRowsByIDs fetches active fact rows by ID with scope filters
// re-applied in SQL (defense-in-depth; the candidate queries already filter).
func (r *l3FactRepo) queryFactRowsByIDs(ctx context.Context, ids []string, scopeType, scopeID, userID string, includeBlob bool) (*sql.Rows, error) {
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
	args := make([]any, 0, len(ids)+3)
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	clauses = append(clauses, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	q := sqlFactSelectSQL(includeBlob) + " WHERE " + strings.Join(clauses, " AND ")
	return r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
}

// ftsExtraCandidates fetches and scores FTS-hit facts that are not already in
// the present (already-pooled) set. Used by the brute-force and recency-pool
// recall paths, whose SQL pre-limit (importance/updated_at top-N) can miss
// keyword-strong facts with low importance or old timestamps. The FTS hit set
// is ftsCandidateIDs — English FTS merged with the CJK bigram channel
// (20261292), so 中文事实同样可被本注入路径救回。
func (r *l3FactRepo) ftsExtraCandidates(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, present map[string]struct{}, minScore float64, now time.Time) ([]scoredFact, error) {
	return r.ftsExtraCandidatesAt(ctx, scopeType, scopeID, userID, query, queryEmbedding, present, minScore, now, l3FTSCandidateLimit)
}

func (r *l3FactRepo) ftsExtraCandidatesAt(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, present map[string]struct{}, minScore float64, now time.Time, limit int) ([]scoredFact, error) {
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	ftsIDs := r.ftsCandidateIDs(ctx, scopeType, scopeID, userID, query, limit)
	if len(ftsIDs) == 0 {
		return nil, nil
	}
	missing := make([]string, 0, len(ftsIDs))
	for _, id := range ftsIDs {
		if _, ok := present[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	rows, err := r.queryFactRowsByIDs(ctx, missing, scopeType, scopeID, userID, len(queryEmbedding) > 0)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	scored := scoreFactRows(rows, tokenizeQuery(query), queryEmbedding, nil, minScore, now)
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return scored, nil
}

// buildL3FTSQuery turns a natural-language query into an OR tsquery of
// content tokens. Chat users ask questions ("Alice 喜欢什么颜色"), not
// Boolean AND of every word; AND-of-all-terms misses evidence that uses
// different function words. Ranking still uses ts_rank + hybrid score.
func buildL3FTSQuery(query string) string {
	query = strings.TrimSpace(query)
	tokens := tokenizeQuery(query)
	var parts []string
	seen := map[string]struct{}{}
	for _, tok := range tokens {
		term := escapeTSQueryTerm(tok)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		parts = append(parts, term)
	}
	if len(parts) == 0 {
		if fallback := escapeTSQueryTerm(query); fallback != "" {
			return fallback
		}
		return "empty"
	}
	return strings.Join(parts, " | ")
}

func escapeTSQueryTerm(tok string) string {
	var b strings.Builder
	b.Grow(len(tok))
	for _, r := range tok {
		switch r {
		case '&', '|', '!', '(', ')', ':', '\'', '\\', '<', '>':
			continue
		default:
			if r > 32 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// scoredFactIDs returns the ID set of an already-scored candidate list.
func scoredFactIDs(scored []scoredFact) map[string]struct{} {
	out := make(map[string]struct{}, len(scored))
	for _, s := range scored {
		out[s.id] = struct{}{}
	}
	return out
}
