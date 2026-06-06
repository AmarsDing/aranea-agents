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
	// Count total
	var total int32
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	countQ := "SELECT COUNT(*) FROM memory_facts" + where
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countQ, countArgs, &total); err != nil {
		return nil, 0, 0, 0, err
	}
	// Count active
	var active int32
	activeArgs := make([]any, len(args))
	copy(activeArgs, args)
	activeQ := "SELECT COUNT(*) FROM memory_facts" + where
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), activeQ, activeArgs, &active); err != nil {
		active = total
	}
	// Count archived
	var archived int32
	archQ := "SELECT COUNT(*) FROM memory_facts WHERE status = 'archived' AND deleted_at = ''"
	if scopeType != "" {
		archQ += " AND scope_type = ?"
	}
	if scopeID != "" {
		archQ += " AND scope_id = ?"
	}
	archArgs := []any{}
	if scopeType != "" {
		archArgs = append(archArgs, scopeType)
	}
	if scopeID != "" {
		archArgs = append(archArgs, scopeID)
	}
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), archQ, archArgs, &archived); err != nil {
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

func (r *l3FactRepo) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	if r.vectorStore != nil && len(queryEmbedding) > 0 {
		return r.recallL3WithVectorStore(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
	}
	return r.recallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
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
		_ = anyFloat(row, "confidence")
		qScore := anyFloat(row, "quality_score")
		updatedAt, _ := row["updated_at"].(string)

		kwScore := keywordOverlapScore(tokens, stmt+" "+details)
		var vecScore float64
		if len(queryEmbedding) > 0 {
			embBlob, _ := row["embedding_blob"].([]byte)
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
		fp = factFingerprint(in.Statement, in.ScopeType, in.ScopeID)
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

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_facts (
		id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		pii_flag, redacted_statement,
		quality_score, metadata_json, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
		statement = excluded.statement, details_markdown = excluded.details_markdown,
		confidence = excluded.confidence, importance = excluded.importance,
		use_count = excluded.use_count, hit_count = excluded.hit_count,
		positive_feedback_count = excluded.positive_feedback_count,
		negative_feedback_count = excluded.negative_feedback_count,
		conflict_count = excluded.conflict_count,
		fact_kind = excluded.fact_kind, tags_json = excluded.tags_json,
		source_kind = excluded.source_kind, source_episode_id = excluded.source_episode_id,
		source_session_id = excluded.source_session_id, source_message_id = excluded.source_message_id,
		source_external = excluded.source_external,
		version = version + 1, status = excluded.status,
		pii_flag = excluded.pii_flag, redacted_statement = excluded.redacted_statement,
		quality_score = excluded.quality_score, metadata_json = excluded.metadata_json,
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
		pii, "",
		0.5, meta, createdAt, updatedAt,
	)
	if err != nil {
		return nil, err
	}
	// Read back the row — use fingerprint since ON CONFLICT may keep the original id
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlFactSelect+` WHERE fingerprint = ?`, fp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
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
	return int(n), nil
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
		`UPDATE memory_facts SET pii_flag = 0, redacted_statement = '', updated_at = ? WHERE id = ?`,
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
