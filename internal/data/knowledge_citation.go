package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── 29-token P2-2: knowledge chunk citation tracking（cited 回采） ──────────
//
// Knowledge-side counterpart of memory_citation_trace.go + l3FactRepo.
// RecordFactCitations. The knowledge_search / knowledge_reflect tools emit a
// knowledge_recalled notice per call carrying the returned chunk IDs
// (internal/tools/knowledge/tool.go); the citation backfill worker
// (cronrunner/jobs) joins each notice to its turn's final assistant reply and
// applies the same heuristics as the memory side. Recording is idempotent via
// the knowledge_chunk_citations dedup ledger (migration 20261215).
//
// Knowledge chunks have no recalled/injected counters: only the cited stage is
// tracked. Recall notices come from two paths (P1, 2026-08-15): knowledge_search /
// knowledge_reflect tool calls, and the first-turn ## Retrieved Knowledge
// pre-retrieval injection (internal/agent/knowledge_inject.go); the (chunk, turn)
// dedup ledger absorbs any overlap between the two.

// knowledgeCitationNoticePayload mirrors the knowledge_recalled notice payload
// emitted by the knowledge tools after each search/reflect call. The notice
// type string itself is defined canonically as
// bizknowledge.KnowledgeRecalledNoticeType.
type knowledgeCitationNoticePayload struct {
	Chunks []struct {
		ChunkID string `json:"chunk_id"`
		N       int    `json:"n,omitempty"`
	} `json:"chunks"`
}

// knowledgeCitationTraceRepo implements
// bizknowledge.KnowledgeCitationTraceReader over steps_v2 + knowledge_chunks.
type knowledgeCitationTraceRepo struct {
	data *Data
}

// Compile-time interface check.
var _ bizknowledge.KnowledgeCitationTraceReader = (*knowledgeCitationTraceRepo)(nil)

// NewKnowledgeCitationTraceReader creates a
// bizknowledge.KnowledgeCitationTraceReader backed by data.
// Returns nil when data is nil.
func NewKnowledgeCitationTraceReader(data *Data) bizknowledge.KnowledgeCitationTraceReader {
	if data == nil {
		return nil
	}
	return &knowledgeCitationTraceRepo{data: data}
}

// ListKnowledgeCitationCandidates returns candidates from knowledge_recalled
// notices created at or after since, newest first, capped at limit notices.
// Candidates with no parseable chunks or no reply text are skipped.
func (r *knowledgeCitationTraceRepo) ListKnowledgeCitationCandidates(ctx context.Context, since time.Time, limit int) ([]bizknowledge.KnowledgeCitationCandidate, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	// Join each notice to its turn's final reply (same shape as the memory
	// citation trace: last final reply step of the turn).
	q := `
		SELECT n.turn_id, n.content,
			COALESCE((
				SELECT r.content FROM steps_v2 r
				WHERE r.turn_id = n.turn_id AND r.kind = 'reply' AND r.is_final = true
				ORDER BY r.seq DESC LIMIT 1
			), '') AS reply_text
		FROM steps_v2 n
		WHERE n.kind = 'notice' AND n.notice_type = ? AND n.started_at >= ?
		ORDER BY n.started_at DESC
		LIMIT ?`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(q), bizknowledge.KnowledgeRecalledNoticeType, since, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
	}
	defer rows.Close()

	type rawChunk struct {
		id string
		n  int
	}
	type rawCandidate struct {
		turnID string
		chunks []rawChunk
		reply  string
	}
	var raws []rawCandidate
	for rows.Next() {
		var turnID, noticeContent, reply string
		if scanErr := rows.Scan(&turnID, &noticeContent, &reply); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "KNOWLEDGE_CITATION")
		}
		reply = strings.TrimSpace(reply)
		if reply == "" {
			continue
		}
		var payload knowledgeCitationNoticePayload
		if json.Unmarshal([]byte(noticeContent), &payload) != nil {
			continue
		}
		seen := make(map[string]struct{}, len(payload.Chunks))
		var chunks []rawChunk
		for _, c := range payload.Chunks {
			id := strings.TrimSpace(c.ChunkID)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			n := c.N
			if n < 0 {
				n = 0
			}
			chunks = append(chunks, rawChunk{id: id, n: n})
		}
		if len(chunks) == 0 {
			continue
		}
		raws = append(raws, rawCandidate{turnID: strings.TrimSpace(turnID), chunks: chunks, reply: reply})
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
	}
	if len(raws) == 0 {
		return nil, nil
	}

	// Resolve full contents for all referenced chunks in one batch, then
	// assemble candidates (chunks that no longer exist are dropped).
	idSet := make(map[string]struct{})
	for _, rc := range raws {
		for _, ch := range rc.chunks {
			idSet[ch.id] = struct{}{}
		}
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	contents, err := r.loadChunkContents(ctx, allIDs)
	if err != nil {
		return nil, err
	}
	out := make([]bizknowledge.KnowledgeCitationCandidate, 0, len(raws))
	for _, rc := range raws {
		if rc.turnID == "" {
			continue
		}
		cand := bizknowledge.KnowledgeCitationCandidate{TurnID: rc.turnID, ReplyText: rc.reply}
		for _, ch := range rc.chunks {
			content := strings.TrimSpace(contents[ch.id])
			if content == "" {
				continue
			}
			cand.Chunks = append(cand.Chunks, bizknowledge.CitationChunkRef{ChunkID: ch.id, Content: content, N: ch.n})
		}
		if len(cand.Chunks) == 0 {
			continue
		}
		out = append(out, cand)
	}
	return out, nil
}

// loadChunkContents returns id → content for the given chunk IDs.
func (r *knowledgeCitationTraceRepo) loadChunkContents(ctx context.Context, chunkIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(chunkIDs))
	if len(chunkIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(chunkIDs))
	args := make([]any, len(chunkIDs))
	for i, id := range chunkIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT id, content FROM knowledge_chunks WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
	}
	defer rows.Close()
	for rows.Next() {
		var id, content string
		if scanErr := rows.Scan(&id, &content); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "KNOWLEDGE_CITATION")
		}
		out[id] = content
	}
	return out, entErrToBizErr(rows.Err(), "KNOWLEDGE_CITATION")
}

// knowledgeChunkCitationRepo implements
// bizknowledge.KnowledgeChunkCitationRecorder over knowledge_chunk_citations
// + knowledge_chunks.cited_count.
type knowledgeChunkCitationRepo struct {
	data *Data
}

// Compile-time interface check.
var _ bizknowledge.KnowledgeChunkCitationRecorder = (*knowledgeChunkCitationRepo)(nil)

// NewKnowledgeChunkCitationRecorder creates a
// bizknowledge.KnowledgeChunkCitationRecorder backed by data.
// Returns nil when data is nil.
func NewKnowledgeChunkCitationRecorder(data *Data) bizknowledge.KnowledgeChunkCitationRecorder {
	if data == nil {
		return nil
	}
	return &knowledgeChunkCitationRepo{data: data}
}

// RecordChunkCitations records (chunk, turn) citations into the dedup ledger
// and increments cited_count for first-seen pairs only. Knowledge tables are
// Postgres-only (pgvector), so ON CONFLICT needs no dialect branch.
func (r *knowledgeChunkCitationRepo) RecordChunkCitations(ctx context.Context, citations []bizknowledge.ChunkCitation) error {
	if r == nil || r.data == nil || len(citations) == 0 {
		return nil
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC()
		for _, c := range citations {
			chunkID := strings.TrimSpace(c.ChunkID)
			turnID := strings.TrimSpace(c.TurnID)
			if chunkID == "" || turnID == "" {
				continue
			}
			res, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`INSERT INTO knowledge_chunk_citations (chunk_id, turn_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`),
				chunkID, turnID, now)
			if execErr != nil {
				return entErrToBizErr(execErr, "KNOWLEDGE_CITATION")
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				continue // already recorded — idempotent skip
			}
			if _, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE knowledge_chunks SET cited_count = cited_count + 1 WHERE id = ?`),
				chunkID); execErr != nil {
				return entErrToBizErr(execErr, "KNOWLEDGE_CITATION")
			}
		}
		return nil
	})
}
