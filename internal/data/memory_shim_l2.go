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
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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

// --- L2EpisodeWriter ---

// l2MergedTranscriptMaxChars bounds the merged episode transcript so
// long-running sessions cannot grow outcome/outcome_summary without limit.
const l2MergedTranscriptMaxChars = 12000

// l2EpisodeConflictSetReplace is the default (session_id, l1_task_id) upsert
// semantics: the incoming archive replaces the stored episode.
const l2EpisodeConflictSetReplace = `goal = excluded.goal, outcome = excluded.outcome,
	outcome_summary = excluded.outcome_summary, importance = excluded.importance,
	confidence = excluded.confidence,
	key_decisions_json = excluded.key_decisions_json,
	key_artifacts_json = excluded.key_artifacts_json,
	l1_snapshot_json = excluded.l1_snapshot_json,
	title = excluded.title,
	episode_kind = excluded.episode_kind,
	ended_at = excluded.ended_at,
	updated_at = excluded.updated_at`

// l2EpisodeMergeConflictSet appends the incoming transcript to the stored
// episode instead of replacing it. An identical replay is detected by
// substring match and leaves the transcript unchanged; goal keeps the first
// non-empty value. Used by the eval ingest path (MergeOutcome). maxChars
// bounds the merged transcript; <=0 falls back to l2MergedTranscriptMaxChars
// (AMC C3-A Q8a passes 32768 for the eval path).
func l2EpisodeMergeConflictSet(maxChars int) string {
	if maxChars <= 0 {
		maxChars = l2MergedTranscriptMaxChars
	}
	return fmt.Sprintf(`goal = CASE WHEN memory_episodes.goal = '' THEN excluded.goal ELSE memory_episodes.goal END,
	outcome = CASE
		WHEN memory_episodes.outcome = '' THEN excluded.outcome
		WHEN excluded.outcome = '' OR position(excluded.outcome in memory_episodes.outcome) > 0 THEN memory_episodes.outcome
		ELSE left(memory_episodes.outcome || E'\n' || excluded.outcome, %d) END,
	outcome_summary = CASE
		WHEN memory_episodes.outcome_summary = '' THEN excluded.outcome_summary
		WHEN excluded.outcome_summary = '' OR position(excluded.outcome_summary in memory_episodes.outcome_summary) > 0 THEN memory_episodes.outcome_summary
		ELSE left(memory_episodes.outcome_summary || E'\n' || excluded.outcome_summary, %d) END,
	importance = excluded.importance,
	confidence = excluded.confidence,
	key_decisions_json = excluded.key_decisions_json,
	key_artifacts_json = excluded.key_artifacts_json,
	l1_snapshot_json = excluded.l1_snapshot_json,
	title = excluded.title,
	episode_kind = excluded.episode_kind,
	ended_at = excluded.ended_at,
	updated_at = excluded.updated_at`, maxChars, maxChars)
}

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
	conflictSet := l2EpisodeConflictSetReplace
	if in.MergeOutcome {
		conflictSet = l2EpisodeMergeConflictSet(in.MaxMergeChars)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_episodes (
		id, session_id, agent_id, l1_task_id, episode_kind, title, goal,
		outcome, outcome_summary, importance, confidence,
		key_decisions_json, key_artifacts_json, l1_snapshot_json,
		consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(session_id, l1_task_id) WHERE l1_task_id != '' DO UPDATE SET `+conflictSet),
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
		consolidationStatus, 0, "{}", now, now, now,
	)
	return entErrToBizErr(err, "MEMORY_L2")
}

// StripEpisodeStatement removes a statement line from the merged eval episode
// transcript. The merge-on-conflict path (MergeOutcome) appends every batch,
// so when an L3 fact is superseded its line must also leave the L2
// transcript, otherwise recall keeps surfacing the stale value. The three
// anchored replaces cover middle/first/last line (and the bare one the
// single-line case); btrim cleans the freed edge newline.
func (r *l2EpisodeRepo) StripEpisodeStatement(ctx context.Context, agentID, sessionID, statement string) error {
	agentID = strings.TrimSpace(agentID)
	sessionID = strings.TrimSpace(sessionID)
	statement = strings.TrimSpace(statement)
	if agentID == "" || sessionID == "" || statement == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := `UPDATE memory_episodes
		SET outcome = btrim(replace(replace(replace(outcome, E'\n' || ?, ''), ? || E'\n', ''), ?, ''), E'\n'),
			outcome_summary = btrim(replace(replace(replace(outcome_summary, E'\n' || ?, ''), ? || E'\n', ''), ?, ''), E'\n'),
			updated_at = ?
		WHERE agent_id = ? AND session_id = ? AND l1_task_id != '' AND deleted_at = ''`
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q),
		statement, statement, statement, statement, statement, statement, now, agentID, sessionID)
	return entErrToBizErr(err, "MEMORY_L2")
}

// --- L2EpisodeAdminReader ---

func (r *l2EpisodeRepo) ListEpisodeRowsAdmin(ctx context.Context, agentID, sessionID string, limit, offset int32) ([][]byte, int32, int32, error) {
	if r == nil {
		return nil, 0, 0, apierror.Internal("MEMORY_L2", "l2 repo not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, 0, 0, nil
	}
	clauses := []string{`agent_id = ?`, `deleted_at = ''`}
	args := []any{agentID}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		clauses = append(clauses, `session_id = ?`)
		args = append(args, sessionID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")

	todayStart := time.Now().UTC().Truncate(24 * time.Hour).Format(time.RFC3339Nano)
	var total, today int32
	countQ := `SELECT COUNT(*), COALESCE(SUM(CASE WHEN created_at >= '` + todayStart + `' THEN 1 ELSE 0 END), 0) FROM memory_episodes` + where
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders(countQ), args, &total, &today); err != nil {
		return nil, 0, 0, entErrToBizErr(err, "MEMORY_L2")
	}

	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlEpisodeSelect + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, 0, entErrToBizErr(err, "MEMORY_L2")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanEpisodeRowJSON(rows)
		if err != nil {
			return nil, 0, 0, entErrToBizErr(err, "MEMORY_L2")
		}
		out = append(out, b)
	}
	return out, total, today, entErrToBizErr(rows.Err(), "MEMORY_L2")
}

func (r *l2EpisodeRepo) ListEpisodeRowsByIDs(ctx context.Context, ids []string) ([][]byte, error) {
	if r == nil {
		return nil, apierror.Internal("MEMORY_L2", "l2 repo not initialized")
	}
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return nil, nil
	}
	ph := make([]string, len(clean))
	args := make([]any, len(clean))
	for i, id := range clean {
		ph[i] = "?"
		args[i] = id
	}
	q := sqlEpisodeSelect + ` WHERE deleted_at = '' AND id IN (` + strings.Join(ph, ",") + `)`
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

// RecallL2Episodes ranks the agent's episodes for query. There is no
// episode-level vector index: vector_embeddings holds L3 fact vectors only
// (memoryFactIndexSync is its sole writer). The former vector path searched
// that table and looked the fact IDs up as episode IDs, which never match, so
// with pgvector enabled L2 recall came back empty. Semantic matching now goes
// through the facts: see sessionVectorScores.
func (r *l2EpisodeRepo) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
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
	// P2-R2: candidate pool is agent-wide (sessionID passed as ""). The
	// sessionID is still used below for the continuity session boost in
	// scoring, so same-session episodes outrank equal cross-session ones
	// without excluding them.
	candidates, err := r.ListEpisodeRowsForRecall(ctx, agentID, "", int32(pool))
	if err != nil {
		return nil, err
	}
	// Semantic leg: the recency pool misses older episodes, so add the
	// episodes of sessions whose facts match the query vector.
	sessionVec := r.sessionVectorScores(ctx, agentID, queryEmbedding, pool)
	if len(sessionVec) > 0 {
		extra, err := r.listEpisodeRowsForSessions(ctx, agentID, sessionVec, pool)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, extra...)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	tokens := tokenizeQuery(query)
	now := time.Now().UTC()
	var scored []scoredEpisode
	seen := make(map[string]struct{}, len(candidates))
	for _, raw := range candidates {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		title, _ := row["title"].(string)
		summary, _ := row["outcome_summary"].(string)
		id, _ := row["id"].(string)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
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
		if sess, _ := row["session_id"].(string); sess != "" {
			if s := sessionVec[sess]; s > vecScore {
				vecScore = s
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
		out = append(out, annotateEpisodeScores(s.raw, s.breakdown))
	}
	return out, nil
}

// sessionVectorScores proxies episode similarity through the fact vector
// index: it searches this agent's fact vectors and maps each hit to the
// fact's source session, keeping the best score per session. Episodes are
// written per session (eval: exactly one per user session), so a session
// whose facts match the query marks its episodes relevant. Fail-soft: a
// vector or SQL error leaves the recency/keyword legs only.
func (r *l2EpisodeRepo) sessionVectorScores(ctx context.Context, agentID string, queryEmbedding []float32, pool int) map[string]float64 {
	if r.vectorStore == nil || len(queryEmbedding) == 0 || strings.TrimSpace(agentID) == "" {
		return nil
	}
	hits, err := searchRecallVectors(ctx, r.vectorStore, queryEmbedding, pool, map[string]string{"agent_id": agentID})
	if err != nil {
		r.data.lg.Warn("L2 fact-vector session search failed, using recency/keyword recall",
			loggateway.StepID("memory.l2_vector_search"), loggateway.Err(err))
		return nil
	}
	hitScore := make(map[string]float64, len(hits))
	args := []any{agentID, agentID}
	ph := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.ID == "" {
			continue
		}
		if _, dup := hitScore[h.ID]; !dup {
			ph = append(ph, "?")
			args = append(args, h.ID)
		}
		hitScore[h.ID] = h.Score
	}
	if len(ph) == 0 {
		return nil
	}
	// Mirrors the meta agent_id written by memoryFactIndexSync (agent_id,
	// falling back to scope_id) and the recall gates on memory_facts.
	q := `SELECT id, source_session_id FROM memory_facts
 WHERE (COALESCE(agent_id, '') = ? OR (COALESCE(agent_id, '') = '' AND scope_id = ?))
 AND status = 'active' AND deleted_at = '' AND valid_until = '' AND ` + factPIIGateClause + `
 AND COALESCE(source_session_id, '') <> '' AND id IN (` + strings.Join(ph, ",") + `)`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		r.data.lg.Warn("L2 fact-vector session lookup failed, using recency/keyword recall",
			loggateway.StepID("memory.l2_vector_search"), loggateway.Err(err))
		return nil
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var id, sess string
		if err := rows.Scan(&id, &sess); err != nil {
			continue
		}
		if s := hitScore[id]; s > out[sess] {
			out[sess] = s
		}
	}
	if err := rows.Err(); err != nil {
		r.data.lg.Warn("L2 fact-vector session lookup failed, using recency/keyword recall",
			loggateway.StepID("memory.l2_vector_search"), loggateway.Err(err))
		return nil
	}
	return out
}

// listEpisodeRowsForSessions loads this agent's live episodes in the given
// sessions, newest first, capped at 2*pool rows.
func (r *l2EpisodeRepo) listEpisodeRowsForSessions(ctx context.Context, agentID string, sessions map[string]float64, pool int) ([][]byte, error) {
	if len(sessions) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(sessions))
	for s := range sessions {
		keys = append(keys, s)
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys)+2)
	args = append(args, agentID)
	ph := make([]string, len(keys))
	for i, s := range keys {
		ph[i] = "?"
		args = append(args, s)
	}
	args = append(args, 2*pool)
	q := sqlEpisodeSelect + ` WHERE agent_id = ? AND deleted_at = '' AND session_id IN (` + strings.Join(ph, ",") + `) ORDER BY created_at DESC LIMIT ?`
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
