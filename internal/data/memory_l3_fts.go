package data

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
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
// Non-Postgres dialects and blank queries return nil — callers degrade to
// vector-only / Go-keyword scoring.
func (r *l3FactRepo) searchL3FTS(ctx context.Context, scopeType, scopeID, userID, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if r == nil || r.data == nil || query == "" || !r.data.Dialect().IsPostgres() {
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
func (r *l3FactRepo) ftsCandidateIDs(ctx context.Context, scopeType, scopeID, userID, query string, limit int) []string {
	ids, err := r.searchL3FTS(ctx, scopeType, scopeID, userID, query, limit)
	if err != nil {
		r.data.lg.Warn("L3 FTS candidate search failed, degrading to non-FTS recall",
			loggateway.StepID("memory.l3_fts_search"),
			loggateway.Err(err))
		return nil
	}
	return ids
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
// keyword-strong facts with low importance or old timestamps.
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
