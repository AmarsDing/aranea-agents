package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	"github.com/lib/pq"
)

const evalControlSource = "agent_memory_challenge_control"

type evalMemoryPolicy struct {
	Selector  biz.EvalMemorySelector `json:"selector"`
	Mode      string                 `json:"mode"`
	SourceAt  string                 `json:"source_at"`
	AppliedAt string                 `json:"applied_at"`
}

func (s *evalMemoryStore) evalMemoryPolicies(ctx context.Context, userID string) ([]evalMemoryPolicy, error) {
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, `SELECT metadata_json FROM memory_facts
		WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND source_kind=$2 AND deleted_at='' ORDER BY updated_at,id`, userID, evalControlSource)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []evalMemoryPolicy
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var meta struct {
			Policy evalMemoryPolicy `json:"eval_policy"`
		}
		if json.Unmarshal([]byte(raw), &meta) != nil || meta.Policy.Mode == "" {
			return nil, apierror.Internal("MEMORY_EVAL", "invalid memory control metadata")
		}
		policies = append(policies, meta.Policy)
	}
	return policies, rows.Err()
}

func evalPolicyAfter(a, b evalMemoryPolicy) bool {
	if a.SourceAt != "" && b.SourceAt != "" && a.SourceAt != b.SourceAt {
		return evalStrictlyAfter(a.SourceAt, b.SourceAt)
	}
	return evalStrictlyAfter(a.AppliedAt, b.AppliedAt)
}

func evalPolicyBlocks(policies []evalMemoryPolicy, statement, sourceAt string) bool {
	var latest *evalMemoryPolicy
	for i := range policies {
		p := &policies[i]
		if !p.Selector.Matches(statement) {
			continue
		}
		if p.Mode != biz.EvalControlAllow && p.SourceAt != "" && sourceAt != "" && !evalStrictlyAfter(sourceAt, p.SourceAt) {
			return true
		}
		if p.Mode == biz.EvalControlForget {
			continue
		}
		if p.SourceAt != "" && sourceAt != "" && evalStrictlyAfter(p.SourceAt, sourceAt) {
			continue
		}
		if latest == nil || evalPolicyAfter(*p, *latest) {
			latest = p
		}
	}
	return latest != nil && latest.Mode == biz.EvalControlSuppress
}

// Control writes share Add's transaction. Plaintext commands and target values
// never enter the searchable fact or transcript projection.
func (s *evalMemoryStore) applyEvalMemoryDirective(ctx context.Context, userID, sessionID string, m biz.EvalMessage, atomIndex int, d biz.EvalMemoryDirective) (evalMemoryPolicy, bool, error) {
	identity := "control:" + d.Mode + ":" + d.Selector.Key()
	fingerprint := evalOccurrenceFingerprint(userID, sessionID, m, atomIndex, identity)
	if exists, err := s.evalOccurrenceExists(ctx, userID, fingerprint); err != nil {
		return evalMemoryPolicy{}, false, err
	} else if exists {
		return evalMemoryPolicy{}, false, nil
	}
	p := evalMemoryPolicy{Selector: d.Selector, Mode: d.Mode, SourceAt: normalizeEvalTimestamp(m.Timestamp), AppliedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if d.Mode != biz.EvalControlAllow {
		if err := s.deleteEvalSelectedFacts(ctx, userID, p); err != nil {
			return p, false, err
		}
	}
	meta, err := json.Marshal(map[string]any{"eval_policy": p, "source_timestamp": p.SourceAt})
	if err != nil {
		return p, false, err
	}
	_, err = s.facts.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType: "user", ScopeID: userID, UserID: userID, AgentID: userID,
		Statement: "memory lifecycle control", Fingerprint: fingerprint, FactKind: "constraint", Status: "active", Confidence: 1, Importance: 0.1,
		SourceKind: evalControlSource, SourceSessionID: sessionID, SourceMessageID: m.MessageID,
		CreatedAt: p.SourceAt, ValidFrom: p.SourceAt, MetadataJSON: string(meta), SkipPIIRedact: true,
	})
	return p, err == nil, err
}

func (s *evalMemoryStore) deleteEvalSelectedFacts(ctx context.Context, userID string, p evalMemoryPolicy) error {
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, `SELECT id,statement,source_session_id,
		CASE WHEN metadata_json::jsonb ? 'source_timestamp' THEN COALESCE(metadata_json::jsonb->>'source_timestamp','') ELSE valid_from END
		FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND deleted_at='' AND source_kind<>$2 AND source_kind<>$3`, userID, evalControlSource, evalDerivedSource)
	if err != nil {
		return err
	}
	var ids, sessions []string
	for rows.Next() {
		var id, text, session, source string
		if err := rows.Scan(&id, &text, &session, &source); err != nil {
			rows.Close()
			return err
		}
		if !p.Selector.Matches(text) || p.SourceAt != "" && source != "" && evalStrictlyAfter(source, p.SourceAt) {
			continue
		}
		ids = append(ids, id)
		if session != "" {
			sessions = append(sessions, session)
		}
	}
	rowErr := rows.Err()
	rows.Close() // Release the transaction cursor before any mutation.
	if rowErr != nil {
		return rowErr
	}
	if len(ids) > 0 {
		if _, err := s.facts.data.RWDB().WriteDB(ctx).ExecContext(ctx, `UPDATE memory_facts SET deleted_at=$1,updated_at=$1
			WHERE scope_type='user' AND scope_id=$2 AND user_id=$2 AND id=ANY($3::text[])`, p.AppliedAt, userID, pq.Array(ids)); err != nil {
			return err
		}
	}
	// A compressed row may no longer contain the selector's literal wording.
	// Drop every derived generation and rebuild from surviving source evidence.
	if _, err := s.facts.data.RWDB().WriteDB(ctx).ExecContext(ctx, `UPDATE memory_facts SET deleted_at=$1,updated_at=$1
		WHERE scope_type='user' AND scope_id=$2 AND user_id=$2 AND source_kind=$3 AND deleted_at=''`, p.AppliedAt, userID, evalDerivedSource); err != nil {
		return err
	}
	if err := s.rebuildEvalNumericDigests(ctx, userID); err != nil {
		return err
	}
	if p.Selector.All && p.SourceAt == "" {
		_, err = s.facts.data.RWDB().WriteDB(ctx).ExecContext(ctx, `UPDATE memory_episodes SET outcome='',outcome_summary='',goal='',updated_at=$1 WHERE agent_id=$2 AND deleted_at=''`, p.AppliedAt, userID)
		return err
	}
	return s.rebuildEvalEpisodes(ctx, userID, uniqueNonEmpty(sessions, 0), p.AppliedAt)
}

func (s *evalMemoryStore) rebuildEvalEpisodes(ctx context.Context, userID string, sessions []string, at string) error {
	if len(sessions) == 0 {
		return nil
	}
	_, err := s.facts.data.RWDB().WriteDB(ctx).ExecContext(ctx, `UPDATE memory_episodes SET outcome='',outcome_summary='',goal='',updated_at=$1
		WHERE agent_id=$2 AND session_id=ANY($3::text[]) AND episode_kind='eval_session' AND deleted_at=''`, at, userID, pq.Array(sessions))
	if err != nil {
		return err
	}
	_, err = s.facts.data.RWDB().WriteDB(ctx).ExecContext(ctx, `WITH remaining AS (
		SELECT source_session_id,LEFT(string_agg(statement,E'\n' ORDER BY created_at,source_message_id,COALESCE((metadata_json::jsonb->>'atom_index')::int,0)),32768) AS body
		FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND status='active' AND deleted_at='' AND valid_until=''
		AND source_kind<>$4 AND COALESCE(metadata_json::jsonb->>'historical_context','false')<>'true' AND source_session_id=ANY($2::text[]) GROUP BY source_session_id)
		UPDATE memory_episodes ep SET outcome=r.body,outcome_summary=r.body,goal=LEFT(r.body,200),updated_at=$3
		FROM remaining r WHERE ep.agent_id=$1 AND ep.session_id=r.source_session_id AND ep.episode_kind='eval_session' AND ep.deleted_at=''`, userID, pq.Array(sessions), at, evalControlSource)
	return err
}

func evalEmptySessionEpisode(raw []byte) bool {
	return strings.TrimSpace(factJSONString(raw, "outcome_summary")) == "" && factJSONString(raw, "episode_kind") == "eval_session"
}
