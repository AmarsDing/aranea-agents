package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
)

// l2EpisodeRepo implements biz.L2EpisodeWriter + biz.L2RecallStore using direct Raw SQL.
type l2EpisodeRepo struct {
	data        *Data
	vectorStore vector.VectorStore
}

func newL2EpisodeRepo(data *Data, vs vector.VectorStore) *l2EpisodeRepo {
	if data == nil {
		return nil
	}
	return &l2EpisodeRepo{data: data, vectorStore: vs}
}

// Compile-time interface checks.
var (
	_ biz.L2EpisodeWriter = (*l2EpisodeRepo)(nil)
	_ biz.L2RecallStore   = (*l2EpisodeRepo)(nil)
)

// --- L2EpisodeWriter ---

func (r *l2EpisodeRepo) InsertL1ArchiveEpisode(ctx context.Context, in biz.L1ArchiveEpisodeInsert) error {
	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(in.TaskTitle)
	if title == "" {
		title = "L1 Archive: " + in.TaskID
	}
	outcomeSummary := strings.TrimSpace(in.OutcomeSummary)
	if outcomeSummary == "" {
		outcomeSummary = strings.TrimSpace(in.Status)
	}
	if outcomeSummary == "" {
		outcomeSummary = "completed"
	}
	goal := strings.TrimSpace(in.Goal)
	outcome := strings.TrimSpace(in.Outcome)
	if outcome == "" {
		outcome = outcomeSummary
	}
	episodeKind := strings.TrimSpace(in.EpisodeKind)
	if episodeKind == "" {
		episodeKind = "l1_archive"
	}
	keyDecisionsJSON := strings.TrimSpace(in.KeyDecisionsJSON)
	if keyDecisionsJSON == "" {
		keyDecisionsJSON = "[]"
	}
	keyArtifactsJSON := strings.TrimSpace(in.KeyArtifactsJSON)
	if keyArtifactsJSON == "" {
		keyArtifactsJSON = "[]"
	}
	l1SnapshotJSON := strings.TrimSpace(in.L1SnapshotJSON)
	if l1SnapshotJSON == "" {
		l1SnapshotJSON = "{}"
	}
	importance := in.Importance
	if importance <= 0 {
		importance = 0.5
	}
	confidence := in.Confidence
	if confidence <= 0 {
		confidence = 0.6
	}
	consolidationStatus := "consolidated"
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_episodes (
		id, session_id, agent_id, l1_task_id, episode_kind, title, goal,
		outcome, outcome_summary, importance, confidence,
		key_decisions_json, key_artifacts_json, l1_snapshot_json,
		consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(session_id, l1_task_id) WHERE l1_task_id != '' DO UPDATE SET
		goal = excluded.goal, outcome = excluded.outcome,
		outcome_summary = excluded.outcome_summary, importance = excluded.importance,
		confidence = excluded.confidence,
		key_decisions_json = excluded.key_decisions_json,
		key_artifacts_json = excluded.key_artifacts_json,
		l1_snapshot_json = excluded.l1_snapshot_json,
		title = excluded.title,
		episode_kind = excluded.episode_kind,
		ended_at = excluded.ended_at`),
		id,
		strings.TrimSpace(in.SessionID),
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.TaskID),
		episodeKind,
		title,
		goal,
		outcome,
		outcomeSummary,
		importance,
		confidence,
		keyDecisionsJSON,
		keyArtifactsJSON,
		l1SnapshotJSON,
		consolidationStatus, 0, "{}", now, now,
	)
	return entErrToBizErr(err, "MEMORY_L2")
}

// --- L2RecallStore ---

func (r *l2EpisodeRepo) ListEpisodeRowsForRecall(ctx context.Context, agentID, sessionID string, limit int32) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 50
	}
	q := sqlEpisodeSelect + ` WHERE agent_id = ? AND deleted_at = ''`
	args := []any{agentID}
	if sessionID != "" {
		q += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L2")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanEpisodeRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L2")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L2")
}

func (r *l2EpisodeRepo) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	if r.vectorStore != nil && len(queryEmbedding) > 0 {
		return r.recallL2WithVectorStore(ctx, agentID, sessionID, query, queryEmbedding, limit)
	}
	return r.recallL2Episodes(ctx, agentID, sessionID, query, queryEmbedding, limit)
}

func (r *l2EpisodeRepo) recallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	pool := l2RecallCandidatePool
	if pool < lim {
		pool = lim
	}
	candidates, err := r.ListEpisodeRowsForRecall(ctx, agentID, sessionID, int32(pool))
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	tokens := tokenizeQuery(query)
	now := time.Now().UTC()
	var scored []scoredEpisode
	for _, raw := range candidates {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		title, _ := row["title"].(string)
		summary, _ := row["outcome_summary"].(string)
		id, _ := row["id"].(string)
		imp := float64(anyFloat(row, "importance"))
		endedAt, _ := row["ended_at"].(string)
		_ = row["created_at"]

		kwScore := keywordOverlapScore(tokens, title+" "+summary)
		var vecScore float64
		if len(queryEmbedding) > 0 {
			// JSON unmarshal produces string (not []byte) for binary columns,
			// so we must handle both types (same pattern as L3 BUG-7 fix).
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
		decay := decayFactor(endedAt, now)
		sessionBoost := 0.0
		if sessionID != "" {
			if s, ok := row["session_id"].(string); ok && s == sessionID {
				sessionBoost = 0.1
			}
		}
		total := l2ScoreWeightKeyword*kwScore +
			l2ScoreWeightVector*vecScore +
			l2ScoreWeightImport*imp*decay +
			l2ScoreWeightSession*sessionBoost

		scored = append(scored, scoredEpisode{
			raw:     raw,
			id:      id,
			title:   title,
			summary: summary,
			score:   total,
			breakdown: recallScoreBreakdown{
				Keyword:    kwScore,
				Vector:     vecScore,
				Importance: imp * decay,
				Total:      total,
			},
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > lim {
		scored = scored[:lim]
	}
	// Apply cross-encoder rerank
	passages := make([]string, len(scored))
	for i, s := range scored {
		passages[i] = episodePassage(s.raw)
	}
	applyCrossEncoderRerankToScored(r.data.Reranker(), query, scored, passages, func(i int, ceScore, total float64) {
		scored[i].breakdown.CrossEncoder = ceScore
		scored[i].breakdown.Total = total
		scored[i].score = total
	})
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	var out [][]byte
	for _, s := range scored {
		out = append(out, s.raw)
	}
	return out, nil
}

func (r *l2EpisodeRepo) recallL2WithVectorStore(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	pool := l2RecallCandidatePool
	if pool < lim {
		pool = lim
	}
	// Get vector hits from VectorStore
	vecHits, err := r.vectorStore.Search(ctx, float32To64(queryEmbedding), pool, 0.3)
	if err != nil || len(vecHits) == 0 {
		return r.recallL2Episodes(ctx, agentID, sessionID, query, queryEmbedding, limit)
	}
	// Build ID set for SQL IN clause
	ids := make([]string, 0, len(vecHits))
	hitMap := make(map[string]float64, len(vecHits))
	for _, h := range vecHits {
		ids = append(ids, h.ID)
		hitMap[h.ID] = h.Score
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids)+1)
	args[0] = agentID
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+1] = id
	}
	q := fmt.Sprintf(`%s WHERE agent_id = ? AND id IN (%s)`, sqlEpisodeSelect, strings.Join(placeholders, ","))
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := tokenizeQuery(query)
	now := time.Now().UTC()
	var scored []scoredEpisode
	for rows.Next() {
		b, err := scanEpisodeRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(b, &row) != nil {
			continue
		}
		id, _ := row["id"].(string)
		title, _ := row["title"].(string)
		summary, _ := row["outcome_summary"].(string)
		imp := float64(anyFloat(row, "importance"))
		endedAt, _ := row["ended_at"].(string)

		vecScore := hitMap[id]
		kwScore := keywordOverlapScore(tokens, title+" "+summary)
		decay := decayFactor(endedAt, now)
		sessionBoost := 0.0
		if sessionID != "" {
			if s, ok := row["session_id"].(string); ok && s == sessionID {
				sessionBoost = 0.1
			}
		}
		total := l2ScoreWeightKeyword*kwScore +
			l2ScoreWeightVector*vecScore +
			l2ScoreWeightImport*imp*decay +
			l2ScoreWeightSession*sessionBoost

		scored = append(scored, scoredEpisode{
			raw:     b,
			id:      id,
			title:   title,
			summary: summary,
			score:   total,
			breakdown: recallScoreBreakdown{
				Keyword:    kwScore,
				Vector:     vecScore,
				Importance: imp * decay,
				Total:      total,
			},
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > lim {
		scored = scored[:lim]
	}
	passages := make([]string, len(scored))
	for i, s := range scored {
		passages[i] = episodePassage(s.raw)
	}
	applyCrossEncoderRerankToScored(r.data.Reranker(), query, scored, passages, func(i int, ceScore, total float64) {
		scored[i].breakdown.CrossEncoder = ceScore
		scored[i].breakdown.Total = total
		scored[i].score = total
	})
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	var out [][]byte
	for _, s := range scored {
		out = append(out, s.raw)
	}
	return out, nil
}

// anyFloat extracts a float from a map value (handles float64, int, etc.)
func anyFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// anyInt extracts an int from a map value
func anyInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
