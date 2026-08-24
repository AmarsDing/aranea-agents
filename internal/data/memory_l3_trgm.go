package data

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// searchL3Trigram returns trigram-ranked fact IDs (best first) for CJK and
// short queries. FTS 'simple' treats a Chinese run as one token, so
// word_similarity (query trigrams vs a contiguous span of statement) is the
// third recall channel — same operator knowledge / agent-case already use.
//
// Operators resolve via search_path (production includes public; tests pin
// schema, public then CREATE EXTENSION). Missing extension degrades to nil
// without failing recall.
func (r *l3FactRepo) searchL3Trigram(ctx context.Context, scopeType, scopeID, userID, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if r == nil || r.data == nil || !r.data.Dialect().IsPostgres() {
		return nil, nil
	}
	if utf8.RuneCountInString(query) < 2 {
		return nil, nil
	}
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	clauses := []string{
		"status = 'active'", "deleted_at = ''", "valid_until = ''",
		"statement %> ?",
	}
	args := []any{query}
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
		` ORDER BY word_similarity(?, statement) DESC, COALESCE(NULLIF(valid_from, ''), created_at) DESC LIMIT ?`
	args = append(args, query, limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		if isMissingTrigramOp(err) {
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

func isMissingTrigramOp(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case "42883", "42704", "42809": // undefined_function / undefined_object / wrong_object_type
			return true
		}
	}
	return false
}

// trgmCandidateIDs is the best-effort wrapper around searchL3Trigram.
func (r *l3FactRepo) trgmCandidateIDs(ctx context.Context, scopeType, scopeID, userID, query string, limit int) []string {
	ids, err := r.searchL3Trigram(ctx, scopeType, scopeID, userID, query, limit)
	if err != nil {
		r.data.lg.Warn("L3 trigram candidate search failed, degrading to non-trgm recall",
			loggateway.StepID("memory.l3_trgm_search"),
			loggateway.Err(err))
		return nil
	}
	return ids
}

func (r *l3FactRepo) trgmExtraCandidates(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, present map[string]struct{}, minScore float64, now time.Time) ([]scoredFact, error) {
	return r.trgmExtraCandidatesAt(ctx, scopeType, scopeID, userID, query, queryEmbedding, present, minScore, now, l3FTSCandidateLimit)
}

func (r *l3FactRepo) trgmExtraCandidatesAt(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, present map[string]struct{}, minScore float64, now time.Time, limit int) ([]scoredFact, error) {
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	ids := r.trgmCandidateIDs(ctx, scopeType, scopeID, userID, query, limit)
	if len(ids) == 0 {
		return nil, nil
	}
	missing := make([]string, 0, len(ids))
	for _, id := range ids {
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
