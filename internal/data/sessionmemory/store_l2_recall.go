package sessionmemory

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	l2RecallCandidatePool = 40
	l2DecayHalfLifeDays   = 14.0
	l2ScoreWeightKeyword  = 0.35
	l2ScoreWeightVector   = 0.45
	l2ScoreWeightImport   = 0.15
	l2ScoreWeightSession  = 0.05
)

type scoredEpisode struct {
	raw         []byte
	id          string
	title       string
	summary     string
	score       float64
	breakdown   recallScoreBreakdown
}

// RecallL2Episodes retrieves episodes with keyword/vector fusion, importance decay, and rerank.
func (st *Store) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	scored, err := st.RecallL2EpisodesScored(ctx, agentID, sessionID, query, queryEmbedding, limit)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(scored))
	for i := range scored {
		out[i] = scored[i].Raw
	}
	return out, nil
}

// RecallL2EpisodesScored returns L2 recall hits with full score breakdown (admin debug / composite search).
func (st *Store) RecallL2EpisodesScored(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([]RecallDebugRow, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 3
	}
	if lim > 10 {
		lim = 10
	}

	const sqlEpisodeRecallSelect = `SELECT id, session_id, agent_id, episode_kind, title, outcome_summary, importance,
 consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at, embedding_blob FROM memory_episodes`

	rows, err := st.client.QueryContext(ctx, sqlEpisodeRecallSelect+`
 WHERE agent_id = ? AND deleted_at = '' AND consolidation_status = 'consolidated'
 ORDER BY importance DESC, ended_at DESC LIMIT ?`, agentID, l2RecallCandidatePool)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	query = strings.ToLower(strings.TrimSpace(query))
	queryTokens := tokenizeQuery(query)
	now := time.Now().UTC()

	var scored []scoredEpisode
	for rows.Next() {
		var (
			id, sessionRow, agentRow, kind, title, summary, status, meta, endedAt, createdAt string
			l3Count                                                                              int
			importance                                                                           float64
			embedBlob                                                                            []byte
		)
		if err := rows.Scan(&id, &sessionRow, &agentRow, &kind, &title, &summary, &importance, &status, &l3Count, &meta, &endedAt, &createdAt, &embedBlob); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(map[string]any{
			"id": id, "session_id": sessionRow, "agent_id": agentRow, "episode_kind": kind,
			"title": title, "outcome_summary": summary, "importance": importance,
			"consolidation_status": status, "consolidated_l3_count": l3Count,
			"metadata_json": meta, "ended_at": endedAt, "created_at": createdAt,
		})
		if err != nil {
			continue
		}

		titleLower := strings.ToLower(strings.TrimSpace(title))
		summaryLower := strings.ToLower(strings.TrimSpace(summary))
		text := titleLower + " " + summaryLower

		kwScore := keywordOverlapScore(queryTokens, text)
		vecScore := 0.0
		if len(queryEmbedding) > 0 && len(embedBlob) > 0 {
			if epVec := decodeFloat32Blob(embedBlob); len(epVec) == len(queryEmbedding) {
				vecScore = cosineSimilarity(queryEmbedding, epVec)
				if vecScore < 0 {
					vecScore = 0
				}
			}
		}
		imp := importance
		if imp <= 0 {
			imp = 0.5
		}
		decayedImp := imp * decayFactor(endedAt, now)
		sessBoost := 0.0
		if sessionID != "" && sessionRow == sessionID {
			sessBoost = 1.0
		}
		total := l2ScoreWeightKeyword*kwScore + l2ScoreWeightVector*vecScore + l2ScoreWeightImport*decayedImp + l2ScoreWeightSession*sessBoost
		if query == "" && kwScore == 0 && vecScore == 0 {
			total = decayedImp + sessBoost*0.1
		}
		scored = append(scored, scoredEpisode{
			raw: raw, id: id, title: title, summary: summary, score: total,
			breakdown: recallScoreBreakdown{
				Keyword: kwScore, Vector: vecScore, Importance: decayedImp,
				Recency: sessBoost, Total: total,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if query != "" && len(scored) > 1 {
		passages := make([]string, len(scored))
		for i := range scored {
			passages[i] = episodePassage(scored[i].raw)
		}
		applyCrossEncoderRerankToScored(query, scored, passages, func(i int, ceScore, total float64) {
			scored[i].breakdown.CrossEncoder = ceScore
			scored[i].breakdown.Total = total
			scored[i].score = total
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	out := make([]RecallDebugRow, 0, lim)
	for i := 0; i < len(scored) && len(out) < lim; i++ {
		s := scored[i]
		out = append(out, RecallDebugRow{
			Layer: "L2", ID: s.id, Title: s.title, Summary: s.summary,
			Scores: s.breakdown, Raw: json.RawMessage(s.raw),
		})
	}
	return out, nil
}

// ApplyEpisodeImportanceDecay reduces stored importance for episodes older than cutoff (batch maintenance).
func (st *Store) ApplyEpisodeImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	agentID = strings.TrimSpace(agentID)
	cutoffRFC3339 = strings.TrimSpace(cutoffRFC3339)
	if agentID == "" || cutoffRFC3339 == "" || factor <= 0 || factor >= 1 {
		return 0, nil
	}
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_episodes SET
 importance = MAX(0.05, importance * ?),
 updated_at = ?
WHERE agent_id = ? AND deleted_at = '' AND ended_at != '' AND ended_at < ?`,
		factor, time.Now().UTC().Format(time.RFC3339Nano), agentID, cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
			Action:        "L2_DECAY",
			TargetKind:    "episode_scope",
			TargetID:      agentID,
			Reason:        cutoffRFC3339,
			PolicyVersion: "l2_decay_v1",
		})
	}
	return int(n), nil
}

// ApplyAllEpisodeImportanceDecay batch-decays episode importance for every agent scope.
func (st *Store) ApplyAllEpisodeImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	cutoffRFC3339 = strings.TrimSpace(cutoffRFC3339)
	if cutoffRFC3339 == "" || factor <= 0 || factor >= 1 {
		return 0, nil
	}
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_episodes SET
 importance = MAX(0.05, importance * ?),
 updated_at = ?
WHERE deleted_at = '' AND ended_at != '' AND ended_at < ?`,
		factor, time.Now().UTC().Format(time.RFC3339Nano), cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
			Action:        "L2_DECAY",
			TargetKind:    "episode_scope",
			TargetID:      "global",
			Reason:        cutoffRFC3339,
			PolicyVersion: "l2_decay_v1",
		})
	}
	return int(n), nil
}

// UpsertEpisodeEmbedding stores a recall index vector on one episode row.
func (st *Store) UpsertEpisodeEmbedding(ctx context.Context, episodeID string, embedding []float32, model string, dim int) error {
	if st == nil || st.client == nil || strings.TrimSpace(episodeID) == "" || len(embedding) == 0 {
		return nil
	}
	blob := encodeFloat32Blob(embedding)
	norm := vectorL2Norm(embedding)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.client.ExecContext(ctx, `
UPDATE memory_episodes SET
 embedding_status = 'ready', embedding_model = ?, embedding_dim = ?,
 embedding_blob = ?, embedding_norm = ?, updated_at = ?
WHERE id = ? AND deleted_at = ''`,
		strings.TrimSpace(model), dim, blob, norm, now, strings.TrimSpace(episodeID))
	return err
}

func tokenizeQuery(q string) []string {
	if q == "" {
		return nil
	}
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) >= 2 {
			out = append(out, p)
		}
	}
	return out
}

func keywordOverlapScore(tokens []string, text string) float64 {
	if len(tokens) == 0 || text == "" {
		return 0
	}
	hits := 0
	for _, tok := range tokens {
		if strings.Contains(text, tok) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

func decayFactor(endedAt string, now time.Time) float64 {
	endedAt = strings.TrimSpace(endedAt)
	if endedAt == "" {
		return 1
	}
	t, err := time.Parse(time.RFC3339Nano, endedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, endedAt)
	}
	if err != nil {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Pow(0.5, days/l2DecayHalfLifeDays)
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func encodeFloat32Blob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeFloat32Blob(b []byte) []float32 {
	if len(b) < 4 {
		return nil
	}
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func vectorL2Norm(v []float32) float64 {
	var sum float64
	for _, f := range v {
		x := float64(f)
		sum += x * x
	}
	return math.Sqrt(sum)
}
