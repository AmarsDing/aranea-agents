package data

import (
	"strings"
)

const sqlL0Select = `SELECT id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
 FROM memory_l0_assembly_snapshots`

const sqlL0Insert = `INSERT INTO memory_l0_assembly_snapshots (
 id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const sqlL1Task = `SELECT id, session_id, run_id, team_id, agent_id,
 task_key, task_title, task_goal, status,
 schema_version, budget_tokens, used_tokens,
 parent_task_id, shared_with_json,
 started_at, ended_at, archived_at,
 metadata_json, created_at, updated_at FROM memory_l1_tasks`

const sqlL1Field = `SELECT id, task_id, session_id, agent_id,
 field_path, field_kind, visibility, pin_to_prompt, is_required,
 value_text, value_json, value_ref, preview, token_estimate,
 source, source_ref, ttl_seconds, expires_at,
 revision, last_read_at, read_count,
 metadata_json, created_at, updated_at FROM memory_l1_fields`

const sqlFactSelect = `SELECT id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
 statement, statement_normalized, fingerprint, details_markdown,
 fact_kind, tags_json,
 confidence, importance, use_count, hit_count,
 recalled_count, injected_count, cited_count,
 positive_feedback_count, negative_feedback_count, conflict_count,
 source_kind, source_episode_id, source_session_id, source_message_id, source_external,
 version, status, superseded_by,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 pii_flag, redacted_statement,
 ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
 metadata_json, quality_score, pii_types, created_at, updated_at, archived_at, deleted_at,
 valid_from, valid_until, links, keywords, tags,
 decay_score, context_note, index_attempts
 FROM memory_facts`

// sqlFactSelectSQL returns the fact row projection. Lexical recall paths omit
// embedding_blob (NULL placeholder keeps scanFactRowJSON's column count) so a
// small-scope scan cannot pull megabytes of vectors on every chat turn.
func sqlFactSelectSQL(includeBlob bool) string {
	if includeBlob {
		return sqlFactSelect
	}
	return strings.Replace(sqlFactSelect, " embedding_blob,", " NULL AS embedding_blob,", 1)
}

const sqlEpisodeSelect = `SELECT id, session_id, agent_id, episode_kind, title, outcome_summary, importance,
 consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at FROM memory_episodes`

const evolutionProposalSelect = `SELECT id, agent_id, workspace_id, proposal_kind, target_field, rationale,
 expected_impact, risk_level, status, created_at FROM agent_evolution_proposals`

const cascadeProposalSelect = `SELECT id, agent_id, workspace_id, trigger_entity_id, trigger_entity_name, trigger_attribute,
 old_value, new_value, affected_json, status, risk_level, rationale, metadata_json,
 reviewed_by, reviewed_at, expires_at, created_at, updated_at FROM memory_cascade_proposals`

const sqlEntityCols = `
 id, scope_type, scope_id, workspace_id, user_id,
 entity_type, name, name_normalized, aliases_json, description, attributes_json,
 importance, confidence, use_count, source_kind,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 status, merged_into,
 metadata_json, created_at, updated_at, archived_at, deleted_at,
 activation, activation_updated_at, source_type, valence, arousal`

const sqlRelationCols = `
 id, scope_type, scope_id, workspace_id,
 source_id, target_id, relation_type, bidirectional,
 weight, confidence, importance, use_count,
 attributes_json, evidence_json, status, source_kind,
 metadata_json, valid_from, valid_to, created_at, updated_at, archived_at, deleted_at,
 co_activation_count, last_reinforced_at, context_note`
