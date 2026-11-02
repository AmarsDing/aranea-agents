package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// Governance vectors are best-effort enrichment. Reserve the rest of the
// caller's Add deadline for the atomic facts/governance database commit.
const evalGovernanceEmbedTimeout = 5 * time.Second

// evalAddChunkAtomsDefault bounds how many atoms one Add transaction
// processes before committing. AMC 长会话重放（单请求数千原子、每原子多次
// 查询）曾整体超过平台 worker 的单请求超时（约 40s）：单事务回滚后平台
// 重试又从零开始，确定性反复超时（2026-11-01 teval_e9efcf9f2a515e66
// attempt 1/2 均因此 failed）。分块提交把重试变成增量推进——已提交块
// 经发生指纹去重在下次尝试中跳过。EVAL_ADD_CHUNK_ATOMS<=0 恢复单事务。
const evalAddChunkAtomsDefault = 400

func evalAddChunkAtoms() int {
	if v := strings.TrimSpace(os.Getenv("EVAL_ADD_CHUNK_ATOMS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return evalAddChunkAtomsDefault
}

// splitEvalMessagesByAtomBudget groups whole messages into chunks whose atom
// count stays within budget (a message is never split). budget<=0 keeps the
// legacy single-chunk behavior.
func splitEvalMessagesByAtomBudget(msgs []biz.EvalMessage, budget int) [][]biz.EvalMessage {
	if len(msgs) == 0 {
		return nil
	}
	if budget <= 0 {
		return [][]biz.EvalMessage{msgs}
	}
	var chunks [][]biz.EvalMessage
	start, atoms := 0, 0
	for i, m := range msgs {
		n := len(biz.SplitEvalStatements(m.Content, biz.EvalMaxStatementAtoms))
		if i > start && atoms+n > budget {
			chunks = append(chunks, msgs[start:i])
			start, atoms = i, 0
		}
		atoms += n
	}
	return append(chunks, msgs[start:])
}

// sanitizeEvalMessagesForPG strips NUL (0x00) bytes from message content:
// PostgreSQL text columns reject them (pq 22021), and coding-track seeds
// embed raw file bytes, which otherwise fail the whole add transaction
// deterministically on every retry (2026-11-01 attempt 3, seed 149/150).
// Done once at ingest so facts, episodes, digests and FTS all stay clean,
// and retry fingerprints remain deterministic (same input, same strip).
func sanitizeEvalMessagesForPG(msgs []biz.EvalMessage) {
	for i := range msgs {
		if strings.IndexByte(msgs[i].Content, 0) >= 0 {
			msgs[i].Content = strings.ReplaceAll(msgs[i].Content, "\x00", "")
		}
	}
}

// AddMessages serializes updates for one user across replicas. The same
// transaction owns facts, supersession, links and the searchable L2 projection.
func (s *evalMemoryStore) AddMessages(ctx context.Context, userID, sessionID string, msgs []biz.EvalMessage) (int, error) {
	if strings.TrimSpace(userID) == "" {
		return 0, apierror.BadRequest("MEMORY_EVAL", "user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	sanitizeEvalMessagesForPG(msgs)
	// Provider latency must not consume the database transaction's deadline.
	// All facts and their governance changes still commit together below.
	embeddings := s.prepareEvalWriteEmbeddings(ctx, msgs)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	chunks := splitEvalMessagesByAtomBudget(msgs, evalAddChunkAtoms())
	stored := 0
	var allWritten []evalWrittenFact
	shared := &evalAddBatchState{touchedSlots: map[string]struct{}{}, batchIDs: map[string]struct{}{}}
	offset := 0
	for ci, chunk := range chunks {
		// Remap whole-request governance embeddings onto chunk-local indices.
		var chunkEmb map[int]evalMessageEmbeddings
		if len(embeddings) > 0 {
			chunkEmb = make(map[int]evalMessageEmbeddings, len(chunk))
			for i := range chunk {
				if emb, ok := embeddings[offset+i]; ok {
					chunkEmb[i] = emb
				}
			}
		}
		last := ci == len(chunks)-1
		var written, derivedWritten []evalWrittenFact
		chunkStored := 0
		err := s.facts.data.ExecInTx(ctx, func(txCtx context.Context) error {
			if _, err := s.facts.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "memoryeval:user:"+userID); err != nil {
				return err
			}
			var err error
			chunkStored, err = s.addMessages(txCtx, userID, sessionID, chunk, chunkEmb, &written, shared)
			if err != nil {
				return err
			}
			if last && shared.changed {
				// One derived rollup per request, atomic with the final
				// chunk: raw evidence and its projections still commit
				// together, and a retry re-runs the idempotent projection,
				// repairing chunks committed by earlier canceled attempts.
				return s.rollupEvalDerivedRows(txCtx, userID, sessionID, shared.touchedSlots, shared.batchIDs, &derivedWritten)
			}
			return nil
		})
		offset += len(chunk)
		stored += chunkStored
		allWritten = append(allWritten, written...)
		allWritten = append(allWritten, derivedWritten...)
		if err != nil {
			// Committed chunks persist; the platform retry resumes past them
			// via occurrence fingerprints instead of starting over.
			return stored, err
		}
	}
	s.scheduleEmbed(allWritten)
	return stored, nil
}

type evalMessageEmbeddings struct {
	prefixed map[int][]float32
	routed   map[int][]float32
}

// prepareEvalWriteEmbeddings batches mutable atoms across the whole request.
// Message and atom indices survive filtering so immutable evidence, skipped
// controls and mixed messages cannot shift vectors onto the wrong fact.
func (s *evalMemoryStore) prepareEvalWriteEmbeddings(ctx context.Context, msgs []biz.EvalMessage) map[int]evalMessageEmbeddings {
	if s.emb == nil {
		return nil
	}
	type atomRef struct{ message, atom int }
	var refs []atomRef
	var statements []string
	for i, m := range msgs {
		atoms := biz.SplitEvalStatements(m.Content, biz.EvalMaxStatementAtoms)
		if _, directControl := biz.ParseEvalMemoryDirective(m.Role, m.Content); len(atoms) == 1 && directControl {
			continue
		}
		for j, atom := range atoms {
			statement := atom
			if m.Role != "" {
				statement = m.Role + ": " + atom
			}
			if !evalGovernableKindForStore(biz.ClassifyEvalFactKind(statement)) || biz.IsHypotheticalStatement(stripEvalRolePrefix(statement)) {
				continue
			}
			refs = append(refs, atomRef{i, j})
			statements = append(statements, statement)
		}
	}
	if len(statements) == 0 {
		return nil
	}
	embedCtx, cancel := context.WithTimeout(ctx, evalGovernanceEmbedTimeout)
	defer cancel()
	prefixed, routed := s.embedEvalStatements(embedCtx, statements)
	if len(prefixed) == 0 && len(routed) == 0 {
		return nil
	}
	result := make(map[int]evalMessageEmbeddings)
	for index, ref := range refs {
		entry, ok := result[ref.message]
		if !ok {
			entry = evalMessageEmbeddings{prefixed: make(map[int][]float32), routed: make(map[int][]float32)}
		}
		entry.prefixed[ref.atom] = prefixed[index]
		entry.routed[ref.atom] = routed[index]
		result[ref.message] = entry
	}
	return result
}

func evalStableHash(parts ...string) string {
	b, _ := json.Marshal(parts) // []string contains no unsupported JSON values.
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func evalOccurrenceFingerprint(userID, sessionID string, m biz.EvalMessage, atomIndex int, statement string) string {
	return evalStableHash("amc-occurrence-v2", userID, sessionID, m.MessageID,
		normalizeEvalTimestamp(m.Timestamp), strconv.Itoa(atomIndex), statement)
}

func evalEpisodeTaskID(userID, sessionID string) string {
	return "eval:" + evalStableHash(userID, sessionID)
}

func evalStrictlyAfter(a, b string) bool {
	x, e1 := time.Parse(time.RFC3339Nano, normalizeEvalTimestamp(a))
	y, e2 := time.Parse(time.RFC3339Nano, normalizeEvalTimestamp(b))
	return e1 == nil && e2 == nil && x.After(y)
}

func evalValidityBoundary(sourceAt string) string {
	if sourceAt != "" {
		return sourceAt
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func (s *evalMemoryStore) invalidateEvalOccurrence(ctx context.Context, userID, id, sourceAt string) error {
	_, err := s.facts.data.RWDB().WriteDB(ctx).ExecContext(ctx, `UPDATE memory_facts SET valid_until=$1::text, updated_at=$2
		WHERE id=$3 AND scope_type='user' AND scope_id=$4 AND user_id=$4
		AND (valid_until='' OR NULLIF(valid_until,'')::timestamptz > $1::text::timestamptz)`, evalValidityBoundary(sourceAt), time.Now().UTC().Format(time.RFC3339Nano), id, userID)
	return err
}

func (s *evalMemoryStore) listEvalGovernableFacts(ctx context.Context, userID, statement string) ([][]byte, error) {
	key := biz.EvalGovernanceKey(statement)
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, `SELECT id,statement,fact_kind,source_session_id,valid_from,valid_until
		FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND status='active' AND deleted_at=''
		AND fact_kind IN ('user_identity','preference','user_preference','constraint','agent_instruction')
		AND ($3='' OR COALESCE(metadata_json::jsonb->>'governance_key','') IN ('',$3))
		ORDER BY (COALESCE(metadata_json::jsonb->>'governance_key','')=$3) DESC,created_at DESC,id ASC LIMIT $2`, userID, evalSupersedeLook, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var id, text, kind, session, from, until string
		if err := rows.Scan(&id, &text, &kind, &session, &from, &until); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(map[string]string{"id": id, "statement": text, "fact_kind": kind, "source_session_id": session, "valid_from": from, "valid_until": until})
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

func (s *evalMemoryStore) evalOccurrenceExists(ctx context.Context, userID, fingerprint string) (bool, error) {
	var exists bool
	err := queryRowScan(ctx, s.facts.data.RWDB().ReadDB(ctx),
		`SELECT EXISTS (SELECT 1 FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND fingerprint=$2)`, []any{userID, fingerprint}, &exists)
	return exists, err
}

func evalSourceMetadata(session string, msg, atom int, dates []string, prev []evalPrevValue, spans []biz.EvalDateSpan, sourceAt, localAt, statement string) string {
	var meta map[string]any
	if json.Unmarshal([]byte(evalMetadataJSON(session, msg, atom, dates, prev)), &meta) != nil {
		return "{}"
	}
	meta["source_timestamp"] = sourceAt
	if biz.IsEvalStructuredEvidence(stripEvalRolePrefix(statement)) {
		meta["structured"] = true
	}
	if sourceAt != "" {
		meta["source_local_timestamp"] = localAt
	}
	if key := biz.EvalGovernanceKey(statement); key != "" {
		meta["governance_key"] = key
	}
	// Code board: tag commit-shaped evidence so downstream analysis and any
	// later repo-scoped retrieval can segment it. Case kinds are deliberately
	// non-governable (supersession-exempt), so no governance_key is written.
	// EvalCaseStatementBody matches the classifier's role handling exactly, so
	// metadata tagging can never diverge from the stored fact_kind.
	if caseKind := biz.EvalCaseFactKind(biz.EvalCaseStatementBody(statement)); caseKind != "" {
		meta["case_kind"] = caseKind
		if scope := biz.EvalCaseScope(biz.EvalCaseStatementBody(statement)); scope != "" {
			meta["case_scope"] = scope
		}
	}
	if biz.IsEvalHistoricalStatement(statement) {
		meta["historical_context"] = true
	}
	if len(spans) > 0 {
		meta["event_spans"] = spans
	}
	return evalFinishMetadata(meta)
}

// evalAttachMedia records image descriptors on a caption fact. The filesystem
// path stays out of metadata: Search evidence is the caption text, and the
// descriptor only lets a visual question find that caption.
func evalAttachMedia(metaJSON string, parts []biz.EvalContentPart) string {
	if len(parts) == 0 {
		return metaJSON
	}
	var meta map[string]any
	if json.Unmarshal([]byte(metaJSON), &meta) != nil || meta == nil {
		return metaJSON
	}
	media := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p.Mime) == "" && strings.TrimSpace(p.Type) == "" {
			continue
		}
		media = append(media, map[string]any{
			"type":  p.Type,
			"index": p.Index,
			"mime":  p.Mime,
			"bytes": p.Bytes,
		})
	}
	if len(media) == 0 {
		return metaJSON
	}
	meta["media"] = media
	return evalFinishMetadata(meta)
}

func evalFinishMetadata(meta map[string]any) string {
	b, _ := json.Marshal(meta)
	return string(b)
}

// With no source timestamp, accept only dates invariant under a change of
// anchor. Relative expressions remain in the original evidence unexpanded.
func evalAbsoluteSpans(text string) []biz.EvalDateSpan {
	a := biz.ResolveEvalDateSpans(text, time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC))
	b := biz.ResolveEvalDateSpans(text, time.Date(2005, 7, 10, 0, 0, 0, 0, time.UTC))
	var out []biz.EvalDateSpan
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
				break
			}
		}
	}
	return out
}

func (s *evalMemoryStore) evalActiveWrittenMessages(ctx context.Context, userID string, written []evalWrittenFact) ([]biz.EvalMessage, error) {
	ids := make([]string, 0, len(written))
	for _, w := range written {
		ids = append(ids, w.id)
	}
	rows, err := s.facts.queryFactRowsByIDs(ctx, ids, "user", userID, userID, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	active := map[string]string{}
	for rows.Next() {
		raw, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		f := decodeEvalFactRow(raw)
		if biz.IsEvalHistoricalStatement(f.statement) {
			continue // Historical state stays in L3; current L2 must not reintroduce it.
		}
		active[f.id] = f.statement
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var msgs []biz.EvalMessage
	for _, w := range written {
		if text := active[w.id]; text != "" {
			msgs = append(msgs, biz.EvalMessage{Content: text})
		}
	}
	return msgs, nil
}
