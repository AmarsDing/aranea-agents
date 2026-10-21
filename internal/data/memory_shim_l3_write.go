package data

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// applyFactPIIGate scans every L3 write for PII. The default path redacts
// statement/details before fingerprinting. SkipPIIRedact (eval Add only)
// still flags PII types but keeps plaintext so benchmark evidence stays
// searchable. redacted_statement stores the original text for restore.
func applyFactPIIGate(in biz.FactUpsert) (statement, details string, pii int, redacted, piiTypesJSON string) {
	statement = strings.TrimSpace(in.Statement)
	details = strings.TrimSpace(in.DetailsMarkdown)
	pii = memBoolToInt(in.PIIFlag)
	if pii != 0 {
		if in.OriginalStatement != "" {
			redacted = in.OriginalStatement
		} else if details != "" {
			redacted = details
		}
	}
	piiTypesJSON = "[]"
	if len(in.PIITypes) > 0 {
		if b, err := json.Marshal(in.PIITypes); err == nil {
			piiTypesJSON = string(b)
		}
	}
	red := redactFactWritePII(statement, details)
	if red.piiFlag == 0 {
		return statement, details, pii, redacted, piiTypesJSON
	}
	if !in.SkipPIIRedact {
		statement, details = red.statement, red.details
	}
	pii = 1
	if redacted == "" {
		redacted = red.original
	}

	typeSet := map[string]struct{}{}
	for _, t := range in.PIITypes {
		if t = strings.TrimSpace(t); t != "" {
			typeSet[t] = struct{}{}
		}
	}
	var detected []string
	if json.Unmarshal([]byte(red.piiTypesJSON), &detected) == nil {
		for _, t := range detected {
			typeSet[t] = struct{}{}
		}
	}
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	sort.Strings(types)
	if b, err := json.Marshal(types); err == nil {
		piiTypesJSON = string(b)
	}
	return statement, details, pii, redacted, piiTypesJSON
}

func (r *l3FactRepo) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
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
	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_facts (
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
		version = memory_facts.version + 1,
		-- AMC C3 修复 #0/#4：评测注解（source_kind='agent_memory_challenge'）的行一旦被
		-- supersede（valid_until 非空），对其所在请求的重放（平台重试无 request_id 去重，
		-- 幂等仅靠指纹 upsert）绝不能起死（清空 valid_until/status）或抹掉审计元数据。
		-- 守卫以事件时序区分"重放"与"真情改回同一句"：仅当进来的语句事件时间
		-- （excluded.valid_from）不晚于存量行原事件时间（memory_facts.valid_from）时
		-- 判定为重放而保留墓碑；事件时间严格更晚的同文本改写仍照常复活该行。
		status = CASE
			WHEN memory_facts.source_kind = 'agent_memory_challenge'
				AND NULLIF(memory_facts.valid_until, '') IS NOT NULL
				AND COALESCE(NULLIF(excluded.valid_from, '')::timestamptz, '-infinity'::timestamptz)
					<= COALESCE(NULLIF(memory_facts.valid_from, '')::timestamptz, '-infinity'::timestamptz)
			THEN memory_facts.status
			ELSE excluded.status
		END,
		pii_flag = excluded.pii_flag, redacted_statement = excluded.redacted_statement,
		quality_score = excluded.quality_score, pii_types = excluded.pii_types,
		metadata_json = CASE
			WHEN memory_facts.source_kind = 'agent_memory_challenge'
				AND memory_facts.statement = excluded.statement
				AND COALESCE(NULLIF(excluded.valid_from, '')::timestamptz, '-infinity'::timestamptz)
					<= COALESCE(NULLIF(memory_facts.valid_from, '')::timestamptz, '-infinity'::timestamptz)
			THEN memory_facts.metadata_json
			ELSE excluded.metadata_json
		END,
		updated_at = excluded.updated_at,
		valid_from = COALESCE(NULLIF(memory_facts.valid_from, ''), excluded.valid_from),
		valid_until = CASE
			WHEN memory_facts.source_kind = 'agent_memory_challenge'
				AND NULLIF(memory_facts.valid_until, '') IS NOT NULL
				AND COALESCE(NULLIF(excluded.valid_from, '')::timestamptz, '-infinity'::timestamptz)
					<= COALESCE(NULLIF(memory_facts.valid_from, '')::timestamptz, '-infinity'::timestamptz)
			THEN memory_facts.valid_until
			ELSE excluded.valid_until
		END,
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

		rows, queryErr := r.data.RWDB().WriteDB(txCtx).QueryContext(txCtx,
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

// markFactEmbeddingStale marks a fact's embedding_status as 'stale' so the
// reconciler can detect and fix the inconsistency between SQLite and pgvector.
func (r *l3FactRepo) markFactEmbeddingStale(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET embedding_status = 'stale' WHERE id = ?`), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) setFactLinks(ctx context.Context, factID, linksJSON string) error {
	factID = strings.TrimSpace(factID)
	if factID == "" {
		return nil
	}
	if strings.TrimSpace(linksJSON) == "" {
		linksJSON = "[]"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET links = ?, updated_at = ? WHERE id = ?`),
		linksJSON, time.Now().UTC().Format(time.RFC3339Nano), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}
