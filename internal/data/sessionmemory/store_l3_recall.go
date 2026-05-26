package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	l3RecallCandidatePool = 60
	l3DecayHalfLifeDays   = 30.0
	l3ScoreWeightKeyword  = 0.40
	l3ScoreWeightVector   = 0.40
	l3ScoreWeightImport   = 0.15
	l3ScoreWeightRecency  = 0.05
)

type scoredFact struct {
	raw       []byte
	id        string
	stmt      string
	details   string
	score     float64
	breakdown recallScoreBreakdown
}

// RecallL3Facts retrieves semantic facts with keyword/vector fusion and rerank.
func (st *Store) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	scored, err := st.RecallL3FactsScored(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit)
	if err != nil {
		return nil, err
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 12
	}
	out := make([][]byte, 0, lim)
	for _, s := range scored {
		if minScore > 0 && strings.TrimSpace(query) != "" && s.Scores.Total < minScore {
			continue
		}
		out = append(out, s.Raw)
		if len(out) >= lim {
			break
		}
	}
	return out, nil
}

// RecallL3FactsScored returns L3 recall hits with full score breakdown.
func (st *Store) RecallL3FactsScored(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32) ([]RecallDebugRow, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil, nil
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 12
	}
	if lim > 20 {
		lim = 20
	}

	w := []string{"deleted_at = ''", "status = 'active'", "scope_type = ?", "scope_id = ?"}
	args := []any{scopeType, scopeID}
	if uid := strings.TrimSpace(userID); uid != "" {
		w = append(w, "(user_id = ? OR user_id = '')")
		args = append(args, uid)
	}
	whereSQL := strings.Join(w, " AND ")
	q := sqlFactSelect + ` WHERE ` + whereSQL + ` ORDER BY importance DESC, updated_at DESC LIMIT ?`
	listArgs := append(append([]any{}, args...), l3RecallCandidatePool)

	rows, err := st.client.QueryContext(ctx, q, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	query = strings.ToLower(strings.TrimSpace(query))
	queryTokens := tokenizeQuery(query)
	now := time.Now().UTC()

	var scored []scoredFact
	for rows.Next() {
		raw, stmt, details, importance, updatedAt, embedBlob, err := scanFactRowForRecall(rows)
		if err != nil {
			return nil, err
		}
		text := strings.ToLower(strings.TrimSpace(stmt) + " " + strings.TrimSpace(details))
		kwScore := keywordOverlapScore(queryTokens, text)
		vecScore := 0.0
		if len(queryEmbedding) > 0 && len(embedBlob) > 0 {
			if factVec := decodeFloat32Blob(embedBlob); len(factVec) == len(queryEmbedding) {
				vecScore = cosineSimilarity(queryEmbedding, factVec)
				if vecScore < 0 {
					vecScore = 0
				}
			}
		}
		imp := importance
		if imp <= 0 {
			imp = 0.5
		}
		decayedImp := imp * factRecencyDecay(updatedAt, now)
		recency := recencyBoost(updatedAt, now)
		total := l3ScoreWeightKeyword*kwScore + l3ScoreWeightVector*vecScore + l3ScoreWeightImport*decayedImp + l3ScoreWeightRecency*recency
		if query == "" && kwScore == 0 && vecScore == 0 {
			total = decayedImp + recency*0.1
		}
		var id string
		if m := map[string]any{}; json.Unmarshal(raw, &m) == nil {
			id, _ = m["id"].(string)
		}
		scored = append(scored, scoredFact{
			raw: raw, id: id, stmt: stmt, details: details, score: total,
			breakdown: recallScoreBreakdown{
				Keyword: kwScore, Vector: vecScore, Importance: decayedImp,
				Recency: recency, Total: total,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if query != "" && len(scored) > 1 {
		passages := make([]string, len(scored))
		for i := range scored {
			passages[i] = factPassage(scored[i].stmt, scored[i].details)
		}
		applyCrossEncoderRerankToFactScored(query, scored, passages)
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	out := make([]RecallDebugRow, 0, lim)
	for i := 0; i < len(scored) && len(out) < lim; i++ {
		s := scored[i]
		out = append(out, RecallDebugRow{
			Layer: "L3", ID: s.id, Statement: s.stmt, Summary: s.details,
			Scores: s.breakdown, Raw: json.RawMessage(s.raw),
		})
	}
	return out, nil
}

func scanFactRowForRecall(rows *sql.Rows) (raw []byte, stmt, details string, importance float64, updatedAt string, embedBlob []byte, err error) {
	var (
		id, stype, sid, wid, uid, tid, aid string
		snorm, fp                          string
		fkind, tags                        string
		conf, imp                          float64
		uc, hc, pfc, nfc, cc               int
		srcKind, epID, sessID, msgID, ext  string
		ver                                int
		st, sup                            string
		embSt, embModel                    string
		embDim                             int
		embNorm                            float64
		pii                                int
		redacted                           string
		ttlD                               int
		decay                              float64
		nextD, lastU, exp                  string
		meta, ca, ua, arch, del            string
	)
	if err := rows.Scan(
		&id, &stype, &sid, &wid, &uid, &tid, &aid,
		&stmt, &snorm, &fp, &details,
		&fkind, &tags,
		&conf, &imp, &uc, &hc, &pfc, &nfc, &cc,
		&srcKind, &epID, &sessID, &msgID, &ext,
		&ver, &st, &sup,
		&embSt, &embModel, &embDim, &embedBlob, &embNorm,
		&pii, &redacted,
		&ttlD, &decay, &nextD, &lastU, &exp,
		&meta, &ca, &ua, &arch, &del,
	); err != nil {
		return nil, "", "", 0, "", nil, err
	}
	importance = imp
	updatedAt = ua
	m := map[string]any{
		"id": id, "scope_type": stype, "scope_id": sid, "workspace_id": wid,
		"user_id": uid, "team_id": tid, "agent_id": aid,
		"statement": stmt, "statement_normalized": snorm, "fingerprint": fp,
		"details_markdown": details, "fact_kind": fkind, "tags_json": tags,
		"confidence": conf, "importance": imp,
		"use_count": uc, "hit_count": hc,
		"positive_feedback_count": pfc, "negative_feedback_count": nfc, "conflict_count": cc,
		"source_kind": srcKind, "source_episode_id": epID,
		"source_session_id": sessID, "source_message_id": msgID, "source_external": ext,
		"version": ver, "status": st, "superseded_by": sup,
		"embedding_status": embSt, "embedding_model": embModel, "embedding_dim": embDim,
		"embedding_norm":     embNorm,
		"pii_flag":           pii != 0,
		"redacted_statement": redacted,
		"ttl_days":           ttlD, "decay_factor": decay,
		"next_decay_at": nextD, "last_used_at": lastU, "expires_at": exp,
		"metadata_json": meta, "created_at": ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
	}
	raw, err = json.Marshal(m)
	return raw, stmt, details, importance, updatedAt, embedBlob, err
}

func factRecencyDecay(updatedAt string, now time.Time) float64 {
	updatedAt = strings.TrimSpace(updatedAt)
	if updatedAt == "" {
		return 1
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updatedAt)
	}
	if err != nil {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Pow(0.5, days/l3DecayHalfLifeDays)
}

func recencyBoost(updatedAt string, now time.Time) float64 {
	updatedAt = strings.TrimSpace(updatedAt)
	if updatedAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updatedAt)
	}
	if err != nil {
		return 0
	}
	days := now.Sub(t).Hours() / 24
	if days <= 7 {
		return 1.0
	}
	if days <= 30 {
		return 0.5
	}
	return 0.1
}
