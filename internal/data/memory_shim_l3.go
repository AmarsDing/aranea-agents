package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"
)

// l3FactRepo implements biz L3 interfaces using direct Raw SQL.
type l3FactRepo struct {
	data        *Data
	vectorStore vector.VectorStore
}

// Compile-time interface checks.
var (
	_ biz.L3FactReader    = (*l3FactRepo)(nil)
	_ biz.L3FactWriter    = (*l3FactRepo)(nil)
	_ biz.L3ConflictStore = (*l3FactRepo)(nil)
	_ biz.PIIReviewStore  = (*l3FactRepo)(nil)
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
		`SELECT status, embedding_status, statement FROM memory_facts WHERE id = ?`,
		[]any{factID}, &status, &indexStatus, &statement)
	return
}

func (a *factConsistencyAdapter) GetFactResyncRow(ctx context.Context, factID string) (agentID, userID, statement string, err error) {
	err = QueryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		`SELECT COALESCE(agent_id, scope_id), user_id, statement FROM memory_facts WHERE id = ?`,
		[]any{factID}, &agentID, &userID, &statement)
	return
}

// --- L3FactReader ---

func (r *l3FactRepo) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
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
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	} else {
		clauses = append(clauses, "status = 'active'")
	}
	if keyword != "" {
		clauses = append(clauses, "statement_normalized LIKE ?")
		args = append(args, "%"+strings.ToLower(keyword)+"%")
	}
	clauses = append(clauses, "deleted_at = ''")

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	// Count total (without status filter) for the same scope/kind/keyword.
	var total int32
	totalClauses := []string{}
	totalArgs := []any{}
	if scopeType != "" {
		totalClauses = append(totalClauses, "scope_type = ?")
		totalArgs = append(totalArgs, scopeType)
	}
	if scopeID != "" {
		totalClauses = append(totalClauses, "scope_id = ?")
		totalArgs = append(totalArgs, scopeID)
	}
	if kind != "" {
		totalClauses = append(totalClauses, "fact_kind = ?")
		totalArgs = append(totalArgs, kind)
	}
	if keyword != "" {
		totalClauses = append(totalClauses, "statement_normalized LIKE ?")
		totalArgs = append(totalArgs, "%"+strings.ToLower(keyword)+"%")
	}
	totalClauses = append(totalClauses, "deleted_at = ''")
	totalWhere := ""
	if len(totalClauses) > 0 {
		totalWhere = " WHERE " + strings.Join(totalClauses, " AND ")
	}
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_facts"+totalWhere, totalArgs, &total); err != nil {
		return nil, 0, 0, 0, err
	}
	// Count active (with status = 'active' filter).
	var active int32
	activeClauses := []string{"status = 'active'"}
	activeArgs := []any{}
	if scopeType != "" {
		activeClauses = append(activeClauses, "scope_type = ?")
		activeArgs = append(activeArgs, scopeType)
	}
	if scopeID != "" {
		activeClauses = append(activeClauses, "scope_id = ?")
		activeArgs = append(activeArgs, scopeID)
	}
	if kind != "" {
		activeClauses = append(activeClauses, "fact_kind = ?")
		activeArgs = append(activeArgs, kind)
	}
	if keyword != "" {
		activeClauses = append(activeClauses, "statement_normalized LIKE ?")
		activeArgs = append(activeArgs, "%"+strings.ToLower(keyword)+"%")
	}
	activeClauses = append(activeClauses, "deleted_at = ''")
	activeWhere := " WHERE " + strings.Join(activeClauses, " AND ")
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_facts"+activeWhere, activeArgs, &active); err != nil {
		active = total
	}
	// Count archived (with status = 'archived' filter, same scope/kind/keyword as main query).
	var archived int32
	archivedClauses := []string{"status = 'archived'", "deleted_at = ''"}
	archivedArgs := []any{}
	if scopeType != "" {
		archivedClauses = append(archivedClauses, "scope_type = ?")
		archivedArgs = append(archivedArgs, scopeType)
	}
	if scopeID != "" {
		archivedClauses = append(archivedClauses, "scope_id = ?")
		archivedArgs = append(archivedArgs, scopeID)
	}
	if kind != "" {
		archivedClauses = append(archivedClauses, "fact_kind = ?")
		archivedArgs = append(archivedArgs, kind)
	}
	if keyword != "" {
		archivedClauses = append(archivedClauses, "statement_normalized LIKE ?")
		archivedArgs = append(archivedArgs, "%"+strings.ToLower(keyword)+"%")
	}
	archivedWhere := " WHERE " + strings.Join(archivedClauses, " AND ")
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_facts"+archivedWhere, archivedArgs, &archived); err != nil {
		archived = 0
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		out = append(out, b)
	}
	return out, total, active, archived, rows.Err()
}

func (r *l3FactRepo) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
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
	q := sqlFactSelect + " WHERE id IN (" + strings.Join(placeholders, ",") + ") AND status = 'active' AND deleted_at = ''"
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
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
	where := " WHERE " + strings.Join(clauses, " AND ")
	var count int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_facts"+where, args, &count); err != nil {
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
	where := " WHERE " + strings.Join(clauses, " AND ")
	q := sqlFactSelect + where + ` ORDER BY importance DESC, updated_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
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
	return out, rows.Err()
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
	where := " WHERE " + strings.Join(clauses, " AND ")
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, pool)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
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
		decay := factRecencyDecay(updatedAt, now)
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
	applyCrossEncoderRerankToFactScored(query, scored, passages)
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
	clauses := []string{"status = 'active'", "deleted_at = ''"}
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
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

		vecScore := hitMap[id]
		kwScore := keywordOverlapScore(tokens, stmt+" "+details)
		recency := recencyBoost(updatedAt, now)
		decay := factRecencyDecay(updatedAt, now)
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
	applyCrossEncoderRerankToFactScored(query, scored, passages)
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

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_facts (
		id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		pii_flag, redacted_statement,
		quality_score, pii_types, metadata_json, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
		statement = excluded.statement, details_markdown = excluded.details_markdown,
		confidence = excluded.confidence, importance = excluded.importance,
		use_count = use_count + excluded.use_count, hit_count = hit_count + excluded.hit_count,
		positive_feedback_count = positive_feedback_count + excluded.positive_feedback_count,
		negative_feedback_count = negative_feedback_count + excluded.negative_feedback_count,
		conflict_count = conflict_count + excluded.conflict_count,
		fact_kind = excluded.fact_kind, tags_json = excluded.tags_json,
		source_kind = excluded.source_kind, source_episode_id = excluded.source_episode_id,
		source_session_id = excluded.source_session_id, source_message_id = excluded.source_message_id,
		source_external = excluded.source_external,
		version = version + 1, status = excluded.status,
		pii_flag = excluded.pii_flag, redacted_statement = excluded.redacted_statement,
		quality_score = excluded.quality_score, pii_types = excluded.pii_types, metadata_json = excluded.metadata_json,
		updated_at = excluded.updated_at`,
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
		pii, redacted,
		defaultFactQualityScore, piiTypesJSON, meta, createdAt, updatedAt,
	)
	if err != nil {
		return nil, err
	}
	// Read back the row using the unique constraint (scope_type, scope_id, fingerprint)
	// rather than fingerprint alone. The triple is the actual unique key, so this
	// is deterministic even under concurrent writes.
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		sqlFactSelect+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`,
		strings.TrimSpace(in.ScopeType), strings.TrimSpace(in.ScopeID), fp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("fact row read-back failed: %w", err)
		}
		return nil, errors.New("fact row not found after upsert")
	}
	return scanFactRowJSON(rows)
}

func (r *l3FactRepo) DeleteFactRow(ctx context.Context, factID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE id = ?`, now, factID)
	if err != nil {
		return err
	}
	// Cascade: remove pgvector embedding so deleted facts are not recalled.
	if r.vectorStore != nil {
		if delErr := r.vectorStore.Delete(ctx, factID); delErr != nil {
			// Best-effort: log but don't fail the primary delete.
			r.data.lg.Warn("cascade: delete fact vector failed (best-effort)",
				loggateway.StepID("memory.l3.vector_delete"),
				loggateway.Str("fact_id", factID),
				loggateway.Err(delErr))
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
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	// Cascade: remove pgvector embeddings so deleted facts are not recalled.
	if r.vectorStore != nil {
		for _, fid := range factIDs {
			if delErr := r.vectorStore.Delete(ctx, fid); delErr != nil {
				r.data.lg.Warn("cascade: delete fact vector failed (best-effort)",
					loggateway.StepID("memory.l3.vector_delete_batch"),
					loggateway.Str("fact_id", fid),
					loggateway.Err(delErr))
			}
		}
	}
	return int(n), nil
}

func (r *l3FactRepo) ClearFactsByScope(ctx context.Context, scopeType, scopeID, userID string) ([]string, error) {
	// 1. Collect all active fact IDs before soft-deleting them.
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id FROM memory_facts WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`,
		scopeType, scopeID, userID)
	if err != nil {
		return nil, err
	}
	var factIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		factIDs = append(factIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(factIDs) == 0 {
		return nil, nil
	}

	// 2. Soft-delete in SQLite.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`,
		now, scopeType, scopeID, userID)
	if err != nil {
		return nil, err
	}

	// 3. Cascade: remove pgvector embeddings so deleted facts are not recalled.
	if r.vectorStore != nil {
		for _, fid := range factIDs {
			if delErr := r.vectorStore.Delete(ctx, fid); delErr != nil {
				r.data.lg.Warn("cascade: clear fact vector failed (best-effort)",
					loggateway.StepID("memory.l3.vector_clear"),
					loggateway.Str("fact_id", fid),
					loggateway.Err(delErr))
			}
		}
	}
	return factIDs, nil
}

// --- L3ConflictStore ---

func (r *l3FactRepo) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET conflict_count = conflict_count + 1, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	if err != nil {
		return 0, err
	}
	var count int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), `SELECT conflict_count FROM memory_facts WHERE id = ?`, []any{factID}, &count); err != nil {
		return 0, err
	}
	return count, nil
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
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_facts"+where, args, &total); err != nil {
		return nil, 0, err
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
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
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_facts"+where, args, &total); err != nil {
		return nil, 0, err
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

func (r *l3FactRepo) ApprovePIIFact(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET pii_flag = 0, statement = COALESCE(NULLIF(redacted_statement, ''), statement), redacted_statement = '', pii_types = '[]', updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return err
}

func (r *l3FactRepo) RejectPIIFact(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET status = 'redacted', updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return err
}

// ensure math is referenced
var _ = math.Pi
