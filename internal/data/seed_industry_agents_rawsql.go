package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/scenario/loader"
	"aranea-agents/pkg/loggateway"
)

func SeedIndustryAgentsRawSQL(ctx context.Context, rawDB *sql.DB, scenarioDir string, lg loggateway.Logger) error {
	industries := []string{"softwaredev", "selfmedia", "finance"}
	deps := loader.Deps{ScenarioDir: scenarioDir}

	type agentEntry struct {
		agent biz.Agent
		id    string
	}

	specCache := make(map[string]*loader.IndustrySpec, len(industries))

	var allAgents []agentEntry
	var allTeams []biz.Team
	agentKeyToID := make(map[string]string)

	for _, ind := range industries {
		spec, specErr := loader.LoadIndustrySpec(scenarioDir, ind)
		if specErr != nil {
			lg.Warn("load industry spec failed", loggateway.StepID("data.seed.industry_agents.load_spec"), loggateway.Str("industry", ind), loggateway.Err(specErr))
			return fmt.Errorf("load industry spec %s: %w", ind, specErr)
		}
		specCache[ind] = spec

		for i := range spec.Agents {
			as := &spec.Agents[i]
			agent, buildErr := loader.BuildBizAgentFromSpec(ctx, deps, spec, as)
			if buildErr != nil {
				lg.Warn("build agent failed", loggateway.StepID("data.seed.industry_agents.build_agent"), loggateway.Str("industry", ind), loggateway.Str("agent_key", as.Key), loggateway.Err(buildErr))
				return fmt.Errorf("build agent %s/%s: %w", ind, as.Key, buildErr)
			}
			id := fmt.Sprintf("agent_%s", agent.AgentKey)
			agentKeyToID[agent.AgentKey] = id
			allAgents = append(allAgents, agentEntry{agent: agent, id: id})
		}
	}

	for _, ind := range industries {
		spec := specCache[ind]
		if len(spec.Teams) == 0 {
			continue
		}
		for i := range spec.Teams {
			ts := &spec.Teams[i]
			team, buildErr := loader.BuildBizTeamFromSpec(spec, ts, agentKeyToID)
			if buildErr != nil {
				lg.Warn("build team failed", loggateway.StepID("data.seed.industry_agents.build_team"), loggateway.Str("industry", ind), loggateway.Str("team_key", ts.Key), loggateway.Err(buildErr))
				return fmt.Errorf("build team %s/%s: %w", ind, ts.Key, buildErr)
			}
			allTeams = append(allTeams, team)
		}
	}

	tx, txErr := rawDB.BeginTx(ctx, nil)
	if txErr != nil {
		lg.Warn("industry agent seed begin tx failed", loggateway.StepID("data.seed.industry_agents.begin_tx"), loggateway.Err(txErr))
		return fmt.Errorf("industry agent seed begin tx: %w", txErr)
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	var exists int
	if qErr := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", SeedIndustryAgentsV1,
	).Scan(&exists); qErr != nil {
		lg.Warn("industry agent seed version check failed", loggateway.StepID("data.seed.industry_agents.version_check"), loggateway.Err(qErr))
		return fmt.Errorf("industry agent seed version check: %w", qErr)
	}
	if exists > 0 {
		committed = true
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, entry := range allAgents {
		agent := entry.agent
		id := entry.id

		rolesJSON := "[]"
		if len(agent.Roles) > 0 {
			b, marshalErr := json.Marshal(agent.Roles)
			if marshalErr != nil {
				return fmt.Errorf("marshal roles for %s: %w", agent.AgentKey, marshalErr)
			}
			rolesJSON = string(b)
		}

		if err := insertAgent(ctx, tx, id, agent, rolesJSON, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.industry_agents.insert_agent"), loggateway.Str("agent_key", agent.AgentKey), loggateway.Err(err))
			return fmt.Errorf("insert agent %s: %w", agent.AgentKey, err)
		}

		if agent.Settings != nil {
			if err := insertAgentRuntimeSettings(ctx, tx, id, agent.Settings, now); err != nil {
				lg.Warn("seed step failed", loggateway.StepID("data.seed.industry_agents.insert_settings"), loggateway.Str("agent_key", agent.AgentKey), loggateway.Err(err))
				return fmt.Errorf("insert agent_runtime_settings %s: %w", agent.AgentKey, err)
			}
		}
	}

	for _, team := range allTeams {
		teamID := fmt.Sprintf("team_%s", team.TeamKey)
		if err := insertTeam(ctx, tx, teamID, team, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.industry_agents.insert_team"), loggateway.Str("team_key", team.TeamKey), loggateway.Err(err))
			return fmt.Errorf("insert team %s: %w", team.TeamKey, err)
		}
	}

	_, execErr := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, applied_at)
		VALUES (?, ?, ?)
		ON CONFLICT(version) DO NOTHING
	`, SeedIndustryAgentsV1, "industry_agents_v1", now)
	if execErr != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.industry_agents.mark_version"), loggateway.Err(execErr))
		return fmt.Errorf("mark seed version: %w", execErr)
	}

	if err := tx.Commit(); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.industry_agents.commit"), loggateway.Err(err))
		return fmt.Errorf("industry agent seed commit: %w", err)
	}
	committed = true

	lg.Info("industry agent seed completed",
		loggateway.StepID("data.startup"), loggateway.Int("agents", len(allAgents)), loggateway.Int("teams", len(allTeams)))
	return nil
}

func insertAgent(ctx context.Context, tx *sql.Tx, id string, a biz.Agent, rolesJSON string, now string) error {
	cols := []string{
		"id", "agent_key", "display_name", "provider", "model", "status",
		"is_default", "is_favorite", "icon", "agent_description",
		"taxonomy_position_id", "system_prompt_mode", "context_window",
		"budget_monthly_cents", "config_json", "roles_json",
		"created_by", "created_at", "updated_at", "deleted_at",
		"readonly", "kind", "position_key", "agent_variant", "variant_description",
	}
	vals := []any{
		id, a.AgentKey, a.DisplayName, a.Provider, a.Model, a.Status,
		false, false, "", a.AgentDescription,
		a.PositionKey, a.SystemPromptMode, a.ContextWindow,
		0, "", rolesJSON,
		"", now, now, "",
		true, "system", a.PositionKey, a.AgentVariant, a.VariantDescription,
	}
	if len(cols) != len(vals) {
		return fmt.Errorf("agents column count %d != value count %d", len(cols), len(vals))
	}
	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}
	onConflictSets := []string{
		"display_name = excluded.display_name",
		"provider = excluded.provider",
		"model = excluded.model",
		"agent_description = excluded.agent_description",
		"taxonomy_position_id = excluded.taxonomy_position_id",
		"system_prompt_mode = excluded.system_prompt_mode",
		"context_window = excluded.context_window",
		"roles_json = excluded.roles_json",
		"position_key = excluded.position_key",
		"agent_variant = excluded.agent_variant",
		"variant_description = excluded.variant_description",
		"updated_at = excluded.updated_at",
		"readonly = excluded.readonly",
		"kind = excluded.kind",
	}
	query := fmt.Sprintf(
		"INSERT INTO agents (%s) VALUES (%s) ON CONFLICT(agent_key) DO UPDATE SET %s",
		strings.Join(cols, ", "),
		strings.Join(ph, ", "),
		strings.Join(onConflictSets, ", "),
	)
	_, err := tx.ExecContext(ctx, query, vals...)
	return err
}

func insertTeam(ctx context.Context, tx *sql.Tx, teamID string, t biz.Team, now string) error {
	cols := []string{
		"id", "team_key", "display_name", "status", "is_default",
		"definition_json", "adk_app_name", "category_industry_id",
		"created_at", "updated_at", "deleted_at",
	}
	vals := []any{
		teamID, t.TeamKey, t.DisplayName, t.Status, false,
		t.DefinitionJSON, "", t.CategoryIndustryID,
		now, now, "",
	}
	if len(cols) != len(vals) {
		return fmt.Errorf("teams column count %d != value count %d", len(cols), len(vals))
	}
	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}
	onConflictSets := []string{
		"display_name = excluded.display_name",
		"definition_json = excluded.definition_json",
		"category_industry_id = excluded.category_industry_id",
		"updated_at = excluded.updated_at",
	}
	query := fmt.Sprintf(
		"INSERT INTO teams (%s) VALUES (%s) ON CONFLICT(team_key) DO UPDATE SET %s",
		strings.Join(cols, ", "),
		strings.Join(ph, ", "),
		strings.Join(onConflictSets, ", "),
	)
	_, err := tx.ExecContext(ctx, query, vals...)
	return err
}

func insertAgentRuntimeSettings(ctx context.Context, tx *sql.Tx, agentID string, s *biz.AgentRuntimeSettings, now string) error {
	cols := []string{
		"agent_id", "self_evolve", "subagents_enabled", "subagents_max_concurrency",
		"subagents_max_generation_depth", "subagents_max_children_per_agent",
		"subagents_archive_after_minutes", "subagents_max_retries", "subagents_model_override",
		"tools_enabled", "tools_profile", "tools_tool_call_prefix",
		"tools_allow_json", "tools_deny_json", "tools_concurrent_allow_json",
		"memory_enabled", "memory_max_chunk_length", "memory_max_results", "memory_min_score",
		"heartbeat_enabled", "heartbeat_interval_minutes",
		"evolution_self_evolve", "evolution_skill_evolve", "evolution_metrics_enabled", "evolution_suggestions_enabled",
		"guardrail_max_change_per_period", "guardrail_min_data_points", "guardrail_rollback_on_decline_percent",
		"l0_recent_window_turns", "l0_recent_window_tokens", "l0_summary_threshold", "l0_summary_keep_turns",
		"l0_compress_min_gap_sec", "l0_compress_provider", "l0_compress_model",
		"memory_worker_provider", "memory_worker_model",
		"l0_truncate_strategy", "l0_inject_l1", "l0_inject_l3", "l0_inject_l4",
		"l0_l3_max_chunks", "l0_l4_max_paths", "l0_snapshot_mode",
		"l1_enabled", "l1_budget_tokens", "l1_field_max_tokens", "l1_history_keep_revisions", "l1_default_schema_id", "l1_archive_on_idle_minutes",
		"l2_episode_enabled", "l2_episode_min_importance", "l2_index_enabled", "l2_index_embedding_model",
		"l2_recall_enabled", "l2_recall_max", "l2_retention_days", "l2_archive_after_days",
		"l3_enabled", "l3_recall_top_k", "l3_recall_min_score", "l3_recall_scopes_json",
		"l3_embedding_model", "l3_decay_interval_hours", "l3_archive_threshold", "l3_max_per_recall_chars",
		"l4_enabled", "l4_graph_inject_neighbors", "l4_graph_max_neighbors", "l4_graph_max_hops",
		"l4_identity_inject", "l4_strategy_inject", "l4_decay_interval_hours", "l4_decay_overrides_json",
		"evo_enabled", "evo_auto_apply", "evo_min_episodes", "evo_min_negative_feedback",
		"evo_throttle_hours", "evo_proposal_ttl_days", "evo_persona_max_chars", "evo_system_prompt_max_appends",
		"skill_runtime_json", "intent_pass_enabled", "channel_id", "chat_id", "workspace",
		"reasoning_mode", "reasoning_level", "variables_json", "model_instructions_json",
		"context_compaction_enabled", "session_summary_enabled", "skill_load_mode", "code_executor_type",
		"planner_kind", "planner_config_json",
		"ralph_loop_max_iterations", "ralph_loop_completion_promise", "ralph_loop_verify_command",
		"ralph_loop_verify_timeout_seconds", "ralph_loop_promise_tag_open", "ralph_loop_promise_tag_close",
		"ralph_loop_verify_work_dir", "output_schema_json", "model_selector",
		"tools_retry_enabled", "tools_retry_max_attempts", "tools_retry_initial_interval_ms",
		"tools_retry_backoff_factor", "tools_retry_max_interval_ms", "tools_retry_jitter",
		"tools_parallel_enabled", "tools_streaming_enabled",
		"created_at", "updated_at",
	}
	vals := []any{
		agentID, s.SelfEvolve, s.SubagentsEnabled, s.SubagentsMaxConcurrency,
		s.SubagentsMaxGenerationDepth, s.SubagentsMaxChildrenPerAgent,
		s.SubagentsArchiveAfterMinutes, s.SubagentsMaxRetries, s.SubagentsModelOverride,
		s.ToolsEnabled, s.ToolsProfile, s.ToolsToolCallPrefix,
		s.ToolsAllowJSON, s.ToolsDenyJSON, s.ToolsConcurrentAllowJSON,
		s.MemoryEnabled, s.MemoryMaxChunkLength, s.MemoryMaxResults, s.MemoryMinScore,
		s.HeartbeatEnabled, s.HeartbeatIntervalMinutes,
		s.EvolutionSelfEvolve, s.EvolutionSkillEvolve, s.EvolutionMetricsEnabled, s.EvolutionSuggestionsEnabled,
		s.GuardrailMaxChangePerPeriod, s.GuardrailMinDataPoints, s.GuardrailRollbackOnDeclinePercent,
		s.L0RecentWindowTurns, s.L0RecentWindowTokens, s.L0SummaryThreshold, s.L0SummaryKeepTurns,
		s.L0CompressMinGapSec, s.L0CompressProvider, s.L0CompressModel,
		s.MemoryWorkerProvider, s.MemoryWorkerModel,
		s.L0TruncateStrategy, s.L0InjectL1, s.L0InjectL3, s.L0InjectL4,
		s.L0L3MaxChunks, s.L0L4MaxPaths, s.L0SnapshotMode,
		s.L1Enabled, s.L1BudgetTokens, s.L1FieldMaxTokens, s.L1HistoryKeepRevisions, s.L1DefaultSchemaID, s.L1ArchiveOnIdleMinutes,
		s.L2EpisodeEnabled, s.L2EpisodeMinImportance, s.L2IndexEnabled, s.L2IndexEmbeddingModel,
		s.L2RecallEnabled, s.L2RecallMax, s.L2RetentionDays, s.L2ArchiveAfterDays,
		s.L3Enabled, s.L3RecallTopK, s.L3RecallMinScore, s.L3RecallScopesJSON,
		s.L3EmbeddingModel, s.L3DecayIntervalHours, s.L3ArchiveThreshold, s.L3MaxPerRecallChars,
		s.L4Enabled, s.L4GraphInjectNeighbors, s.L4GraphMaxNeighbors, s.L4GraphMaxHops,
		s.L4IdentityInject, s.L4StrategyInject, s.L4DecayIntervalHours, s.L4DecayOverridesJSON,
		s.EvoEnabled, s.EvoAutoApply, s.EvoMinEpisodes, s.EvoMinNegativeFeedback,
		s.EvoThrottleHours, s.EvoProposalTTLDays, s.EvoPersonaMaxChars, s.EvoSystemPromptMaxAppends,
		s.SkillRuntimeJSON, s.IntentPassEnabled, s.ChannelID, s.ChatID, s.Workspace,
		s.ReasoningMode, s.ReasoningLevel, s.VariablesJSON, s.ModelInstructionsJSON,
		s.ContextCompactionEnabled, s.SessionSummaryEnabled, s.SkillLoadMode, s.CodeExecutorType,
		s.PlannerKind, s.PlannerConfigJSON,
		s.RalphLoopMaxIterations, s.RalphLoopCompletionPromise, s.RalphLoopVerifyCommand,
		s.RalphLoopVerifyTimeoutSeconds, s.RalphLoopPromiseTagOpen, s.RalphLoopPromiseTagClose,
		s.RalphLoopVerifyWorkDir, s.OutputSchemaJSON, s.ModelSelector,
		s.ToolsRetryEnabled, s.ToolsRetryMaxAttempts, s.ToolsRetryInitialIntervalMs,
		s.ToolsRetryBackoffFactor, s.ToolsRetryMaxIntervalMs, s.ToolsRetryJitter,
		s.ToolsParallelEnabled, s.ToolsStreamingEnabled,
		now, now,
	}
	if len(cols) != len(vals) {
		return fmt.Errorf("agent_runtime_settings column count %d != value count %d", len(cols), len(vals))
	}
	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}
	skipOnConflict := map[string]bool{
		"agent_id":   true,
		"created_at": true,
	}
	var onConflictSets []string
	for _, c := range cols {
		if skipOnConflict[c] {
			continue
		}
		onConflictSets = append(onConflictSets, fmt.Sprintf("%s = excluded.%s", c, c))
	}
	query := fmt.Sprintf(
		"INSERT INTO agent_runtime_settings (%s) VALUES (%s) ON CONFLICT(agent_id) DO UPDATE SET %s",
		strings.Join(cols, ", "),
		strings.Join(ph, ", "),
		strings.Join(onConflictSets, ", "),
	)
	_, err := tx.ExecContext(ctx, query, vals...)
	return err
}
