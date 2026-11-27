package data

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (r *l3FactRepo) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {

	if r.vectorStore != nil && len(queryEmbedding) > 0 {
		return r.recallL3WithVectorStore(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}
	if r.shouldUseBruteForce(ctx, scopeType, scopeID, userID, queryEmbedding) {
		return r.recallL3FactsBruteForce(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}
	return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
}

// shouldUseBruteForce reports that the active fact count is small enough to
// score in-process. Callers must already have ruled out the vector+RRF path.
func (r *l3FactRepo) shouldUseBruteForce(ctx context.Context, scopeType, scopeID, userID string, _ []float32) bool {
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''", factPIIGateClause}
	args := []any{}
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
	where := " WHERE " + strings.Join(clauses, " AND ")
	var count int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &count); err != nil {
		return false
	}
	threshold := r.bruteForceThreshold
	if threshold <= 0 {
		threshold = biz.DefaultFactBruteForceThreshold
	}

	return count <= threshold
}

// recallL3FactsBruteForce scores the bounded active fact set in-process
// (keyword / importance / recency / optional local cosine). SQL LIMIT is the
// brute-force threshold. embedding_blob is loaded only when the caller
// supplied a query vector and there is no pgvector path.
func (r *l3FactRepo) recallL3FactsBruteForce(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	threshold := r.bruteForceThreshold
	if threshold <= 0 {
		threshold = biz.DefaultFactBruteForceThreshold
	}
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''", factPIIGateClause}
	args := []any{}
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
	where := " WHERE " + strings.Join(clauses, " AND ")
	includeBlob := len(queryEmbedding) > 0
	q := sqlFactSelectSQL(includeBlob) + where + ` LIMIT ?`
	args = append(args, threshold)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	scored := scoreFactRows(ctx, r.data.factCipher, rows, tokenizeQuery(query), queryEmbedding, nil, 0, time.Now().UTC())
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}

	now := time.Now().UTC()
	extra, err := r.ftsExtraCandidates(ctx, scopeType, scopeID, userID, query, queryEmbedding, scoredFactIDs(scored), 0, now)
	if err != nil {
		return nil, err
	}
	scored = append(scored, extra...)
	trgmExtra, err := r.trgmExtraCandidates(ctx, scopeType, scopeID, userID, query, queryEmbedding, scoredFactIDs(scored), 0, now)
	if err != nil {
		return nil, err
	}
	scored = append(scored, trgmExtra...)
	return r.finalizeScoredFacts(query, scored, lim, minScore), nil
}

// finalizeScoredFacts sorts, applies the adaptive minScore floor, truncates,
// reranks, and annotates scored facts into the output JSON rows. Score
// annotation happens after rerank so the persisted breakdown reflects the
// final (possibly cross-encoder-adjusted) total. minScore is not re-applied
// after rerank.
func (r *l3FactRepo) finalizeScoredFacts(query string, scored []scoredFact, lim int, minScore float64) [][]byte {
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if minScore > 0 && len(scored) > 0 {
		eff := biz.AdaptiveRecallMinScore(minScore, scored[0].score)
		kept := make([]scoredFact, 0, len(scored))
		for _, s := range scored {
			if s.score >= eff {
				kept = append(kept, s)
			}
		}
		scored = kept
	}
	if len(scored) > lim {
		scored = scored[:lim]
	}
	passages := make([]string, len(scored))
	for i, s := range scored {
		passages[i] = factPassage(s.stmt, s.details)
	}
	applyCrossEncoderRerankToFactScored(r.data.Reranker(), query, scored, passages)
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	out := make([][]byte, 0, len(scored))
	for _, s := range scored {
		out = append(out, annotateFactScores(s.raw, s.breakdown))
	}
	return out
}

func (r *l3FactRepo) recallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	pool := l3RecallCandidatePool
	if pool < lim {
		pool = lim
	}
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''", factPIIGateClause}
	args := []any{}
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
	where := " WHERE " + strings.Join(clauses, " AND ")
	includeBlob := len(queryEmbedding) > 0
	q := sqlFactSelectSQL(includeBlob) + where + ` ORDER BY COALESCE(NULLIF(valid_from, ''), created_at) DESC LIMIT ?`
	args = append(args, pool)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	scored := scoreFactRows(ctx, r.data.factCipher, rows, tokenizeQuery(query), queryEmbedding, nil, 0, time.Now().UTC())
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	ftsLimit := l3FTSLargeLimit
	if ftsLimit < lim {
		ftsLimit = lim
	}
	now := time.Now().UTC()

	extra, err := r.ftsExtraCandidatesAt(ctx, scopeType, scopeID, userID, query, queryEmbedding, scoredFactIDs(scored), 0, now, ftsLimit)
	if err != nil {
		return nil, err
	}
	scored = append(scored, extra...)
	trgmExtra, err := r.trgmExtraCandidatesAt(ctx, scopeType, scopeID, userID, query, queryEmbedding, scoredFactIDs(scored), 0, now, ftsLimit)
	if err != nil {
		return nil, err
	}
	scored = append(scored, trgmExtra...)
	return r.finalizeScoredFacts(query, scored, lim, minScore), nil
}

func (r *l3FactRepo) recallL3WithVectorStore(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	pool := l3RecallCandidatePool
	if pool < lim {
		pool = lim
	}
	vecHits, err := searchRecallVectors(ctx, r.vectorStore, queryEmbedding, pool, l3VectorFilter(userID))
	if err != nil {

		r.data.lg.Warn("L3 vector search failed, degrading to FTS/recency recall",
			loggateway.StepID("memory.l3_vector_search"),
			loggateway.Err(err))
		vecHits = nil
	}

	ftsIDs := r.ftsCandidateIDs(ctx, scopeType, scopeID, userID, query, pool)
	trgmIDs := r.trgmCandidateIDs(ctx, scopeType, scopeID, userID, query, pool)
	vecIDs := make([]string, 0, len(vecHits))
	hitMap := make(map[string]float64, len(vecHits))
	for _, h := range vecHits {
		vecIDs = append(vecIDs, h.ID)
		hitMap[h.ID] = h.Score
	}
	candidates := uniqueIDs(vecIDs, ftsIDs, trgmIDs)
	if len(candidates) == 0 {
		return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}

	// Scope-filter every leg BEFORE fusing and truncating. Without a user
	// filter the vector leg is a global top-K over a shared table; fusing it
	// first and cutting to pool let out-of-scope vector IDs evict this
	// scope's own FTS/trigram candidates, which the SQL filter then dropped.
	includeBlob := len(queryEmbedding) > 0
	rows, err := r.queryFactRowsByIDs(ctx, candidates, scopeType, scopeID, userID, includeBlob)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	scored := scoreFactRows(ctx, r.data.factCipher, rows, tokenizeQuery(query), queryEmbedding, hitMap, 0, time.Now().UTC())
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	if len(scored) == 0 {
		// Every candidate fell outside the scope (stale or foreign vector
		// rows): recency recall still finds this scope's facts.
		return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}

	inScope := scoredFactIDs(scored)
	rrfScores, fusedOrder := rrfFuseRanked(l3RRFK, keepInSet(vecIDs, inScope), keepInSet(ftsIDs, inScope), keepInSet(trgmIDs, inScope))
	if len(fusedOrder) > pool {
		fusedOrder = fusedOrder[:pool]
	}
	keep := make(map[string]struct{}, len(fusedOrder))
	for _, id := range fusedOrder {
		keep[id] = struct{}{}
	}
	kept := scored[:0]
	for _, s := range scored {
		if _, ok := keep[s.id]; ok {
			s.breakdown.RRF = rrfScores[s.id]
			kept = append(kept, s)
		}
	}
	return r.finalizeScoredFacts(query, kept, lim, minScore), nil
}

// l3VectorFilter maps an L3 recall scope onto vector_embeddings meta. Only
// user_id is mapped: it is copied verbatim from memory_facts.user_id, and
// queryFactRowsByIDs re-applies user_id = ? in SQL. Agent/team scopes keep
// the unfiltered search (meta agent_id falls back to scope_id, which is not a
// safe equality for every scope type).
func l3VectorFilter(userID string) map[string]string {
	if userID == "" {
		return nil
	}
	return map[string]string{"user_id": userID}
}
