package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"

	"github.com/lib/pq"
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
		ChunkID         string `json:"chunk_id"`
		N               int    `json:"n,omitempty"`
		CollectionID    string `json:"collection_id,omitempty"`
		EvidenceKind    string `json:"evidence_kind,omitempty"`
		EvidenceHash    string `json:"evidence_hash,omitempty"`
		EvidenceContent string `json:"evidence_content,omitempty"`
	} `json:"chunks"`
	WorkspaceID  string   `json:"workspace_id,omitempty"`
	ViewerUserID int64    `json:"viewer_user_id,omitempty"`
	OrgScope     []string `json:"org_scope,omitempty"`
}

type graphCitationSnapshot struct {
	id, collectionID, kind, content, hash string
	refID                                 int64
	docID                                 string
}

func (s graphCitationSnapshot) key() string { return s.collectionID + "\x00" + s.id }

func parseGraphCitationID(id string) (kind string, refID int64, docID string, ok bool) {
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 || parts[2] == "" {
		return "", 0, "", false
	}
	switch parts[0] {
	case "g33e":
		kind = bizknowledge.GraphCardKindEntity
	case "g33r":
		kind = bizknowledge.GraphCardKindEdge
	default:
		return "", 0, "", false
	}
	refID, err := strconv.ParseInt(parts[1], 10, 64)
	return kind, refID, parts[2], err == nil && refID > 0
}

func (s graphCitationSnapshot) valid() bool {
	return s.collectionID != "" && s.content != "" && s.hash == bizknowledge.GraphCardSourceHash(s.content)
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
		id       string
		n        int
		snapshot *graphCitationSnapshot
	}
	type rawCandidate struct {
		turnID string
		chunks []rawChunk
		reply  string
		viewer string
	}
	type graphGroup struct {
		viewer context.Context
		refs   []graphCitationSnapshot
	}
	groups := make(map[string]*graphGroup)
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
		viewerKey := ""
		if payload.WorkspaceID != "" {
			viewerKey = fmt.Sprintf("%q/%d/%q", payload.WorkspaceID, payload.ViewerUserID, payload.OrgScope)
			if groups[viewerKey] == nil {
				// 不继承后台 worker 的 system/auth/org 值；仅恢复服务端 notice 的原调用者身份。
				viewer := workspace.WithContext(context.Background(), payload.WorkspaceID)
				viewer = auth.NewContext(viewer, &auth.Auth{UserID: payload.ViewerUserID})
				viewer = bizknowledge.WithOrgScope(viewer, payload.OrgScope)
				groups[viewerKey] = &graphGroup{viewer: viewer}
			}
		}
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
			ch := rawChunk{id: id, n: n}
			if kind, refID, docID, card := parseGraphCitationID(id); card {
				snapshot := graphCitationSnapshot{id: id, kind: kind, refID: refID, docID: docID,
					collectionID: c.CollectionID, content: c.EvidenceContent, hash: c.EvidenceHash}
				if viewerKey == "" || kind != c.EvidenceKind || !snapshot.valid() {
					continue // 旧卡片 notice 无版本/身份，不用当前卡文冒充历史版本。
				}
				ch.snapshot = &snapshot
				groups[viewerKey].refs = append(groups[viewerKey].refs, snapshot)
			}
			chunks = append(chunks, ch)
		}
		if len(chunks) == 0 {
			continue
		}
		raws = append(raws, rawCandidate{turnID: strings.TrimSpace(turnID), chunks: chunks, reply: reply, viewer: viewerKey})
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
	}
	if err := rows.Close(); err != nil {
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
			if ch.snapshot == nil {
				idSet[ch.id] = struct{}{}
			}
		}
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	contents, err := r.loadChunkContents(ctx, allIDs)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	allowedCards := make(map[string]map[string]bool, len(groups))
	for key, group := range groups {
		allowed, err := r.validateGraphCitationHosts(ctx, group.viewer, group.refs)
		if err != nil {
			return nil, err
		}
		allowedCards[key] = allowed
	}
	out := make([]bizknowledge.KnowledgeCitationCandidate, 0, len(raws))
	for _, rc := range raws {
		if rc.turnID == "" {
			continue
		}
		cand := bizknowledge.KnowledgeCitationCandidate{TurnID: rc.turnID, ReplyText: rc.reply}
		for _, ch := range rc.chunks {
			content := strings.TrimSpace(contents[ch.id])
			if ch.snapshot != nil && allowedCards[rc.viewer][ch.snapshot.key()] {
				content = ch.snapshot.content
			}
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
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
	}
	// G-32 T4（决策7）引用链档案回落：current 缺席 id 移交 chunk_archive 最新死亡
	// 化身（MAX revision_no）补内容——DropsMissingChunks 钉的演化形态：有档回落
	// 补内容、全无档仍弃；授权由调用方维持 collection 级当下口径，内容用历史。
	if len(out) < len(chunkIDs) {
		missing := make([]string, 0, len(chunkIDs)-len(out))
		for _, id := range chunkIDs {
			if _, ok := out[id]; !ok {
				missing = append(missing, id)
			}
		}
		fallback, fErr := queryChunkArchiveLatestContents(ctx, r.data.RWDB().ReadDB(ctx), missing)
		if fErr != nil {
			return nil, fErr
		}
		for id, content := range fallback {
			out[id] = content
		}
	}
	return out, nil
}

// validateGraphCitationHosts validates the current host/ref authorization while
// retaining the exact card text captured by the recall notice. It never replaces
// a historical card snapshot with the projection worker's current card version.
func (r *knowledgeCitationTraceRepo) validateGraphCitationHosts(requestCtx, viewer context.Context, refs []graphCitationSnapshot) (map[string]bool, error) {
	allowed := make(map[string]bool)
	if len(refs) == 0 {
		return allowed, nil
	}
	if err := requestCtx.Err(); err != nil {
		return nil, err
	}
	var tokens, kinds, docs, cols []string
	var ids []int64
	var keys []string
	seen := make(map[string]bool)
	for _, ref := range refs {
		key := ref.key()
		if seen[key] {
			continue
		}
		seen[key] = true
		tokens = append(tokens, strconv.Itoa(len(keys)))
		keys = append(keys, key)
		kinds = append(kinds, ref.kind)
		ids = append(ids, ref.refID)
		docs = append(docs, ref.docID)
		cols = append(cols, ref.collectionID)
	}
	args := []any{pq.Array(tokens), pq.Array(kinds), pq.Array(ids), pq.Array(docs), pq.Array(cols)}
	visible, visArgs := visibleDocIDsSubquery(viewer, "t.collection_id", len(args)+1)
	args = append(args, visArgs...)
	whereHost, whereEdge := "", ""
	if visible != "" {
		whereHost = " AND host.id IN (" + visible + ")"
		whereEdge = " AND l.doc_id IN (" + visible + ") AND l.target_doc_id IN (" + visible + ")"
	}
	whereWorkspace := ""
	if !workspace.IsSystem(viewer) {
		whereWorkspace = fmt.Sprintf(" AND (kc.workspace = $%d OR kc.workspace = '')", len(args)+1)
		args = append(args, workspace.IDFromContext(viewer))
	}
	q := fmt.Sprintf(`
WITH requested(token, kind, ref_id, doc_id, collection_id) AS (
 SELECT * FROM UNNEST($1::text[], $2::text[], $3::bigint[], $4::text[], $5::text[])
)
SELECT rq.token
FROM requested rq
JOIN knowledge_graph_embeddings t ON t.kind = rq.kind AND t.ref_id = rq.ref_id AND t.collection_id = rq.collection_id
JOIN knowledge_collections kc ON kc.id = t.collection_id
JOIN knowledge_documents host ON host.id = rq.doc_id AND host.collection_id = t.collection_id
WHERE 1 = 1%s%s
AND (
 (t.kind = 'entity' AND EXISTS (
   SELECT 1 FROM knowledge_doc_entities de WHERE de.entity_id = t.ref_id
    AND de.collection_id = t.collection_id AND de.doc_id = host.id
 ))
 OR (t.kind = 'edge' AND EXISTS (
   SELECT 1 FROM knowledge_links l WHERE l.id = t.ref_id AND l.collection_id = t.collection_id
    AND l.link_type = 'semantic' AND l.valid_to IS NULL
    AND (l.doc_id = host.id OR l.target_doc_id = host.id)%s
 ))
)`, whereWorkspace, whereHost, whereEdge)
	readCtx, cancel := context.WithTimeout(viewer, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(requestCtx, cancel)
	defer stop()
	rows, err := r.data.RWDB().ReadDB(readCtx).QueryContext(readCtx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
	}
	defer rows.Close()
	for rows.Next() {
		var token int
		if err := rows.Scan(&token); err != nil {
			return nil, entErrToBizErr(err, "KNOWLEDGE_CITATION")
		}
		if token >= 0 && token < len(keys) {
			allowed[keys[token]] = true
		}
	}
	return allowed, entErrToBizErr(rows.Err(), "KNOWLEDGE_CITATION")
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
			if _, _, _, derived := parseGraphCitationID(chunkID); derived {
				// g33 kind/ref/host is a document-attributed derived evidence key in
				// the ledger. It must never increment a fabricated raw chunk counter.
				continue
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
