package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// InvalidateFact marks a fact as superseded by setting valid_until to the
// current time (bi-temporal validity, P3-8). The row is preserved for
// historical reconstruction queries. Returns the updated row as JSON.
func (r *l3FactRepo) InvalidateFact(ctx context.Context, factID string) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET valid_until = ?, updated_at = ? WHERE id = ? AND valid_until = ''`),
			now, now, factID)
		if execErr != nil {
			return execErr
		}
		rows, queryErr := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE id = ?`), factID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			return apierror.NotFound("MEMORY", "fact row not found after invalidate")
		}
		result, execErr = scanFactRowJSON(rows)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}

// InvalidateAndUpsertFactTx atomically invalidates the old fact and upserts the
// new fact in a single transaction (P0-2 fix). If oldFactID is non-empty, the
// old fact is invalidated (valid_until set) before the new fact is upserted.
// Returns the upserted fact row as JSON.
func (r *l3FactRepo) InvalidateAndUpsertFactTx(ctx context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	statement, details, pii, redacted, piiTypesJSON := applyFactPIIGate(in)
	fp := strings.TrimSpace(in.Fingerprint)
	if fp == "" {
		fp = biz.FactFingerprint(statement, in.ScopeType, in.ScopeID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := strings.TrimSpace(in.CreatedAt)
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := strings.TrimSpace(in.UpdatedAt)
	if updatedAt == "" {
		updatedAt = now
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	tags := strings.TrimSpace(in.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	links := strings.TrimSpace(in.LinksJSON)
	if links == "" {
		links = "[]"
	}
	keywords := strings.TrimSpace(in.KeywordsJSON)
	if keywords == "" {
		keywords = "[]"
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	validFrom := strings.TrimSpace(in.ValidFrom)
	if validFrom == "" {
		validFrom = createdAt
	}
	validUntil := strings.TrimSpace(in.ValidUntil)
	contextNote := strings.TrimSpace(in.ContextNote)
	oldID := strings.TrimSpace(oldFactID)

	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())

		if oldID != "" {
			_, execErr := e.ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET valid_until = ?, updated_at = ? WHERE id = ? AND valid_until = ''`),
				now, now, oldID)
			if execErr != nil {
				return execErr
			}
		}

		_, execErr := e.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_facts (
		id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
		pii_flag, redacted_statement,
		ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
		quality_score, pii_types, metadata_json, created_at, updated_at,
		archived_at, deleted_at,
		valid_from, valid_until, links, keywords, tags,
		context_note
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
		statement = excluded.statement, details_markdown = excluded.details_markdown,
		confidence = excluded.confidence, importance = excluded.importance,
		use_count = memory_facts.use_count + excluded.use_count, hit_count = memory_facts.hit_count + excluded.hit_count,
	positive_feedback_count = memory_facts.positive_feedback_count + excluded.positive_feedback_count,
	negative_feedback_count = memory_facts.negative_feedback_count + excluded.negative_feedback_count,
	conflict_count = memory_facts.conflict_count + excluded.conflict_count,
		fact_kind = excluded.fact_kind, tags_json = excluded.tags_json,
		source_kind = excluded.source_kind, source_episode_id = excluded.source_episode_id,
		source_session_id = excluded.source_session_id, source_message_id = excluded.source_message_id,
		source_external = excluded.source_external,
		version = memory_facts.version + 1, status = excluded.status,
		pii_flag = excluded.pii_flag, redacted_statement = excluded.redacted_statement,
		quality_score = excluded.quality_score, pii_types = excluded.pii_types, metadata_json = excluded.metadata_json,
		updated_at = excluded.updated_at,
		valid_from = COALESCE(NULLIF(memory_facts.valid_from, ''), excluded.valid_from),
		valid_until = excluded.valid_until,
		links = excluded.links, keywords = excluded.keywords,
		context_note = excluded.context_note`),
			id,
			strings.TrimSpace(in.ScopeType),
			strings.TrimSpace(in.ScopeID),
			strings.TrimSpace(in.WorkspaceID),
			strings.TrimSpace(in.UserID),
			strings.TrimSpace(in.TeamID),
			strings.TrimSpace(in.AgentID),
			statement,
			strings.ToLower(statement),
			fp, details,
			strings.TrimSpace(in.FactKind), tags,
			in.Confidence, in.Importance, in.UseCount, in.HitCount,
			in.PositiveFeedbackCount, in.NegativeFeedbackCount, in.ConflictCount,
			strings.TrimSpace(in.SourceKind),
			strings.TrimSpace(in.SourceEpisodeID),
			strings.TrimSpace(in.SourceSessionID),
			strings.TrimSpace(in.SourceMessageID),
			strings.TrimSpace(in.SourceExternal),

			1, status, "",
			"pending", "", 0, nil, 0.0,
			pii, redacted,
			0, 0.98, "", "", "",
			defaultFactQualityScore, piiTypesJSON, meta, createdAt, updatedAt,
			"", "",
			validFrom, validUntil, links, keywords, tags,
			contextNote,
		)
		if execErr != nil {
			return execErr
		}

		rows, queryErr := e.QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`),
			strings.TrimSpace(in.ScopeType), strings.TrimSpace(in.ScopeID), fp)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return entErrToBizErr(err, "MEMORY")
			}
			return apierror.NotFound("MEMORY", "fact row not found after upsert")
		}
		result, execErr = scanFactRowJSON(rows)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}
