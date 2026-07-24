package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// l3FactRepo implements biz L3 interfaces using direct Raw SQL.
type l3FactRepo struct {
	data        *Data
	vectorStore vector.VectorStore
}

// Compile-time interface checks.
var (
	_ biz.L3FactReader     = (*l3FactRepo)(nil)
	_ biz.L3FactWriter     = (*l3FactRepo)(nil)
	_ biz.L3ConflictStore  = (*l3FactRepo)(nil)
	_ biz.PIIReviewStore   = (*l3FactRepo)(nil)
	_ biz.DecayScoreWriter = (*l3FactRepo)(nil)
)

func newL3FactRepo(data *Data, vs vector.VectorStore) *l3FactRepo {
	if data == nil {
		return nil
	}
	return &l3FactRepo{data: data, vectorStore: vs}
}

// NewL3FactWriterAdapter creates a biz.L3FactWriter backed by data.
func NewL3FactWriterAdapter(data *Data, vs vector.VectorStore) biz.L3FactWriter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, vs)
}

// NewL3FactReaderForUser creates a biz.L3FactReader backed by data.
func NewL3FactReaderForUser(data *Data) biz.L3FactReader {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewDecayScoreWriter creates a biz.DecayScoreWriter backed by data.
// Used by the Ebbinghaus decay cron job to persist R_t scores.
func NewDecayScoreWriter(data *Data) biz.DecayScoreWriter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// factConsistencyAdapter implements internal/memory/trpc.factConsistencyChecker via raw SQL.
// Note: no compile-time var _ check because data cannot import memory/trpc (reverse dependency).
// Wire binding in cmd/admin/wire_memory.go ensures type compatibility at injection time.
type factConsistencyAdapter struct {
	data *Data
}

// NewFactConsistencyAdapter creates a factConsistencyChecker backed by data.
func NewFactConsistencyAdapter(data *Data) *factConsistencyAdapter {
	if data == nil {
		return nil
	}
	return &factConsistencyAdapter{data: data}
}

func (a *factConsistencyAdapter) GetFactConsistencyRow(ctx context.Context, factID string) (status, indexStatus, statement string, err error) {
	err = QueryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		a.data.Dialect().RenumberPlaceholders(`SELECT status, embedding_status, statement FROM memory_facts WHERE id = ?`),
		[]any{factID}, &status, &indexStatus, &statement)
	return
}

func (a *factConsistencyAdapter) GetFactResyncRow(ctx context.Context, factID string) (agentID, userID, statement string, err error) {
	err = QueryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		a.data.Dialect().RenumberPlaceholders(`SELECT COALESCE(agent_id, scope_id), user_id, statement FROM memory_facts WHERE id = ?`),
		[]any{factID}, &agentID, &userID, &statement)
	return
}

// --- L3FactReader ---

func (r *l3FactRepo) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	clauses, args := buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, true)
	where := " WHERE " + strings.Join(clauses, " AND ")

	// Single query to get total, active, and archived counts.
	countClauses, countArgs := buildFactFilterClauses(scopeType, scopeID, kind, "", keyword, false)
	countWhere := ""
	if len(countClauses) > 0 {
		countWhere = " WHERE " + strings.Join(countClauses, " AND ")
	}
	var total, active, archived int32
	countRow, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) as total, SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as active, SUM(CASE WHEN status = 'archived' THEN 1 ELSE 0 END) as archived FROM memory_facts`+countWhere),
		countArgs...)
	if err != nil {
		return nil, 0, 0, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	if countRow.Next() {
		var a, ar sql.NullInt32
		if scanErr := countRow.Scan(&total, &a, &ar); scanErr != nil {
			countRow.Close()
			return nil, 0, 0, 0, entErrToBizErr(scanErr, "MEMORY_L3")
		}
		if a.Valid {
			active = a.Int32
		} else {
			active = total
		}
		if ar.Valid {
			archived = ar.Int32
		}
	}
	countRow.Close()

	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, 0, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, 0, 0, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, total, active, archived, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// buildFactFilterClauses constructs WHERE clause components for fact queries.
// When withStatusFilter is true, the status parameter is applied; otherwise
// only scope/kind/keyword/deleted_at filters are included (for total counts).
func buildFactFilterClauses(scopeType, scopeID, kind, status, keyword string, withStatusFilter bool) ([]string, []any) {
	clauses := []string{}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if kind != "" {
		clauses = append(clauses, "fact_kind = ?")
		args = append(args, kind)
	}
	if withStatusFilter {
		if status != "" {
			clauses = append(clauses, "status = ?")
			args = append(args, status)
		} else {
			clauses = append(clauses, "status = 'active'")
		}
	}
	if keyword != "" {
		clauses = append(clauses, "statement_normalized LIKE ?")
		args = append(args, "%"+strings.ToLower(keyword)+"%")
	}
	clauses = append(clauses, "deleted_at = ''")
	return clauses, args
}

// countFacts returns the count of facts matching the given filter clauses.
func (r *l3FactRepo) countFacts(ctx context.Context, clauses []string, args ...any) (int32, error) {
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	var count int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &count); err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L3")
	}
	return count, nil
}

func (r *l3FactRepo) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	// Bi-temporal filter (P3-8): only return currently-valid facts by default.
	// Invalidated facts (valid_until != '') are preserved for history but
	// excluded from normal search/read paths.
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
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
	if keyword != "" {
		clauses = append(clauses, "statement_normalized LIKE ?")
		args = append(args, "%"+strings.ToLower(keyword)+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// ListFactRowsForUserAll returns facts for a user including invalidated ones
// (valid_until != ”). Used for historical reconstruction queries when
// SearchOptions.IncludeInvalidated is true. Deleted facts are still excluded.
func (r *l3FactRepo) ListFactRowsForUserAll(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	clauses := []string{"status = 'active'", "deleted_at = ''"}
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
	if keyword != "" {
		clauses = append(clauses, "statement_normalized LIKE ?")
		args = append(args, "%"+strings.ToLower(keyword)+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func (r *l3FactRepo) GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error) {
	if len(factIDs) == 0 {
		return nil, nil
	}
	// Build parameterized IN clause to avoid SQL injection.
	placeholders := make([]string, len(factIDs))
	args := make([]any, len(factIDs))
	for i, id := range factIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := sqlFactSelect + " WHERE id IN (" + strings.Join(placeholders, ",") + ") AND status = 'active' AND deleted_at = '' AND valid_until = ''"
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func (r *l3FactRepo) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	// Check fact count for brute-force threshold.
	// When the number of active facts for the agent is below the threshold,
	// use linear scan by importance instead of vector similarity search.
	if r.shouldUseBruteForce(ctx, scopeType, scopeID, userID, queryEmbedding) {
		return r.recallL3FactsBruteForce(ctx, scopeType, scopeID, userID, limit)
	}
	if r.vectorStore != nil && len(queryEmbedding) > 0 {
		return r.recallL3WithVectorStore(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}
	return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
}

// shouldUseBruteForce checks whether the fact count is below the brute-force threshold.
func (r *l3FactRepo) shouldUseBruteForce(ctx context.Context, scopeType, scopeID, userID string, queryEmbedding []float32) bool {
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
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
	return count <= biz.DefaultFactBruteForceThreshold || len(queryEmbedding) == 0
}

// recallL3FactsBruteForce returns facts ordered by importance DESC without vector scoring.
func (r *l3FactRepo) recallL3FactsBruteForce(ctx context.Context, scopeType, scopeID, userID string, limit int32) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
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
	q := sqlFactSelect + where + ` ORDER BY importance DESC, updated_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
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
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
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
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, pool)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	tokens := tokenizeQuery(query)
	now := time.Now().UTC()
	var scored []scoredFact
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(b, &row) != nil {
			continue
		}
		id, _ := row["id"].(string)
		stmt, _ := row["statement"].(string)
		details, _ := row["details_markdown"].(string)
		imp := anyFloat(row, "importance")
		_ = anyFloat(row, "confidence") // confidence not used in L3 hybrid scoring
		qScore := anyFloat(row, "quality_score")
		updatedAt, _ := row["updated_at"].(string)
		factKind, _ := row["fact_kind"].(string)

		kwScore := keywordOverlapScore(tokens, stmt+" "+details)
		var vecScore float64
		if len(queryEmbedding) > 0 {
			// JSON unmarshal produces string (not []byte) for binary columns,
			// so we must handle both types.
			var embBlob []byte
			switch v := row["embedding_blob"].(type) {
			case []byte:
				embBlob = v
			case string:
				embBlob = []byte(v)
			}
			if embNorm, ok := row["embedding_norm"].(float64); ok && embNorm > 0 && len(embBlob) > 0 {
				emb := decodeFloat32Blob(embBlob)
				if len(emb) == len(queryEmbedding) {
					vecScore = cosineSimilarity(queryEmbedding, emb)
				}
			}
		}
		recency := recencyBoost(updatedAt, now)
		decay := factDecayWithKind(factKind, updatedAt, now)
		total := l3ScoreWeightKeyword*kwScore +
			l3ScoreWeightVector*vecScore +
			l3ScoreWeightImport*imp*decay +
			l3ScoreWeightRecency*recency +
			l3ScoreWeightQuality*qScore

		if total < minScore {
			continue
		}
		scored = append(scored, scoredFact{
			raw:     b,
			id:      id,
			stmt:    stmt,
			details: details,
			score:   total,
			breakdown: recallScoreBreakdown{
				Keyword:      kwScore,
				Vector:       vecScore,
				Importance:   imp * decay,
				Recency:      recency,
				QualityScore: qScore,
				Total:        total,
			},
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > lim {
		scored = scored[:lim]
	}
	passages := make([]string, len(scored))
	for i, s := range scored {
		passages[i] = factPassage(s.stmt, s.details)
	}
	applyCrossEncoderRerankToFactScored(r.data.Reranker(), query, scored, passages)
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	var out [][]byte
	for _, s := range scored {
		out = append(out, s.raw)
	}
	return out, nil
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
	vecHits, err := r.vectorStore.Search(ctx, float32To64(queryEmbedding), pool, 0.3)
	if err != nil || len(vecHits) == 0 {
		return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}
	ids := make([]string, 0, len(vecHits))
	hitMap := make(map[string]float64, len(vecHits))
	for _, h := range vecHits {
		ids = append(ids, h.ID)
		hitMap[h.ID] = h.Score
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+3)
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''"}
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
	phArgs := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		phArgs[i] = id
	}
	clauses = append(clauses, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	args = append(args, phArgs...)
	where := " WHERE " + strings.Join(clauses, " AND ")
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, pool)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	tokens := tokenizeQuery(query)
	now := time.Now().UTC()
	var scored []scoredFact
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(b, &row) != nil {
			continue
		}
		id, _ := row["id"].(string)
		stmt, _ := row["statement"].(string)
		details, _ := row["details_markdown"].(string)
		imp := anyFloat(row, "importance")
		qScore := anyFloat(row, "quality_score")
		updatedAt, _ := row["updated_at"].(string)
		factKind, _ := row["fact_kind"].(string)

		vecScore := hitMap[id]
		kwScore := keywordOverlapScore(tokens, stmt+" "+details)
		recency := recencyBoost(updatedAt, now)
		decay := factDecayWithKind(factKind, updatedAt, now)
		total := l3ScoreWeightKeyword*kwScore +
			l3ScoreWeightVector*vecScore +
			l3ScoreWeightImport*imp*decay +
			l3ScoreWeightRecency*recency +
			l3ScoreWeightQuality*qScore

		if total < minScore {
			continue
		}
		scored = append(scored, scoredFact{
			raw:     b,
			id:      id,
			stmt:    stmt,
			details: details,
			score:   total,
			breakdown: recallScoreBreakdown{
				Keyword:      kwScore,
				Vector:       vecScore,
				Importance:   imp * decay,
				Recency:      recency,
				QualityScore: qScore,
				Total:        total,
			},
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > lim {
		scored = scored[:lim]
	}
	passages := make([]string, len(scored))
	for i, s := range scored {
		passages[i] = factPassage(s.stmt, s.details)
	}
	applyCrossEncoderRerankToFactScored(r.data.Reranker(), query, scored, passages)
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	var out [][]byte
	for _, s := range scored {
		out = append(out, s.raw)
	}
	return out, nil
}

// --- L3FactWriter ---

func (r *l3FactRepo) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	fp := strings.TrimSpace(in.Fingerprint)
	if fp == "" {
		fp = biz.FactFingerprint(in.Statement, in.ScopeType, in.ScopeID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := strings.TrimSpace(in.CreatedAt)
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := strings.TrimSpace(in.UpdatedAt)
	if updatedAt == "" {
		updatedAt = now
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	tags := strings.TrimSpace(in.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	links := strings.TrimSpace(in.LinksJSON)
	if links == "" {
		links = "[]"
	}
	keywords := strings.TrimSpace(in.KeywordsJSON)
	if keywords == "" {
		keywords = "[]"
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	details := strings.TrimSpace(in.DetailsMarkdown)
	pii := memBoolToInt(in.PIIFlag)
	// redacted_statement stores the ORIGINAL text when PII is flagged,
	// so that ApprovePIIFact can restore it. It is NOT the redacted version.
	redacted := ""
	if pii != 0 {
		if in.OriginalStatement != "" {
			redacted = in.OriginalStatement
		} else if details != "" {
			redacted = details
		}
	}
	piiTypesJSON := "[]"
	if len(in.PIITypes) > 0 {
		if b, err := json.Marshal(in.PIITypes); err == nil {
			piiTypesJSON = string(b)
		}
	}

	// INSERT + read-back in a single transaction to ensure read-your-writes
	// consistency under read-write separation.
	validFrom := strings.TrimSpace(in.ValidFrom)
	if validFrom == "" {
		// Default ValidFrom to createdAt for new facts so bi-temporal
		// queries have a meaningful lower bound. For upserts that hit the
		// ON CONFLICT branch, valid_from is preserved (not overwritten).
		validFrom = createdAt
	}
	validUntil := strings.TrimSpace(in.ValidUntil)
	contextNote := strings.TrimSpace(in.ContextNote)
	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_facts (
		id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
		pii_flag, redacted_statement,
		ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
		quality_score, pii_types, metadata_json, created_at, updated_at,
		archived_at, deleted_at,
		valid_from, valid_until, links, keywords, tags,
		context_note
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
		statement = excluded.statement, details_markdown = excluded.details_markdown,
		confidence = excluded.confidence, importance = excluded.importance,
		use_count = memory_facts.use_count + excluded.use_count, hit_count = memory_facts.hit_count + excluded.hit_count,
	positive_feedback_count = memory_facts.positive_feedback_count + excluded.positive_feedback_count,
	negative_feedback_count = memory_facts.negative_feedback_count + excluded.negative_feedback_count,
	conflict_count = memory_facts.conflict_count + excluded.conflict_count,
		fact_kind = excluded.fact_kind, tags_json = excluded.tags_json,
		source_kind = excluded.source_kind, source_episode_id = excluded.source_episode_id,
		source_session_id = excluded.source_session_id, source_message_id = excluded.source_message_id,
		source_external = excluded.source_external,
		version = memory_facts.version + 1, status = excluded.status,
		pii_flag = excluded.pii_flag, redacted_statement = excluded.redacted_statement,
		quality_score = excluded.quality_score, pii_types = excluded.pii_types, metadata_json = excluded.metadata_json,
		updated_at = excluded.updated_at,
		valid_from = COALESCE(NULLIF(memory_facts.valid_from, ''), excluded.valid_from),
		valid_until = excluded.valid_until,
		links = excluded.links, keywords = excluded.keywords,
		context_note = excluded.context_note`),
			id,
			strings.TrimSpace(in.ScopeType),
			strings.TrimSpace(in.ScopeID),
			strings.TrimSpace(in.WorkspaceID),
			strings.TrimSpace(in.UserID),
			strings.TrimSpace(in.TeamID),
			strings.TrimSpace(in.AgentID),
			strings.TrimSpace(in.Statement),
			strings.ToLower(strings.TrimSpace(in.Statement)),
			fp, details,
			strings.TrimSpace(in.FactKind), tags,
			in.Confidence, in.Importance, in.UseCount, in.HitCount,
			in.PositiveFeedbackCount, in.NegativeFeedbackCount, in.ConflictCount,
			strings.TrimSpace(in.SourceKind),
			strings.TrimSpace(in.SourceEpisodeID),
			strings.TrimSpace(in.SourceSessionID),
			strings.TrimSpace(in.SourceMessageID),
			strings.TrimSpace(in.SourceExternal),
			in.Version, status, "",
			"pending", "", 0, nil, 0.0, // embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm
			pii, redacted,
			0, 0.98, "", "", "", // ttl_days, decay_factor, next_decay_at, last_used_at, expires_at
			defaultFactQualityScore, piiTypesJSON, meta, createdAt, updatedAt,
			"", "", // archived_at, deleted_at
			validFrom, validUntil, links, keywords, tags,
			contextNote,
		)
		if execErr != nil {
			return execErr
		}
		// Read back the row using the unique constraint (scope_type, scope_id, fingerprint)
		// within the same transaction so the write connection is reused.
		rows, queryErr := r.data.RWDB().WriteDB(txCtx).QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`),
			strings.TrimSpace(in.ScopeType), strings.TrimSpace(in.ScopeID), fp)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return entErrToBizErr(err, "MEMORY")
			}
			return apierror.NotFound("MEMORY", "fact row not found after upsert")
		}
		result, execErr = scanFactRowJSON(rows)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}

func (r *l3FactRepo) DeleteFactRow(ctx context.Context, factID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE id = ?`), now, factID)
	if err != nil {
		return entErrToBizErr(err, "MEMORY_L3")
	}
	// Cascade: remove pgvector embedding so deleted facts are not recalled.
	if r.vectorStore != nil {
		if delErr := r.vectorStore.Delete(ctx, factID); delErr != nil {
			r.data.lg.Warn("cascade: delete fact vector failed, marking stale",
				loggateway.StepID("memory.l3.vector_delete"),
				loggateway.Str("fact_id", factID),
				loggateway.Err(delErr))
			// Mark embedding_status as stale so reconciler can fix later.
			if markErr := r.markFactEmbeddingStale(ctx, factID); markErr != nil {
				r.data.lg.Warn("cascade: mark fact embedding stale failed",
					loggateway.StepID("memory.l3.vector_mark_stale"),
					loggateway.Str("fact_id", factID),
					loggateway.Err(markErr))
			}
		}
	}
	return nil
}

func (r *l3FactRepo) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	if len(factIDs) == 0 {
		return 0, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(factIDs))
	args := make([]any, 0, len(factIDs)+1)
	args = append(args, now)
	for i, id := range factIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L3")
	}
	n, _ := res.RowsAffected()
	// Cascade: remove pgvector embeddings so deleted facts are not recalled.
	if r.vectorStore != nil {
		for _, fid := range factIDs {
			if delErr := r.vectorStore.Delete(ctx, fid); delErr != nil {
				r.data.lg.Warn("cascade: delete fact vector failed, marking stale",
					loggateway.StepID("memory.l3.vector_delete_batch"),
					loggateway.Str("fact_id", fid),
					loggateway.Err(delErr))
				// Mark embedding_status as stale so reconciler can fix later.
				if markErr := r.markFactEmbeddingStale(ctx, fid); markErr != nil {
					r.data.lg.Warn("cascade: mark fact embedding stale failed",
						loggateway.StepID("memory.l3.vector_mark_stale"),
						loggateway.Str("fact_id", fid),
						loggateway.Err(markErr))
				}
			}
		}
	}
	return int(n), nil
}

func (r *l3FactRepo) ClearFactsByScope(ctx context.Context, scopeType, scopeID, userID string) ([]string, error) {
	// Read IDs + soft-delete in a single transaction to prevent new facts
	// from escaping cleanup (read-write consistency under separation).
	var factIDs []string
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		rows, queryErr := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`SELECT id FROM memory_facts WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`),
			scopeType, scopeID, userID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr != nil {
				return scanErr
			}
			factIDs = append(factIDs, id)
		}
		if err := rows.Err(); err != nil {
			return entErrToBizErr(err, "MEMORY_L3")
		}
		if len(factIDs) == 0 {
			return nil
		}
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET deleted_at = ?, status = 'deleted', valid_until = ? WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`),
			now, now, scopeType, scopeID, userID)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	if len(factIDs) == 0 {
		return nil, nil
	}

	// Cascade: remove pgvector embeddings so deleted facts are not recalled.
	if r.vectorStore != nil {
		for _, fid := range factIDs {
			if delErr := r.vectorStore.Delete(ctx, fid); delErr != nil {
				r.data.lg.Warn("cascade: clear fact vector failed, marking stale",
					loggateway.StepID("memory.l3.vector_clear"),
					loggateway.Str("fact_id", fid),
					loggateway.Err(delErr))
				// Mark embedding_status as stale so reconciler can fix later.
				if markErr := r.markFactEmbeddingStale(ctx, fid); markErr != nil {
					r.data.lg.Warn("cascade: mark fact embedding stale failed",
						loggateway.StepID("memory.l3.vector_mark_stale"),
						loggateway.Str("fact_id", fid),
						loggateway.Err(markErr))
				}
			}
		}
	}
	return factIDs, nil
}

// InvalidateFact marks a fact as superseded by setting valid_until to the
// current time (bi-temporal validity, P3-8). The row is preserved for
// historical reconstruction queries. Returns the updated row as JSON.
func (r *l3FactRepo) InvalidateFact(ctx context.Context, factID string) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET valid_until = ?, updated_at = ? WHERE id = ? AND valid_until = ''`),
			now, now, factID)
		if execErr != nil {
			return execErr
		}
		rows, queryErr := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE id = ?`), factID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			return apierror.NotFound("MEMORY", "fact row not found after invalidate")
		}
		result, execErr = scanFactRowJSON(rows)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}

// InvalidateAndUpsertFactTx atomically invalidates the old fact and upserts the
// new fact in a single transaction (P0-2 fix). If oldFactID is non-empty, the
// old fact is invalidated (valid_until set) before the new fact is upserted.
// Returns the upserted fact row as JSON.
func (r *l3FactRepo) InvalidateAndUpsertFactTx(ctx context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	fp := strings.TrimSpace(in.Fingerprint)
	if fp == "" {
		fp = biz.FactFingerprint(in.Statement, in.ScopeType, in.ScopeID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := strings.TrimSpace(in.CreatedAt)
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := strings.TrimSpace(in.UpdatedAt)
	if updatedAt == "" {
		updatedAt = now
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	tags := strings.TrimSpace(in.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	links := strings.TrimSpace(in.LinksJSON)
	if links == "" {
		links = "[]"
	}
	keywords := strings.TrimSpace(in.KeywordsJSON)
	if keywords == "" {
		keywords = "[]"
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	details := strings.TrimSpace(in.DetailsMarkdown)
	pii := memBoolToInt(in.PIIFlag)
	redacted := ""
	if pii != 0 {
		if in.OriginalStatement != "" {
			redacted = in.OriginalStatement
		} else if details != "" {
			redacted = details
		}
	}
	piiTypesJSON := "[]"
	if len(in.PIITypes) > 0 {
		if b, err := json.Marshal(in.PIITypes); err == nil {
			piiTypesJSON = string(b)
		}
	}
	validFrom := strings.TrimSpace(in.ValidFrom)
	if validFrom == "" {
		validFrom = createdAt
	}
	validUntil := strings.TrimSpace(in.ValidUntil)
	contextNote := strings.TrimSpace(in.ContextNote)
	oldID := strings.TrimSpace(oldFactID)

	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())

		// Step 1: Invalidate old fact (if provided).
		if oldID != "" {
			_, execErr := e.ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET valid_until = ?, updated_at = ? WHERE id = ? AND valid_until = ''`),
				now, now, oldID)
			if execErr != nil {
				return execErr
			}
		}

		// Step 2: Upsert new fact.
		_, execErr := e.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_facts (
		id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
		pii_flag, redacted_statement,
		ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
		quality_score, pii_types, metadata_json, created_at, updated_at,
		archived_at, deleted_at,
		valid_from, valid_until, links, keywords, tags,
		context_note
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
		statement = excluded.statement, details_markdown = excluded.details_markdown,
		confidence = excluded.confidence, importance = excluded.importance,
		use_count = memory_facts.use_count + excluded.use_count, hit_count = memory_facts.hit_count + excluded.hit_count,
	positive_feedback_count = memory_facts.positive_feedback_count + excluded.positive_feedback_count,
	negative_feedback_count = memory_facts.negative_feedback_count + excluded.negative_feedback_count,
	conflict_count = memory_facts.conflict_count + excluded.conflict_count,
		fact_kind = excluded.fact_kind, tags_json = excluded.tags_json,
		source_kind = excluded.source_kind, source_episode_id = excluded.source_episode_id,
		source_session_id = excluded.source_session_id, source_message_id = excluded.source_message_id,
		source_external = excluded.source_external,
		version = memory_facts.version + 1, status = excluded.status,
		pii_flag = excluded.pii_flag, redacted_statement = excluded.redacted_statement,
		quality_score = excluded.quality_score, pii_types = excluded.pii_types, metadata_json = excluded.metadata_json,
		updated_at = excluded.updated_at,
		valid_from = COALESCE(NULLIF(memory_facts.valid_from, ''), excluded.valid_from),
		valid_until = excluded.valid_until,
		links = excluded.links, keywords = excluded.keywords,
		context_note = excluded.context_note`),
			id,
			strings.TrimSpace(in.ScopeType),
			strings.TrimSpace(in.ScopeID),
			strings.TrimSpace(in.WorkspaceID),
			strings.TrimSpace(in.UserID),
			strings.TrimSpace(in.TeamID),
			strings.TrimSpace(in.AgentID),
			strings.TrimSpace(in.Statement),
			strings.ToLower(strings.TrimSpace(in.Statement)),
			fp, details,
			strings.TrimSpace(in.FactKind), tags,
			in.Confidence, in.Importance, in.UseCount, in.HitCount,
			in.PositiveFeedbackCount, in.NegativeFeedbackCount, in.ConflictCount,
			strings.TrimSpace(in.SourceKind),
			strings.TrimSpace(in.SourceEpisodeID),
			strings.TrimSpace(in.SourceSessionID),
			strings.TrimSpace(in.SourceMessageID),
			strings.TrimSpace(in.SourceExternal),
			in.Version, status, "",
			"pending", "", 0, nil, 0.0,
			pii, redacted,
			0, 0.98, "", "", "",
			defaultFactQualityScore, piiTypesJSON, meta, createdAt, updatedAt,
			"", "",
			validFrom, validUntil, links, keywords, tags,
			contextNote,
		)
		if execErr != nil {
			return execErr
		}

		// Step 3: Read back the upserted row within the same tx.
		rows, queryErr := e.QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`),
			strings.TrimSpace(in.ScopeType), strings.TrimSpace(in.ScopeID), fp)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return entErrToBizErr(err, "MEMORY")
			}
			return apierror.NotFound("MEMORY", "fact row not found after upsert")
		}
		result, execErr = scanFactRowJSON(rows)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}

// --- L3ConflictStore ---

func (r *l3FactRepo) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	var count int32
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET conflict_count = conflict_count + 1, updated_at = ? WHERE id = ?`),
			time.Now().UTC().Format(time.RFC3339Nano), factID)
		if execErr != nil {
			return execErr
		}
		return queryRowScan(txCtx, r.data.RWDB().ReadDB(txCtx),
			r.data.Dialect().RenumberPlaceholders(`SELECT conflict_count FROM memory_facts WHERE id = ?`), []any{factID}, &count)
	})
	return count, entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) BatchIncrementConflictCounts(ctx context.Context, factIDs []string) error {
	if len(factIDs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(factIDs))
	args := make([]any, 0, len(factIDs)+1)
	args = append(args, now)
	for i, id := range factIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`UPDATE memory_facts SET conflict_count = conflict_count + 1, updated_at = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	clauses := []string{"conflict_count > 0", "status = 'active'", "deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY conflict_count DESC, updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, total, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// --- PIIReviewStore ---

func (r *l3FactRepo) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	clauses := []string{"pii_flag = 1", "status = 'active'", "deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, total, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func (r *l3FactRepo) ApprovePIIFact(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET pii_flag = 0, statement = COALESCE(NULLIF(redacted_statement, ''), statement), redacted_statement = '', pii_types = '[]', updated_at = ? WHERE id = ?`),
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) RejectPIIFact(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET status = 'redacted', updated_at = ? WHERE id = ?`),
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

// markFactEmbeddingStale marks a fact's embedding_status as 'stale' so the
// reconciler can detect and fix the inconsistency between SQLite and pgvector.
func (r *l3FactRepo) markFactEmbeddingStale(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET embedding_status = 'stale' WHERE id = ?`), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

// --- DecayScoreWriter ---

// UpdateDecayScores batch-updates the persisted Ebbinghaus decay score (R_t)
// for the given fact IDs. The updates are wrapped in a single transaction so
// the batch is atomic (red line #24: cross-table writes must use ExecInTx).
//
// scores maps fact ID → R_t ∈ (0, 1]. A zero-length map is a no-op.
func (r *l3FactRepo) UpdateDecayScores(ctx context.Context, scores map[string]float64) error {
	if r == nil || r.data == nil || len(scores) == 0 {
		return nil
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for factID, score := range scores {
			factID = strings.TrimSpace(factID)
			if factID == "" {
				continue
			}
			_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET decay_score = ?, updated_at = ? WHERE id = ?`),
				score, now, factID)
			if execErr != nil {
				return entErrToBizErr(execErr, "MEMORY_L3")
			}
		}
		return nil
	})
}

// IncrementFactAccessCount batch-increments use_count and updates last_used_at
// for the given fact IDs. Called by the scored recall adapter so the Ebbinghaus
// decay worker can use accurate access-recency signals. The updates are wrapped
// in a single transaction to minimize write load on the recall path.
//
// factIDs must be non-empty; an empty slice is a no-op.
func (r *l3FactRepo) IncrementFactAccessCount(ctx context.Context, factIDs []string) error {
	if r == nil || r.data == nil || len(factIDs) == 0 {
		return nil
	}
	// Deduplicate and trim IDs to avoid redundant updates.
	seen := make(map[string]struct{}, len(factIDs))
	ids := make([]string, 0, len(factIDs))
	for _, id := range factIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		placeholders := make([]string, len(ids))
		args := make([]any, 0, len(ids)+1)
		args = append(args, now)
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		q := fmt.Sprintf(`UPDATE memory_facts SET use_count = use_count + 1, last_used_at = ? WHERE id IN (%s)`,
			strings.Join(placeholders, ","))
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(q), args...)
		return entErrToBizErr(execErr, "MEMORY_L3")
	})
}
