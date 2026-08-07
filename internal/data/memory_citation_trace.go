package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// memoryCitationTraceRepo implements biz.MemoryCitationTraceReader over
// steps_v2 + memory_facts (FR-12.6: the "cited" stage of the three-stage
// counters). It loads recent memory_recalled notices (kind=notice,
// notice_type=memory_recalled), joins each notice's turn to its final
// assistant reply, and resolves the full fact statements for heuristic
// citation matching by the backfill worker.
type memoryCitationTraceRepo struct {
	data *Data
}

// Compile-time interface check.
var _ biz.MemoryCitationTraceReader = (*memoryCitationTraceRepo)(nil)

// NewMemoryCitationTraceReader creates a biz.MemoryCitationTraceReader
// backed by data. Returns nil when data is nil.
func NewMemoryCitationTraceReader(data *Data) biz.MemoryCitationTraceReader {
	if data == nil {
		return nil
	}
	return &memoryCitationTraceRepo{data: data}
}

// citationNoticePayload mirrors the memory_recalled notice payload emitted
// by the before-model inject hook (internal/agent/memory_inject.go).
type citationNoticePayload struct {
	Hits []struct {
		Layer  string `json:"layer"`
		FactID string `json:"fact_id"`
	} `json:"hits"`
}

// ListCitationCandidates returns candidates from memory_recalled notices
// created at or after since, newest first, capped at limit notices.
// Candidates with no parseable facts or no reply text are skipped.
func (r *memoryCitationTraceRepo) ListCitationCandidates(ctx context.Context, since time.Time, limit int) ([]biz.CitationCandidate, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	// Join each notice to its turn's final reply. The reply subquery picks
	// the last final reply step of the turn (a turn normally has exactly
	// one; retries may produce more).
	q := `
		SELECT n.turn_id, n.content,
			COALESCE((
				SELECT r.content FROM steps_v2 r
				WHERE r.turn_id = n.turn_id AND r.kind = 'reply' AND r.is_final = true
				ORDER BY r.seq DESC LIMIT 1
			), '') AS reply_text
		FROM steps_v2 n
		WHERE n.kind = 'notice' AND n.notice_type = 'memory_recalled' AND n.started_at >= ?
		ORDER BY n.started_at DESC
		LIMIT ?`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(q), since, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_CITATION")
	}
	defer rows.Close()

	type rawCandidate struct {
		turnID  string
		factIDs []string
		reply   string
	}
	var raws []rawCandidate
	for rows.Next() {
		var turnID, noticeContent, reply string
		if scanErr := rows.Scan(&turnID, &noticeContent, &reply); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "MEMORY_CITATION")
		}
		reply = strings.TrimSpace(reply)
		if reply == "" {
			continue
		}
		var payload citationNoticePayload
		if json.Unmarshal([]byte(noticeContent), &payload) != nil {
			continue
		}
		seen := make(map[string]struct{}, len(payload.Hits))
		var factIDs []string
		for _, h := range payload.Hits {
			if !strings.EqualFold(strings.TrimSpace(h.Layer), "L3") {
				continue
			}
			id := strings.TrimSpace(h.FactID)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			factIDs = append(factIDs, id)
		}
		if len(factIDs) == 0 {
			continue
		}
		raws = append(raws, rawCandidate{turnID: strings.TrimSpace(turnID), factIDs: factIDs, reply: reply})
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_CITATION")
	}
	if len(raws) == 0 {
		return nil, nil
	}

	// Resolve full statements for all referenced facts in one batch, then
	// assemble candidates (facts that no longer exist are dropped).
	idSet := make(map[string]struct{})
	for _, rc := range raws {
		for _, id := range rc.factIDs {
			idSet[id] = struct{}{}
		}
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	statements, err := r.loadFactStatements(ctx, allIDs)
	if err != nil {
		return nil, err
	}
	out := make([]biz.CitationCandidate, 0, len(raws))
	for _, rc := range raws {
		if rc.turnID == "" {
			continue
		}
		cand := biz.CitationCandidate{TurnID: rc.turnID, ReplyText: rc.reply}
		for _, id := range rc.factIDs {
			stmt := strings.TrimSpace(statements[id])
			if stmt == "" {
				continue
			}
			cand.Facts = append(cand.Facts, biz.CitationFactRef{FactID: id, Statement: stmt})
		}
		if len(cand.Facts) == 0 {
			continue
		}
		out = append(out, cand)
	}
	return out, nil
}

// loadFactStatements returns id → statement for the given fact IDs (active,
// non-deleted facts only).
func (r *memoryCitationTraceRepo) loadFactStatements(ctx context.Context, factIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(factIDs))
	if len(factIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(factIDs))
	args := make([]any, len(factIDs))
	for i, id := range factIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT id, statement FROM memory_facts WHERE id IN (` + strings.Join(placeholders, ",") + `) AND status = 'active' AND deleted_at = ''`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_CITATION")
	}
	defer rows.Close()
	for rows.Next() {
		var id, stmt string
		if scanErr := rows.Scan(&id, &stmt); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "MEMORY_CITATION")
		}
		out[id] = stmt
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_CITATION")
}
