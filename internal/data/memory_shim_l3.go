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
	// bruteForceThreshold overrides biz.DefaultFactBruteForceThreshold when
	// > 0. Tests set it to force the non-brute-force recall paths (recency
	// pool / pgvector+FTS fusion) with small fixtures.
	bruteForceThreshold int
}

// Compile-time interface checks.
var (
	_ biz.L3FactReader               = (*l3FactRepo)(nil)
	_ biz.L3FactWriter               = (*l3FactRepo)(nil)
	_ biz.L3FactReviewStore          = (*l3FactRepo)(nil)
	_ biz.L3ConflictStore            = (*l3FactRepo)(nil)
	_ biz.PIIReviewStore             = (*l3FactRepo)(nil)
	_ biz.DecayScoreWriter           = (*l3FactRepo)(nil)
	_ biz.MemoryPreferenceLister     = (*l3FactRepo)(nil)
	_ biz.FactAccessCounter          = (*l3FactRepo)(nil)
	_ biz.MemoryFactInjectCounter    = (*l3FactRepo)(nil)
	_ biz.MemoryFactCitationRecorder = (*l3FactRepo)(nil)
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

// NewL3ConflictStore creates a biz.L3ConflictStore backed by data.
// Used by the chat turn path for conflict governance (supersede/mark).
func NewL3ConflictStore(data *Data) biz.L3ConflictStore {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3FactAccessCounter creates a biz.FactAccessCounter backed by data.
// Used by the unified fact write pipeline (P1-3) for dedup-merge access bumps.
func NewL3FactAccessCounter(data *Data) biz.FactAccessCounter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3FactInjectCounter creates a biz.MemoryFactInjectCounter backed by
// data (FR-12.6). Used by the before-model memory inject hook for the
// once-per-turn injected_count bump.
func NewL3FactInjectCounter(data *Data) biz.MemoryFactInjectCounter {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewL3FactCitationRecorder creates a biz.MemoryFactCitationRecorder backed
// by data (FR-12.6). Used by the citation backfill worker.
func NewL3FactCitationRecorder(data *Data) biz.MemoryFactCitationRecorder {
	if data == nil {
		return nil
	}
	return newL3FactRepo(data, nil)
}

// NewMemoryPreferenceLister creates a biz.MemoryPreferenceLister backed by
// data. Used by the agent layer for pinned preference injection (FR-M3).
func NewMemoryPreferenceLister(data *Data) biz.MemoryPreferenceLister {
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

func (r *l3FactRepo) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	clauses, args := buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, agentID, true)
	where := " WHERE " + strings.Join(clauses, " AND ")

	// Single query to get total, active, and archived counts.
	countClauses, countArgs := buildFactFilterClauses(scopeType, scopeID, kind, "", keyword, agentID, false)
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

// CountFactRows returns the count matching the rows-caliber filter (same WHERE
// as ListFactRows items, including the status filter with default 'active').
// ListFactRows' total deliberately ignores status for the stats-row breakdown,
// so the memory center uses this as the server-side pagination total.
func (r *l3FactRepo) CountFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string) (int32, error) {
	clauses, args := buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, agentID, true)
	return r.countFacts(ctx, clauses, args...)
}

// buildFactFilterClauses constructs WHERE clause components for fact queries.
// When withStatusFilter is true, the status parameter is applied; otherwise
// only scope/kind/keyword/deleted_at filters are included (for total counts).
// agentID filters by the originating-agent column (memory_facts.agent_id)
// independently of the scope namespace — see biz.L3FactReader.
func buildFactFilterClauses(scopeType, scopeID, kind, status, keyword, agentID string, withStatusFilter bool) ([]string, []any) {
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
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
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
		return r.recallL3FactsBruteForce(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
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
	threshold := r.bruteForceThreshold
	if threshold <= 0 {
		threshold = biz.DefaultFactBruteForceThreshold
	}
	return count <= threshold || len(queryEmbedding) == 0
}

// recallL3FactsBruteForce scans the (bounded) active fact set without a
// vector store and applies the same hybrid scoring as the other recall paths.
// Candidates are pre-limited in SQL by importance to keep the scan bounded;
// scoring, minScore filtering, and ranking happen in Go.
func (r *l3FactRepo) recallL3FactsBruteForce(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
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
	q := sqlFactSelect + where + ` ORDER BY importance DESC, updated_at DESC LIMIT ?`
	args = append(args, pool)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	scored := scoreFactRows(rows, tokenizeQuery(query), queryEmbedding, nil, minScore, time.Now().UTC())
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	// P2-3: inject FTS-ranked candidates missed by the importance pre-limit
	// (keyword-strong facts with low importance), scored via the same hybrid.
	extra, err := r.ftsExtraCandidates(ctx, scopeType, scopeID, userID, query, queryEmbedding, scoredFactIDs(scored), minScore, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	scored = append(scored, extra...)
	return r.finalizeScoredFacts(query, scored, lim), nil
}

// finalizeScoredFacts sorts, truncates, reranks, and annotates scored facts
// into the output JSON rows. Score annotation happens after rerank so the
// persisted breakdown reflects the final (possibly cross-encoder-adjusted)
// total.
func (r *l3FactRepo) finalizeScoredFacts(query string, scored []scoredFact, lim int) [][]byte {
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
	scored := scoreFactRows(rows, tokenizeQuery(query), queryEmbedding, nil, minScore, time.Now().UTC())
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	// P2-3: inject FTS-ranked candidates missed by the recency pre-limit
	// (keyword-strong but older facts), scored via the same hybrid.
	extra, err := r.ftsExtraCandidates(ctx, scopeType, scopeID, userID, query, queryEmbedding, scoredFactIDs(scored), minScore, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	scored = append(scored, extra...)
	return r.finalizeScoredFacts(query, scored, lim), nil
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
	if err != nil {
		// Degrade: a vector-store failure must not fail the recall — FTS and
		// recency candidates still apply (previously it fell back silently).
		r.data.lg.Warn("L3 vector search failed, degrading to FTS/recency recall",
			loggateway.StepID("memory.l3_vector_search"),
			loggateway.Err(err))
		vecHits = nil
	}
	// P2-3: fuse pgvector-ranked and FTS-ranked candidates with RRF so
	// keyword-strong facts (codes, names, exact tokens) enter the recall
	// pool even when embedding similarity misses them.
	ftsIDs := r.ftsCandidateIDs(ctx, scopeType, scopeID, userID, query, pool)
	if len(vecHits) == 0 && len(ftsIDs) == 0 {
		return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}
	vecIDs := make([]string, 0, len(vecHits))
	hitMap := make(map[string]float64, len(vecHits))
	for _, h := range vecHits {
		vecIDs = append(vecIDs, h.ID)
		hitMap[h.ID] = h.Score
	}
	rrfScores, fusedOrder := rrfFuseRanked(l3RRFK, vecIDs, ftsIDs)
	if len(fusedOrder) > pool {
		fusedOrder = fusedOrder[:pool]
	}
	rows, err := r.queryFactRowsByIDs(ctx, fusedOrder, scopeType, scopeID, userID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	scored := scoreFactRows(rows, tokenizeQuery(query), nil, hitMap, minScore, time.Now().UTC())
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	// Annotate the fused RRF score into the breakdown (observability only —
	// Total stays the calibrated hybrid score so minScore semantics hold).
	for i := range scored {
		scored[i].breakdown.RRF = rrfScores[scored[i].id]
	}
	return r.finalizeScoredFacts(query, scored, lim), nil
}

// --- L3FactWriter ---

// applyFactPIIGate enforces the M1 PII invariant at the L3 persistence
// boundary: memory_facts.statement / details_markdown never store plaintext
// PII. Every write is scanned here, so producers that did not pre-scan
// (Admin API path) cannot persist plaintext PII; inputs already redacted by
// upstream producers (trpc tool path) scan clean and pass through unchanged.
// redacted_statement keeps the ORIGINAL text so ApprovePIIFact can restore it.
// Must run before fingerprinting so the dedup key derives from redacted text,
// consistent with the consolidation path (memory_maintenance_adapter.go).
func applyFactPIIGate(in biz.FactUpsert) (statement, details string, pii int, redacted, piiTypesJSON string) {
	statement = strings.TrimSpace(in.Statement)
	details = strings.TrimSpace(in.DetailsMarkdown)
	pii = memBoolToInt(in.PIIFlag)
	if pii != 0 {
		if in.OriginalStatement != "" {
			redacted = in.OriginalStatement
		} else if details != "" {
			redacted = details
		}
	}
	piiTypesJSON = "[]"
	if len(in.PIITypes) > 0 {
		if b, err := json.Marshal(in.PIITypes); err == nil {
			piiTypesJSON = string(b)
		}
	}
	red := redactFactWritePII(statement, details)
	if red.piiFlag == 0 {
		return statement, details, pii, redacted, piiTypesJSON
	}
	statement, details, pii = red.statement, red.details, 1
	if redacted == "" {
		redacted = red.original
	}
	// Union caller-declared types with detector-matched types.
	typeSet := map[string]struct{}{}
	for _, t := range in.PIITypes {
		if t = strings.TrimSpace(t); t != "" {
			typeSet[t] = struct{}{}
		}
	}
	var detected []string
	if json.Unmarshal([]byte(red.piiTypesJSON), &detected) == nil {
		for _, t := range detected {
			typeSet[t] = struct{}{}
		}
	}
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	sort.Strings(types)
	if b, err := json.Marshal(types); err == nil {
		piiTypesJSON = string(b)
	}
	return statement, details, pii, redacted, piiTypesJSON
}

func (r *l3FactRepo) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	statement, details, pii, redacted, piiTypesJSON := applyFactPIIGate(in)
	fp := strings.TrimSpace(in.Fingerprint)
	if fp == "" {
		fp = biz.FactFingerprint(statement, in.ScopeType, in.ScopeID)
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
			statement,
			strings.ToLower(statement),
			fp, details,
			strings.TrimSpace(in.FactKind), tags,
			in.Confidence, in.Importance, in.UseCount, in.HitCount,
			in.PositiveFeedbackCount, in.NegativeFeedbackCount, in.ConflictCount,
			strings.TrimSpace(in.SourceKind),
			strings.TrimSpace(in.SourceEpisodeID),
			strings.TrimSpace(in.SourceSessionID),
			strings.TrimSpace(in.SourceMessageID),
			strings.TrimSpace(in.SourceExternal),
			// version is a system-managed revision counter: new rows always
			// start at 1, the ON CONFLICT branch bumps memory_facts.version + 1.
			// in.Version is intentionally ignored so callers cannot reset the
			// counter (import/restore paths use memory_migrate.go instead).
			1, status, "",
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
	statement, details, pii, redacted, piiTypesJSON := applyFactPIIGate(in)
	fp := strings.TrimSpace(in.Fingerprint)
	if fp == "" {
		fp = biz.FactFingerprint(statement, in.ScopeType, in.ScopeID)
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
			statement,
			strings.ToLower(statement),
			fp, details,
			strings.TrimSpace(in.FactKind), tags,
			in.Confidence, in.Importance, in.UseCount, in.HitCount,
			in.PositiveFeedbackCount, in.NegativeFeedbackCount, in.ConflictCount,
			strings.TrimSpace(in.SourceKind),
			strings.TrimSpace(in.SourceEpisodeID),
			strings.TrimSpace(in.SourceSessionID),
			strings.TrimSpace(in.SourceMessageID),
			strings.TrimSpace(in.SourceExternal),
			// version: system-managed counter, see UpsertFactRow.
			1, status, "",
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

// --- L3FactReviewStore ---

// ReviewFactRow applies a single-fact user governance action via a precise
// column-targeted UPDATE (memory.md §9.4). Unlike UpsertFactRow it never
// touches links/keywords/metadata/quality_score, so feedback actions cannot
// silently wipe A-MEM graph linkages.
//
// Action semantics (L3.md acceptance: confirm +0.10 / reject -0.20, clamped
// to [0,1]):
//   - confirm:   positive_feedback_count+1, confidence bump
//   - reject:    negative_feedback_count+1, confidence drop
//   - archive:   status='archived' (forget)
//   - dispute:   status='disputed'   (conflict governance quarantine)
//   - deprecate: status='deprecated' (conflict arbitration loser)
//   - refine:    statement/details/kind/tags replace, version+1, fingerprint
//     recomputed, PII gate re-run, embedding marked stale
//
// Refine recomputes the fingerprint from the new statement: keeping the stale
// fingerprint would let the next auto-extraction of the OLD text merge via
// ON CONFLICT and silently revert the user's edit.
func (r *l3FactRepo) ReviewFactRow(ctx context.Context, in biz.FactReview) ([]byte, error) {
	factID := strings.TrimSpace(in.FactID)
	if factID == "" {
		return nil, apierror.BadRequest("MEMORY", "fact_id is required")
	}
	action := strings.TrimSpace(in.Action)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	d := r.data.Dialect()

	var updateSQL string
	var args []any
	switch action {
	case biz.FactReviewConfirm:
		updateSQL = `UPDATE memory_facts SET positive_feedback_count = positive_feedback_count + 1, confidence = ` +
			d.Least("1.0", d.Greatest("0.0", "confidence + 0.10")) +
			`, updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewReject:
		updateSQL = `UPDATE memory_facts SET negative_feedback_count = negative_feedback_count + 1, confidence = ` +
			d.Least("1.0", d.Greatest("0.0", "confidence - 0.20")) +
			`, updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewArchive:
		updateSQL = `UPDATE memory_facts SET status = 'archived', archived_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, now, factID}
	case biz.FactReviewDispute:
		updateSQL = `UPDATE memory_facts SET status = 'disputed', updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewDeprecate:
		updateSQL = `UPDATE memory_facts SET status = 'deprecated', updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewRefine:
		// SQL built inside the transaction (needs the row's scope to recompute fingerprint).
	default:
		return nil, apierror.BadRequest("MEMORY", "unknown fact review action: "+action)
	}

	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		if action == biz.FactReviewRefine {
			sql_, args_, buildErr := r.buildFactRefineUpdate(txCtx, in, factID, now)
			if buildErr != nil {
				return buildErr
			}
			updateSQL, args = sql_, args_
		}
		res, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx, d.RenumberPlaceholders(updateSQL), args...)
		if execErr != nil {
			return execErr
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return apierror.NotFound("MEMORY", "fact not found or deleted")
		}
		rows, queryErr := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			d.RenumberPlaceholders(sqlFactSelect+` WHERE id = ?`), factID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return err
			}
			return apierror.NotFound("MEMORY", "fact row not found after review")
		}
		var scanErr error
		result, scanErr = scanFactRowJSON(rows)
		return scanErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}

// buildFactRefineUpdate constructs the UPDATE for the refine action. It reads
// the fact's scope inside the transaction so the fingerprint can be recomputed
// from the refined statement (matching UpsertFactRow's gate-then-fingerprint
// order). A fingerprint collision with an existing fact surfaces as
// CodeConflict via entErrToBizErr, which is the desired duplicate guard.
func (r *l3FactRepo) buildFactRefineUpdate(ctx context.Context, in biz.FactReview, factID, now string) (string, []any, error) {
	var scopeType, scopeID string
	err := QueryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT scope_type, scope_id FROM memory_facts WHERE id = ? AND deleted_at = ''`),
		[]any{factID}, &scopeType, &scopeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil, apierror.NotFound("MEMORY", "fact not found or deleted")
		}
		return "", nil, err
	}
	statement, details, pii, redacted, piiTypesJSON := applyFactPIIGate(biz.FactUpsert{
		Statement:       in.Statement,
		DetailsMarkdown: in.DetailsMarkdown,
	})
	if statement == "" {
		return "", nil, apierror.BadRequest("MEMORY", "statement is required for refine")
	}
	fp := biz.FactFingerprint(statement, scopeType, scopeID)
	tags := strings.TrimSpace(in.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	updateSQL := `UPDATE memory_facts SET statement = ?, statement_normalized = ?, fingerprint = ?, details_markdown = ?, fact_kind = ?, tags_json = ?, version = version + 1, embedding_status = 'stale', pii_flag = ?, redacted_statement = ?, pii_types = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`
	args := []any{statement, strings.ToLower(statement), fp, details, strings.TrimSpace(in.FactKind), tags, pii, redacted, piiTypesJSON, now, factID}
	return updateSQL, args, nil
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

func (r *l3FactRepo) ListConflictingFacts(ctx context.Context, scopeType, scopeID, agentID string, limit, offset int32) ([][]byte, int32, error) {
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
	// H2: agent_id filters by ORIGINATING agent across ALL scopes (same
	// caliber as buildFactFilterClauses / F1).
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
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

// SupersedeFact marks oldID as superseded by newID so the old statement no
// longer participates in recall or conflict arbitration.
func (r *l3FactRepo) SupersedeFact(ctx context.Context, oldID, newID string) error {
	if oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET status = 'superseded', superseded_by = ?, updated_at = ? WHERE id = ? AND status = 'active'`),
		newID, time.Now().UTC().Format(time.RFC3339Nano), oldID)
	return entErrToBizErr(err, "MEMORY_L3")
}

// ListActivePreferenceFacts returns active facts of the given kinds within the
// user+agent scopes, ordered by importance DESC, updated_at DESC (FR-M3 pinned
// preference injection).
func (r *l3FactRepo) ListActivePreferenceFacts(ctx context.Context, agentID, userID string, kinds []string, limit int32) ([][]byte, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	kindPH := make([]string, len(kinds))
	args := make([]any, 0, len(kinds)+4)
	for i, k := range kinds {
		kindPH[i] = "?"
		args = append(args, k)
	}
	where := " WHERE status = 'active' AND deleted_at = '' AND valid_until = ''" +
		" AND fact_kind IN (" + strings.Join(kindPH, ",") + ")" +
		" AND ((scope_type = 'user' AND scope_id = ?) OR (scope_type = 'agent' AND scope_id = ?))"
	args = append(args, userID, agentID)
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	if lim > 50 {
		lim = 50
	}
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
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
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

// IncrementFactRecalledCount batch-increments recalled_count and updates
// last_used_at for the given fact IDs. Called by the scored recall adapter so
// the Ebbinghaus decay worker has accurate access-recency signals. The updates
// are wrapped in a single transaction to minimize write load on the recall
// path.
//
// FR-12.6: this is the "recalled" stage of the three-stage counters — the
// fact entered a recall result set (previously mis-labelled use_count).
//
// factIDs must be non-empty; an empty slice is a no-op.
func (r *l3FactRepo) IncrementFactRecalledCount(ctx context.Context, factIDs []string) error {
	return r.incrementFactCounter(ctx, factIDs, "recalled_count")
}

// IncrementFactInjectedCount batch-increments injected_count and updates
// last_used_at for the given fact IDs. Called by the before-model memory
// inject hook once per turn for the facts actually written into the prompt
// (FR-12.6: the "injected" stage — the only usage count shown to users).
func (r *l3FactRepo) IncrementFactInjectedCount(ctx context.Context, factIDs []string) error {
	return r.incrementFactCounter(ctx, factIDs, "injected_count")
}

// incrementFactCounter is the shared batch-increment helper for the
// recalled/injected stages. column is a compile-time constant from the two
// callers above — never user input (SQL injection safe by construction).
func (r *l3FactRepo) incrementFactCounter(ctx context.Context, factIDs []string, column string) error {
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
		q := fmt.Sprintf(`UPDATE memory_facts SET %s = %s + 1, last_used_at = ? WHERE id IN (%s)`,
			column, column, strings.Join(placeholders, ","))
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(q), args...)
		return entErrToBizErr(execErr, "MEMORY_L3")
	})
}

// RecordFactCitations records (fact, turn) citations into the dedup ledger
// and increments cited_count only for pairs not seen before (FR-12.6: the
// "cited" stage). Idempotent: re-recording the same (fact_id, turn_id) pair
// is a no-op, so the backfill worker may re-scan overlapping windows freely.
// last_used_at is updated only for newly-cited facts.
func (r *l3FactRepo) RecordFactCitations(ctx context.Context, citations []biz.FactCitation) error {
	if r == nil || r.data == nil || len(citations) == 0 {
		return nil
	}
	insertSQL := `INSERT INTO memory_fact_citations (fact_id, turn_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`
	if !r.data.Dialect().IsPostgres() {
		insertSQL = `INSERT OR IGNORE INTO memory_fact_citations (fact_id, turn_id, created_at) VALUES (?, ?, ?)`
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, c := range citations {
			factID := strings.TrimSpace(c.FactID)
			turnID := strings.TrimSpace(c.TurnID)
			if factID == "" || turnID == "" {
				continue
			}
			res, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(insertSQL), factID, turnID, now)
			if execErr != nil {
				return entErrToBizErr(execErr, "MEMORY_L3")
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				continue // already recorded — idempotent skip
			}
			if _, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET cited_count = cited_count + 1, last_used_at = ? WHERE id = ?`),
				now, factID); execErr != nil {
				return entErrToBizErr(execErr, "MEMORY_L3")
			}
		}
		return nil
	})
}
