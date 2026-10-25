package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// AddMessages serializes updates for one user across replicas. The same
// transaction owns facts, supersession, links and the searchable L2 projection.
func (s *evalMemoryStore) AddMessages(ctx context.Context, userID, sessionID string, msgs []biz.EvalMessage) (int, error) {
	if strings.TrimSpace(userID) == "" {
		return 0, apierror.BadRequest("MEMORY_EVAL", "user_id is required")
	}
	var written []evalWrittenFact
	stored := 0
	err := s.facts.data.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := s.facts.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "memoryeval:user:"+userID); err != nil {
			return err
		}
		var err error
		stored, err = s.addMessages(txCtx, userID, sessionID, msgs, &written)
		return err
	})
	if err != nil {
		return 0, err
	}
	s.scheduleEmbed(written)
	return stored, nil
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
