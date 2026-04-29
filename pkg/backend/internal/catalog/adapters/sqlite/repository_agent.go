package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"arenea/backend/internal/domain"
)

// AgentRepository 承载 agents / agent_runtime_settings / agent_prompt_files 的 SQLite 实现。

type AgentRepository struct {
	db *sql.DB
}

// NewAgentRepository 从 *sql.DB 构建 Agent 仓储。
func NewAgentRepository(db *sql.DB) *AgentRepository {
	return &AgentRepository{db: db}
}

func (r *AgentRepository) ListAgents() ([]domain.Agent, error) {
	rows, err := r.db.Query(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE deleted_at = '' ORDER BY is_default DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (r *AgentRepository) SearchAgents(query domain.AgentListQuery) (domain.AgentListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 24
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	where := []string{"deleted_at = ''"}
	args := []any{}
	if q := strings.TrimSpace(query.Keyword); q != "" {
		where = append(where, "(LOWER(agent_key) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(provider) LIKE ? OR LOWER(model) LIKE ? OR LOWER(agent_description) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like, like, like)
	}
	if query.Status != "" {
		where = append(where, "status = ?")
		args = append(args, query.Status)
	}
	if query.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, query.Provider)
	}
	if query.CategoryID != "" {
		where = append(where, "category_position_id = ?")
		args = append(args, query.CategoryID)
	}

	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM agents WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.AgentListResult{}, err
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE `+whereSQL+` ORDER BY is_default DESC, updated_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.AgentListResult{}, err
	}
	defer rows.Close()

	items, err := scanAgents(rows)
	if err != nil {
		return domain.AgentListResult{}, err
	}
	return domain.AgentListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *AgentRepository) GetAgentByID(id string) (domain.Agent, error) {
	row := r.db.QueryRow(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE id = ? AND deleted_at = ''`, id)
	return scanAgent(row)
}

func (r *AgentRepository) GetAgentByKey(key string) (domain.Agent, error) {
	row := r.db.QueryRow(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE agent_key = ? AND deleted_at = ''`, key)
	return scanAgent(row)
}

func (r *AgentRepository) CreateAgent(a domain.Agent) (domain.Agent, error) {
	if a.ID == "" || a.AgentKey == "" || a.DisplayName == "" || a.Provider == "" || a.Model == "" {
		return domain.Agent{}, errors.New("missing required fields")
	}
	now := nowISO()
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Status == "" {
		a.Status = "active"
	}
	_, err := r.db.Exec(
		`INSERT INTO agents(id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.AgentKey, a.DisplayName, a.Provider, a.Model, a.Status, a.IsDefault, a.IsFavorite, a.Icon, a.AgentDescription, a.CategoryPositionID, a.SystemPromptMode, a.ContextWindow, a.BudgetMonthlyCents, a.ConfigJSON, a.CreatedAt, a.UpdatedAt, a.DeletedAt,
	)
	return a, err
}

func (r *AgentRepository) UpdateAgent(a domain.Agent) (domain.Agent, error) {
	if a.ID == "" {
		return domain.Agent{}, errors.New("id is required")
	}
	current, err := r.GetAgentByID(a.ID)
	if err != nil {
		return domain.Agent{}, err
	}
	if a.AgentKey == "" {
		a.AgentKey = current.AgentKey
	}
	if a.DisplayName == "" {
		a.DisplayName = current.DisplayName
	}
	if a.Provider == "" {
		a.Provider = current.Provider
	}
	if a.Model == "" {
		a.Model = current.Model
	}
	if a.Status == "" {
		a.Status = current.Status
	}
	a.CreatedAt = current.CreatedAt
	a.UpdatedAt = nowISO()
	_, err = r.db.Exec(
		`UPDATE agents SET display_name = ?, provider = ?, model = ?, status = ?, is_default = ?, is_favorite = ?, icon = ?, agent_description = ?, category_position_id = ?, system_prompt_mode = ?, context_window = ?, budget_monthly_cents = ?, config_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		a.DisplayName, a.Provider, a.Model, a.Status, a.IsDefault, a.IsFavorite, a.Icon, a.AgentDescription, a.CategoryPositionID, a.SystemPromptMode, a.ContextWindow, a.BudgetMonthlyCents, a.ConfigJSON, a.UpdatedAt, a.ID,
	)
	return a, err
}

func (r *AgentRepository) GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error) {
	row := r.db.QueryRow(`SELECT agent_id, self_evolve, subagents_enabled, subagents_max_concurrency, subagents_max_generation_depth, subagents_max_children_per_agent, subagents_archive_after_minutes, subagents_max_retries, subagents_model_override, tools_enabled, tools_profile, tools_tool_call_prefix, tools_allow_json, tools_deny_json, tools_concurrent_allow_json, memory_enabled, memory_max_chunk_length, memory_max_results, memory_min_score, heartbeat_enabled, heartbeat_interval_minutes, evolution_self_evolve, evolution_skill_evolve, evolution_metrics_enabled, evolution_suggestions_enabled, guardrail_max_change_per_period, guardrail_min_data_points, guardrail_rollback_on_decline_percent, l0_recent_window_turns, l0_recent_window_tokens, l0_summary_threshold, l0_summary_keep_turns, l0_truncate_strategy, l0_inject_l1, l0_inject_l3, l0_inject_l4, l0_l3_max_chunks, l0_l4_max_paths, l0_snapshot_mode, l1_enabled, l1_budget_tokens, l1_field_max_tokens, l1_history_keep_revisions, l1_default_schema_id, l1_archive_on_idle_minutes, l2_episode_enabled, l2_episode_min_importance, l2_index_enabled, l2_index_embedding_model, l2_recall_enabled, l2_recall_max, l2_retention_days, l2_archive_after_days, l3_enabled, l3_recall_top_k, l3_recall_min_score, l3_recall_scopes_json, l3_embedding_model, l3_decay_interval_hours, l3_archive_threshold, l3_max_per_recall_chars, l4_enabled, l4_graph_inject_neighbors, l4_graph_max_neighbors, l4_graph_max_hops, l4_identity_inject, l4_strategy_inject, evo_enabled, evo_auto_apply, evo_min_episodes, evo_min_negative_feedback, evo_throttle_hours, evo_proposal_ttl_days, evo_persona_max_chars, evo_system_prompt_max_appends, created_at, updated_at FROM agent_runtime_settings WHERE agent_id = ?`, agentID)
	var v domain.AgentRuntimeSettings
	err := row.Scan(
		&v.AgentID, &v.SelfEvolve, &v.SubagentsEnabled, &v.SubagentsMaxConcurrency, &v.SubagentsMaxGenerationDepth, &v.SubagentsMaxChildrenPerAgent, &v.SubagentsArchiveAfterMinutes, &v.SubagentsMaxRetries, &v.SubagentsModelOverride,
		&v.ToolsEnabled, &v.ToolsProfile, &v.ToolsToolCallPrefix, &v.ToolsAllowJSON, &v.ToolsDenyJSON, &v.ToolsConcurrentAllowJSON,
		&v.MemoryEnabled, &v.MemoryMaxChunkLength, &v.MemoryMaxResults, &v.MemoryMinScore,
		&v.HeartbeatEnabled, &v.HeartbeatIntervalMinutes,
		&v.EvolutionSelfEvolve, &v.EvolutionSkillEvolve, &v.EvolutionMetricsEnabled, &v.EvolutionSuggestionsEnabled,
		&v.GuardrailMaxChangePerPeriod, &v.GuardrailMinDataPoints, &v.GuardrailRollbackOnDeclinePercent,
		&v.L0RecentWindowTurns, &v.L0RecentWindowTokens, &v.L0SummaryThreshold, &v.L0SummaryKeepTurns, &v.L0TruncateStrategy,
		&v.L0InjectL1, &v.L0InjectL3, &v.L0InjectL4, &v.L0L3MaxChunks, &v.L0L4MaxPaths, &v.L0SnapshotMode,
		&v.L1Enabled, &v.L1BudgetTokens, &v.L1FieldMaxTokens, &v.L1HistoryKeepRevisions, &v.L1DefaultSchemaID, &v.L1ArchiveOnIdleMinutes,
		&v.L2EpisodeEnabled, &v.L2EpisodeMinImportance, &v.L2IndexEnabled, &v.L2IndexEmbeddingModel, &v.L2RecallEnabled, &v.L2RecallMax, &v.L2RetentionDays, &v.L2ArchiveAfterDays,
		&v.L3Enabled, &v.L3RecallTopK, &v.L3RecallMinScore, &v.L3RecallScopesJSON, &v.L3EmbeddingModel, &v.L3DecayIntervalHours, &v.L3ArchiveThreshold, &v.L3MaxPerRecallChars,
		&v.L4Enabled, &v.L4GraphInjectNeighbors, &v.L4GraphMaxNeighbors, &v.L4GraphMaxHops, &v.L4IdentityInject, &v.L4StrategyInject,
		&v.EvoEnabled, &v.EvoAutoApply, &v.EvoMinEpisodes, &v.EvoMinNegativeFeedback, &v.EvoThrottleHours, &v.EvoProposalTTLDays, &v.EvoPersonaMaxChars, &v.EvoSystemPromptMaxAppends,
		&v.CreatedAt, &v.UpdatedAt,
	)
	return v, err
}

func (r *AgentRepository) UpsertAgentRuntimeSettings(v domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error) {
	if v.AgentID == "" {
		return domain.AgentRuntimeSettings{}, errors.New("agent id is required")
	}
	now := nowISO()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO agent_runtime_settings(agent_id, self_evolve, subagents_enabled, subagents_max_concurrency, subagents_max_generation_depth, subagents_max_children_per_agent, subagents_archive_after_minutes, subagents_max_retries, subagents_model_override, tools_enabled, tools_profile, tools_tool_call_prefix, tools_allow_json, tools_deny_json, tools_concurrent_allow_json, memory_enabled, memory_max_chunk_length, memory_max_results, memory_min_score, heartbeat_enabled, heartbeat_interval_minutes, evolution_self_evolve, evolution_skill_evolve, evolution_metrics_enabled, evolution_suggestions_enabled, guardrail_max_change_per_period, guardrail_min_data_points, guardrail_rollback_on_decline_percent, l0_recent_window_turns, l0_recent_window_tokens, l0_summary_threshold, l0_summary_keep_turns, l0_truncate_strategy, l0_inject_l1, l0_inject_l3, l0_inject_l4, l0_l3_max_chunks, l0_l4_max_paths, l0_snapshot_mode, l1_enabled, l1_budget_tokens, l1_field_max_tokens, l1_history_keep_revisions, l1_default_schema_id, l1_archive_on_idle_minutes, l2_episode_enabled, l2_episode_min_importance, l2_index_enabled, l2_index_embedding_model, l2_recall_enabled, l2_recall_max, l2_retention_days, l2_archive_after_days, l3_enabled, l3_recall_top_k, l3_recall_min_score, l3_recall_scopes_json, l3_embedding_model, l3_decay_interval_hours, l3_archive_threshold, l3_max_per_recall_chars, l4_enabled, l4_graph_inject_neighbors, l4_graph_max_neighbors, l4_graph_max_hops, l4_identity_inject, l4_strategy_inject, evo_enabled, evo_auto_apply, evo_min_episodes, evo_min_negative_feedback, evo_throttle_hours, evo_proposal_ttl_days, evo_persona_max_chars, evo_system_prompt_max_appends, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET self_evolve = excluded.self_evolve, subagents_enabled = excluded.subagents_enabled, subagents_max_concurrency = excluded.subagents_max_concurrency, subagents_max_generation_depth = excluded.subagents_max_generation_depth, subagents_max_children_per_agent = excluded.subagents_max_children_per_agent, subagents_archive_after_minutes = excluded.subagents_archive_after_minutes, subagents_max_retries = excluded.subagents_max_retries, subagents_model_override = excluded.subagents_model_override, tools_enabled = excluded.tools_enabled, tools_profile = excluded.tools_profile, tools_tool_call_prefix = excluded.tools_tool_call_prefix, tools_allow_json = excluded.tools_allow_json, tools_deny_json = excluded.tools_deny_json, tools_concurrent_allow_json = excluded.tools_concurrent_allow_json, memory_enabled = excluded.memory_enabled, memory_max_chunk_length = excluded.memory_max_chunk_length, memory_max_results = excluded.memory_max_results, memory_min_score = excluded.memory_min_score, heartbeat_enabled = excluded.heartbeat_enabled, heartbeat_interval_minutes = excluded.heartbeat_interval_minutes, evolution_self_evolve = excluded.evolution_self_evolve, evolution_skill_evolve = excluded.evolution_skill_evolve, evolution_metrics_enabled = excluded.evolution_metrics_enabled, evolution_suggestions_enabled = excluded.evolution_suggestions_enabled, guardrail_max_change_per_period = excluded.guardrail_max_change_per_period, guardrail_min_data_points = excluded.guardrail_min_data_points, guardrail_rollback_on_decline_percent = excluded.guardrail_rollback_on_decline_percent, l0_recent_window_turns = excluded.l0_recent_window_turns, l0_recent_window_tokens = excluded.l0_recent_window_tokens, l0_summary_threshold = excluded.l0_summary_threshold, l0_summary_keep_turns = excluded.l0_summary_keep_turns, l0_truncate_strategy = excluded.l0_truncate_strategy, l0_inject_l1 = excluded.l0_inject_l1, l0_inject_l3 = excluded.l0_inject_l3, l0_inject_l4 = excluded.l0_inject_l4, l0_l3_max_chunks = excluded.l0_l3_max_chunks, l0_l4_max_paths = excluded.l0_l4_max_paths, l0_snapshot_mode = excluded.l0_snapshot_mode, l1_enabled = excluded.l1_enabled, l1_budget_tokens = excluded.l1_budget_tokens, l1_field_max_tokens = excluded.l1_field_max_tokens, l1_history_keep_revisions = excluded.l1_history_keep_revisions, l1_default_schema_id = excluded.l1_default_schema_id, l1_archive_on_idle_minutes = excluded.l1_archive_on_idle_minutes, l2_episode_enabled = excluded.l2_episode_enabled, l2_episode_min_importance = excluded.l2_episode_min_importance, l2_index_enabled = excluded.l2_index_enabled, l2_index_embedding_model = excluded.l2_index_embedding_model, l2_recall_enabled = excluded.l2_recall_enabled, l2_recall_max = excluded.l2_recall_max, l2_retention_days = excluded.l2_retention_days, l2_archive_after_days = excluded.l2_archive_after_days, l3_enabled = excluded.l3_enabled, l3_recall_top_k = excluded.l3_recall_top_k, l3_recall_min_score = excluded.l3_recall_min_score, l3_recall_scopes_json = excluded.l3_recall_scopes_json, l3_embedding_model = excluded.l3_embedding_model, l3_decay_interval_hours = excluded.l3_decay_interval_hours, l3_archive_threshold = excluded.l3_archive_threshold, l3_max_per_recall_chars = excluded.l3_max_per_recall_chars, l4_enabled = excluded.l4_enabled, l4_graph_inject_neighbors = excluded.l4_graph_inject_neighbors, l4_graph_max_neighbors = excluded.l4_graph_max_neighbors, l4_graph_max_hops = excluded.l4_graph_max_hops, l4_identity_inject = excluded.l4_identity_inject, l4_strategy_inject = excluded.l4_strategy_inject, evo_enabled = excluded.evo_enabled, evo_auto_apply = excluded.evo_auto_apply, evo_min_episodes = excluded.evo_min_episodes, evo_min_negative_feedback = excluded.evo_min_negative_feedback, evo_throttle_hours = excluded.evo_throttle_hours, evo_proposal_ttl_days = excluded.evo_proposal_ttl_days, evo_persona_max_chars = excluded.evo_persona_max_chars, evo_system_prompt_max_appends = excluded.evo_system_prompt_max_appends, updated_at = excluded.updated_at`,
		v.AgentID, v.SelfEvolve, v.SubagentsEnabled, v.SubagentsMaxConcurrency, v.SubagentsMaxGenerationDepth, v.SubagentsMaxChildrenPerAgent, v.SubagentsArchiveAfterMinutes, v.SubagentsMaxRetries, v.SubagentsModelOverride,
		v.ToolsEnabled, v.ToolsProfile, v.ToolsToolCallPrefix, normalizeJSONList(v.ToolsAllowJSON), normalizeJSONList(v.ToolsDenyJSON), normalizeJSONList(v.ToolsConcurrentAllowJSON),
		v.MemoryEnabled, v.MemoryMaxChunkLength, v.MemoryMaxResults, v.MemoryMinScore,
		v.HeartbeatEnabled, v.HeartbeatIntervalMinutes,
		v.EvolutionSelfEvolve, v.EvolutionSkillEvolve, v.EvolutionMetricsEnabled, v.EvolutionSuggestionsEnabled,
		v.GuardrailMaxChangePerPeriod, v.GuardrailMinDataPoints, v.GuardrailRollbackOnDeclinePercent,
		v.L0RecentWindowTurns, v.L0RecentWindowTokens, v.L0SummaryThreshold, v.L0SummaryKeepTurns, v.L0TruncateStrategy,
		v.L0InjectL1, v.L0InjectL3, v.L0InjectL4, v.L0L3MaxChunks, v.L0L4MaxPaths, v.L0SnapshotMode,
		v.L1Enabled, v.L1BudgetTokens, v.L1FieldMaxTokens, v.L1HistoryKeepRevisions, v.L1DefaultSchemaID, v.L1ArchiveOnIdleMinutes,
		v.L2EpisodeEnabled, v.L2EpisodeMinImportance, v.L2IndexEnabled, v.L2IndexEmbeddingModel, v.L2RecallEnabled, v.L2RecallMax, v.L2RetentionDays, v.L2ArchiveAfterDays,
		v.L3Enabled, v.L3RecallTopK, v.L3RecallMinScore, normalizeJSONList(v.L3RecallScopesJSON), v.L3EmbeddingModel, v.L3DecayIntervalHours, v.L3ArchiveThreshold, v.L3MaxPerRecallChars,
		v.L4Enabled, v.L4GraphInjectNeighbors, v.L4GraphMaxNeighbors, v.L4GraphMaxHops, v.L4IdentityInject, v.L4StrategyInject,
		v.EvoEnabled, v.EvoAutoApply, v.EvoMinEpisodes, v.EvoMinNegativeFeedback, v.EvoThrottleHours, v.EvoProposalTTLDays, v.EvoPersonaMaxChars, v.EvoSystemPromptMaxAppends,
		v.CreatedAt, v.UpdatedAt,
	)
	if err != nil {
		return domain.AgentRuntimeSettings{}, err
	}
	return r.GetAgentRuntimeSettings(v.AgentID)
}

func (r *AgentRepository) ListAgentPromptFiles(agentID string) ([]domain.AgentPromptFile, error) {
	rows, err := r.db.Query(`SELECT id, agent_id, file_name, body, sort_order, created_at, updated_at FROM agent_prompt_files WHERE agent_id = ? ORDER BY sort_order ASC, file_name ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []domain.AgentPromptFile{}
	for rows.Next() {
		var v domain.AgentPromptFile
		if err = rows.Scan(&v.ID, &v.AgentID, &v.Name, &v.Body, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		files = append(files, v)
	}
	return files, rows.Err()
}

func (r *AgentRepository) ReplaceAgentPromptFiles(agentID string, files []domain.AgentPromptFile) ([]domain.AgentPromptFile, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`DELETE FROM agent_prompt_files WHERE agent_id = ?`, agentID); err != nil {
		return nil, err
	}
	now := nowISO()
	for i, file := range files {
		if strings.TrimSpace(file.Name) == "" {
			continue
		}
		if file.ID == "" {
			file.ID = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
		}
		if file.SortOrder == 0 {
			file.SortOrder = (i + 1) * 10
		}
		if _, err = tx.Exec(`INSERT INTO agent_prompt_files(id, agent_id, file_name, body, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			file.ID, agentID, strings.TrimSpace(file.Name), file.Body, file.SortOrder, now, now); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.ListAgentPromptFiles(agentID)
}

func (r *AgentRepository) DeleteAgent(id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	_, err := r.db.Exec(`UPDATE agents SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, nowISO(), nowISO(), id)
	return err
}

func scanAgent(row rowScanner) (domain.Agent, error) {
	var v domain.Agent
	err := row.Scan(&v.ID, &v.AgentKey, &v.DisplayName, &v.Provider, &v.Model, &v.Status, &v.IsDefault, &v.IsFavorite, &v.Icon, &v.AgentDescription, &v.CategoryPositionID, &v.SystemPromptMode, &v.ContextWindow, &v.BudgetMonthlyCents, &v.ConfigJSON, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	return v, err
}

func scanAgents(rows *sql.Rows) ([]domain.Agent, error) {
	var result []domain.Agent
	for rows.Next() {
		v, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
