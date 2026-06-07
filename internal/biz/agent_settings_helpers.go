package biz

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// pgoDefaultFilesV2 reads the PGO_DEFAULT_FILES_V2 env-flag without importing
// internal/conf (which would create a circular dependency). biz must remain
// dependency-free of conf. PGO-1-BIZ-01.
//
// Intentional: this duplicates conf.PGODefaultFilesV2() to avoid the
// biz → conf import cycle. Both read the same env var; keep them in sync.
func pgoDefaultFilesV2() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("PGO_DEFAULT_FILES_V2")))
	// Default to V2 (rich Chinese content); set "0" or "false" to use legacy stubs.
	if v == "" {
		return true
	}
	return v == "1" || v == "true" || v == "yes"
}

// withSettingDefaults fills missing numeric/string fields in AgentRuntimeSettings without turning unset booleans into forced defaults.
func withSettingDefaults(v AgentRuntimeSettings) AgentRuntimeSettings {
	d := DefaultAgentRuntimeSettings()
	if v.SelfEvolve && !v.EvolutionSelfEvolve {
		v.EvolutionSelfEvolve = v.SelfEvolve
	}
	defaultInt(&v.SubagentsMaxConcurrency, d.SubagentsMaxConcurrency)
	defaultInt(&v.SubagentsMaxGenerationDepth, d.SubagentsMaxGenerationDepth)
	defaultInt(&v.SubagentsMaxChildrenPerAgent, d.SubagentsMaxChildrenPerAgent)
	defaultInt(&v.SubagentsArchiveAfterMinutes, d.SubagentsArchiveAfterMinutes)
	defaultInt(&v.SubagentsMaxRetries, d.SubagentsMaxRetries)
	defaultString(&v.ToolsProfile, d.ToolsProfile)
	defaultString(&v.ToolsAllowJSON, d.ToolsAllowJSON)
	defaultString(&v.ToolsDenyJSON, d.ToolsDenyJSON)
	defaultString(&v.ToolsConcurrentAllowJSON, d.ToolsConcurrentAllowJSON)
	defaultInt(&v.MemoryMaxChunkLength, d.MemoryMaxChunkLength)
	defaultInt(&v.MemoryMaxResults, d.MemoryMaxResults)
	defaultFloat(&v.MemoryMinScore, d.MemoryMinScore)
	defaultInt(&v.HeartbeatIntervalMinutes, d.HeartbeatIntervalMinutes)
	defaultFloat(&v.GuardrailMaxChangePerPeriod, d.GuardrailMaxChangePerPeriod)
	defaultInt(&v.GuardrailMinDataPoints, d.GuardrailMinDataPoints)
	defaultInt(&v.GuardrailRollbackOnDeclinePercent, d.GuardrailRollbackOnDeclinePercent)
	defaultInt(&v.L0RecentWindowTurns, d.L0RecentWindowTurns)
	defaultFloat(&v.L0SummaryThreshold, d.L0SummaryThreshold)
	defaultInt(&v.L0SummaryKeepTurns, d.L0SummaryKeepTurns)
	defaultInt(&v.L0CompressMinGapSec, d.L0CompressMinGapSec)
	defaultString(&v.L0TruncateStrategy, d.L0TruncateStrategy)
	defaultInt(&v.L0L3MaxChunks, d.L0L3MaxChunks)
	defaultInt(&v.L0L4MaxPaths, d.L0L4MaxPaths)
	defaultString(&v.L0SnapshotMode, d.L0SnapshotMode)
	defaultInt(&v.L1BudgetTokens, d.L1BudgetTokens)
	defaultInt(&v.L1FieldMaxTokens, d.L1FieldMaxTokens)
	defaultInt(&v.L1HistoryKeepRevisions, d.L1HistoryKeepRevisions)
	defaultInt(&v.L1ArchiveOnIdleMinutes, d.L1ArchiveOnIdleMinutes)
	defaultFloat(&v.L2EpisodeMinImportance, d.L2EpisodeMinImportance)
	defaultInt(&v.L2RecallMax, d.L2RecallMax)
	defaultInt(&v.L2RetentionDays, d.L2RetentionDays)
	defaultInt(&v.L2ArchiveAfterDays, d.L2ArchiveAfterDays)
	defaultInt(&v.L3RecallTopK, d.L3RecallTopK)
	defaultFloat(&v.L3RecallMinScore, d.L3RecallMinScore)
	defaultString(&v.L3RecallScopesJSON, d.L3RecallScopesJSON)
	defaultInt(&v.L3DecayIntervalHours, d.L3DecayIntervalHours)
	defaultFloat(&v.L3ArchiveThreshold, d.L3ArchiveThreshold)
	defaultInt(&v.L3MaxPerRecallChars, d.L3MaxPerRecallChars)
	defaultInt(&v.L4GraphMaxNeighbors, d.L4GraphMaxNeighbors)
	defaultInt(&v.L4GraphMaxHops, d.L4GraphMaxHops)
	defaultInt(&v.EvoMinEpisodes, d.EvoMinEpisodes)
	defaultInt(&v.EvoMinNegativeFeedback, d.EvoMinNegativeFeedback)
	defaultInt(&v.EvoThrottleHours, d.EvoThrottleHours)
	defaultInt(&v.EvoProposalTTLDays, d.EvoProposalTTLDays)
	defaultInt(&v.EvoPersonaMaxChars, d.EvoPersonaMaxChars)
	defaultInt(&v.EvoSystemPromptMaxAppends, d.EvoSystemPromptMaxAppends)
	defaultString(&v.SkillRuntimeJSON, d.SkillRuntimeJSON)
	defaultString(&v.CodeExecutorType, d.CodeExecutorType)
	defaultString(&v.PlannerConfigJSON, d.PlannerConfigJSON)
	defaultInt(&v.ToolsRetryMaxAttempts, d.ToolsRetryMaxAttempts)
	defaultInt(&v.ToolsRetryInitialIntervalMs, d.ToolsRetryInitialIntervalMs)
	defaultFloat(&v.ToolsRetryBackoffFactor, d.ToolsRetryBackoffFactor)
	defaultInt(&v.ToolsRetryMaxIntervalMs, d.ToolsRetryMaxIntervalMs)
	return v
}

func defaultInt(target *int, fallback int) {
	if *target == 0 {
		*target = fallback
	}
}

func defaultFloat(target *float64, fallback float64) {
	if *target == 0 {
		*target = fallback
	}
}

func defaultString(target *string, fallback string) {
	if strings.TrimSpace(*target) == "" {
		*target = fallback
	}
}

func settingsFromLegacyConfig(raw string) AgentRuntimeSettings {
	settings := DefaultAgentRuntimeSettings()
	var parsed struct {
		SelfEvolve *bool `json:"self_evolve"`
		Subagents  struct {
			Enabled             *bool  `json:"enabled"`
			MaxConcurrency      int    `json:"max_concurrency"`
			MaxGenerationDepth  int    `json:"max_generation_depth"`
			MaxChildrenPerAgent int    `json:"max_children_per_agent"`
			ArchiveAfterMinutes int    `json:"archive_after_minutes"`
			MaxRetries          int    `json:"max_retries"`
			ModelOverride       string `json:"model_override"`
		} `json:"subagents"`
		Tools struct {
			Enabled         *bool    `json:"enabled"`
			Profile         string   `json:"profile"`
			ToolCallPrefix  string   `json:"tool_call_prefix"`
			Allow           []string `json:"allow"`
			Deny            []string `json:"deny"`
			ConcurrentAllow []string `json:"concurrent_allow"`
		} `json:"tools"`
		Memory struct {
			Enabled        *bool   `json:"enabled"`
			MaxChunkLength int     `json:"max_chunk_length"`
			MaxResults     int     `json:"max_results"`
			MinScore       float64 `json:"min_score"`
		} `json:"memory"`
		Heartbeat struct {
			Enabled         *bool `json:"enabled"`
			IntervalMinutes int   `json:"interval_minutes"`
		} `json:"heartbeat"`
		Evolution struct {
			SelfEvolve                  *bool `json:"self_evolve"`
			SkillEvolve                 *bool `json:"skill_evolve"`
			EvolutionMetricsEnabled     *bool `json:"evolution_metrics_enabled"`
			EvolutionSuggestionsEnabled *bool `json:"evolution_suggestions_enabled"`
		} `json:"evolution"`
		EvolutionGuardrails struct {
			MaxChangePerPeriod       float64 `json:"max_change_per_period"`
			MinDataPoints            int     `json:"min_data_points"`
			RollbackOnDeclinePercent int     `json:"rollback_on_decline_percent"`
		} `json:"evolution_guardrails"`
		IntentPass struct {
			Enabled *bool `json:"enabled"`
		} `json:"intent_pass"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return settings
	}
	if parsed.SelfEvolve != nil {
		settings.SelfEvolve = *parsed.SelfEvolve
	}
	if parsed.Subagents.Enabled != nil {
		settings.SubagentsEnabled = *parsed.Subagents.Enabled
	}
	if parsed.Subagents.MaxConcurrency > 0 {
		settings.SubagentsMaxConcurrency = parsed.Subagents.MaxConcurrency
	}
	if parsed.Subagents.MaxGenerationDepth > 0 {
		settings.SubagentsMaxGenerationDepth = parsed.Subagents.MaxGenerationDepth
	}
	if parsed.Subagents.MaxChildrenPerAgent > 0 {
		settings.SubagentsMaxChildrenPerAgent = parsed.Subagents.MaxChildrenPerAgent
	}
	if parsed.Subagents.ArchiveAfterMinutes > 0 {
		settings.SubagentsArchiveAfterMinutes = parsed.Subagents.ArchiveAfterMinutes
	}
	if parsed.Subagents.MaxRetries > 0 {
		settings.SubagentsMaxRetries = parsed.Subagents.MaxRetries
	}
	settings.SubagentsModelOverride = parsed.Subagents.ModelOverride
	if parsed.Tools.Enabled != nil {
		settings.ToolsEnabled = *parsed.Tools.Enabled
	}
	if parsed.Tools.Profile != "" {
		settings.ToolsProfile = parsed.Tools.Profile
	}
	settings.ToolsToolCallPrefix = parsed.Tools.ToolCallPrefix
	settings.ToolsAllowJSON = mustJSON(parsed.Tools.Allow)
	settings.ToolsDenyJSON = mustJSON(parsed.Tools.Deny)
	settings.ToolsConcurrentAllowJSON = mustJSON(parsed.Tools.ConcurrentAllow)
	if parsed.Memory.Enabled != nil {
		settings.MemoryEnabled = *parsed.Memory.Enabled
	}
	if parsed.Memory.MaxChunkLength > 0 {
		settings.MemoryMaxChunkLength = parsed.Memory.MaxChunkLength
	}
	if parsed.Memory.MaxResults > 0 {
		settings.MemoryMaxResults = parsed.Memory.MaxResults
	}
	if parsed.Memory.MinScore > 0 {
		settings.MemoryMinScore = parsed.Memory.MinScore
	}
	if parsed.Heartbeat.Enabled != nil {
		settings.HeartbeatEnabled = *parsed.Heartbeat.Enabled
	}
	if parsed.Heartbeat.IntervalMinutes > 0 {
		settings.HeartbeatIntervalMinutes = parsed.Heartbeat.IntervalMinutes
	}
	if parsed.Evolution.SelfEvolve != nil {
		settings.EvolutionSelfEvolve = *parsed.Evolution.SelfEvolve
	}
	if parsed.Evolution.SkillEvolve != nil {
		settings.EvolutionSkillEvolve = *parsed.Evolution.SkillEvolve
	}
	if parsed.Evolution.EvolutionMetricsEnabled != nil {
		settings.EvolutionMetricsEnabled = *parsed.Evolution.EvolutionMetricsEnabled
	}
	if parsed.Evolution.EvolutionSuggestionsEnabled != nil {
		settings.EvolutionSuggestionsEnabled = *parsed.Evolution.EvolutionSuggestionsEnabled
	}
	if parsed.EvolutionGuardrails.MaxChangePerPeriod > 0 {
		settings.GuardrailMaxChangePerPeriod = parsed.EvolutionGuardrails.MaxChangePerPeriod
	}
	if parsed.EvolutionGuardrails.MinDataPoints > 0 {
		settings.GuardrailMinDataPoints = parsed.EvolutionGuardrails.MinDataPoints
	}
	if parsed.EvolutionGuardrails.RollbackOnDeclinePercent > 0 {
		settings.GuardrailRollbackOnDeclinePercent = parsed.EvolutionGuardrails.RollbackOnDeclinePercent
	}
	if parsed.IntentPass.Enabled != nil {
		settings.IntentPassEnabled = *parsed.IntentPass.Enabled
	}
	return settings
}

func filesFromLegacyConfig(raw string) []AgentPromptFile {
	var parsed struct {
		Files []AgentPromptFile `json:"files"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return nil
	}
	return withFileDefaults(parsed.Files)
}

// defaultPromptFiles returns the V2 default set (5 core files) when
// PGO_DEFAULT_FILES_V2 is enabled, otherwise returns the legacy 9-file set
// for backward compatibility. PGO-1-BIZ-01.
func defaultPromptFiles() []AgentPromptFile {
	if pgoDefaultFilesV2() {
		return defaultPromptFilesV2()
	}
	return defaultPromptFilesLegacy()
}

// defaultPromptFilesV2 is the PGO-1 canonical 5-file set.
// SOUL/USER/USER_PREDEFINED are removed; HEARTBEAT moves to Settings.
// USER_CONTEXT.md is available as an optional file via OptionalPromptFileTemplates.
func defaultPromptFilesV2() []AgentPromptFile {
	return []AgentPromptFile{
		{
			Name:      "AGENTS_CORE.md",
			Body:      "# AGENTS_CORE\n\n## 语言跟随\n- 始终使用用户使用的语言进行回复和操作\n- 如果用户切换语言，立即跟随切换\n\n## 文件操作约束\n- 所有变更必须通过文件工具（read_file / save_file）执行，禁止绕过\n- 修改前先读取当前内容，避免覆盖他人更改\n- 保存时保留原有格式和缩进\n\n## 交互原则\n- 优先理解用户意图，再选择行动\n- 不确定时主动询问，而非猜测\n- 操作完成后简要说明结果",
			SortOrder: 10,
		},
		{
			Name:      "AGENTS_TASK.md",
			Body:      "# AGENTS_TASK\n\n## 任务执行\n- 执行企业任务时保持可追踪、可恢复\n- 每个关键步骤记录进度，便于中断后恢复\n- 任务完成后输出结构化摘要\n\n## 记忆使用\n- 利用记忆系统存储重要上下文，避免重复询问\n- 敏感信息（密钥、密码）不写入记忆\n- 定期清理过时记忆条目\n\n## 隐私约定\n- 不主动收集与任务无关的个人信息\n- 脱敏处理后再存储用户数据\n- 遵守数据保留策略，到期自动清理",
			SortOrder: 20,
		},
		{
			Name:      "IDENTITY.md",
			Body:      "# IDENTITY\n\n## Persona\n保持专业、清晰、克制。\n\n## 角色定位\n（请描述 Agent 的核心角色和职责）\n\n## 沟通风格\n- 简洁明了，避免冗余\n- 技术内容使用准确术语\n- 面向非技术用户时自动简化表达",
			SortOrder: 30,
		},
		{
			Name:      "CAPABILITIES.md",
			Body:      "# CAPABILITIES\n\n## 核心能力\n- 信息分析与推理\n- 任务规划与执行\n- 结果复盘与优化\n\n## 工具使用\n- 文件读写：通过 read_file / save_file 操作\n- 代码执行：通过沙箱环境运行代码\n- 网络搜索：获取实时信息辅助决策\n\n## 能力边界\n- 无法直接访问用户本地文件系统（需通过工具）\n- 无法执行需要物理交互的操作\n- 无法访问未授权的内部系统",
			SortOrder: 40,
		},
		{
			Name:      "RULE.md",
			Body:      "# RULE\n\n## 禁止行为\n- 不得越权操作（超出当前权限范围的系统操作）\n- 不得删除未备份的重要数据\n- 不得绕过安全检查或审计机制\n\n## 合规要求\n- 遵守组织安全策略\n- 敏感操作需二次确认\n- 所有变更留有审计日志\n\n## 降级策略\n- 遇到不确定的操作时，选择更保守的方案\n- 服务不可用时，提供替代建议而非报错",
			SortOrder: 50,
		},
	}
}

// defaultPromptFilesLegacy is the pre-PGO 9-file set, preserved for
// backward compatibility when PGO_DEFAULT_FILES_V2=false.
func defaultPromptFilesLegacy() []AgentPromptFile {
	return []AgentPromptFile{
		{Name: "AGENTS_CORE.md", Body: "# AGENTS_CORE\nstub", SortOrder: 10},
		{Name: "AGENTS_TASK.md", Body: "# AGENTS_TASK\nstub", SortOrder: 20},
		{Name: "SOUL.md", Body: "# SOUL\nstub", SortOrder: 30},
		{Name: "IDENTITY.md", Body: "# IDENTITY\nstub", SortOrder: 40},
		{Name: "USER.md", Body: "# USER\nstub", SortOrder: 50},
		{Name: "USER_PREDEFINED.md", Body: "# USER_PREDEFINED\nstub", SortOrder: 60},
		{Name: "CAPABILITIES.md", Body: "# CAPABILITIES\nstub", SortOrder: 70},
		{Name: "RULE.md", Body: "# RULE\nstub", SortOrder: 80},
		{Name: "HEARTBEAT.md", Body: "# HEARTBEAT\nstub", SortOrder: 90},
	}
}

// OptionalPromptFileTemplates holds optional files that users can add on demand.
// PGO-1-BIZ-01: USER_CONTEXT replaces legacy USER + USER_PREDEFINED.
var OptionalPromptFileTemplates = map[string]AgentPromptFile{
	"USER_CONTEXT.md": {
		Name:      "USER_CONTEXT.md",
		Body:      "# USER_CONTEXT\n\n## 用户偏好\n（记录用户的稳定偏好，如语言、输出格式、关注领域等）\n\n## 背景信息\n（记录与用户交互相关的背景上下文）\n\n## 注意事项\n- 此文件为可选，由 Agent 根据交互自动维护\n- 仅记录与任务执行相关的偏好，不记录隐私信息",
		SortOrder: 60,
	},
}

func withFileDefaults(files []AgentPromptFile) []AgentPromptFile {
	result := make([]AgentPromptFile, 0, len(files))
	for i, file := range files {
		if strings.TrimSpace(file.Name) == "" {
			continue
		}
		if file.SortOrder == 0 {
			file.SortOrder = (i + 1) * 10
		}
		result = append(result, file)
	}
	return result
}

func configJSONFromSettings(settings AgentRuntimeSettings, files []AgentPromptFile) (string, error) {
	payload := map[string]any{
		"self_evolve": settings.SelfEvolve,
		"subagents": map[string]any{
			"enabled":                settings.SubagentsEnabled,
			"max_concurrency":        settings.SubagentsMaxConcurrency,
			"max_generation_depth":   settings.SubagentsMaxGenerationDepth,
			"max_children_per_agent": settings.SubagentsMaxChildrenPerAgent,
			"archive_after_minutes":  settings.SubagentsArchiveAfterMinutes,
			"max_retries":            settings.SubagentsMaxRetries,
			"model_override":         settings.SubagentsModelOverride,
		},
		"tools": map[string]any{
			"enabled":          settings.ToolsEnabled,
			"profile":          settings.ToolsProfile,
			"tool_call_prefix": settings.ToolsToolCallPrefix,
			"allow":            jsonList(settings.ToolsAllowJSON),
			"deny":             jsonList(settings.ToolsDenyJSON),
			"concurrent_allow": jsonList(settings.ToolsConcurrentAllowJSON),
			"retry": map[string]any{
				"enabled":             settings.ToolsRetryEnabled,
				"max_attempts":        settings.ToolsRetryMaxAttempts,
				"initial_interval_ms": settings.ToolsRetryInitialIntervalMs,
				"backoff_factor":      settings.ToolsRetryBackoffFactor,
				"max_interval_ms":     settings.ToolsRetryMaxIntervalMs,
				"jitter":              settings.ToolsRetryJitter,
			},
			"parallel_enabled":  settings.ToolsParallelEnabled,
			"streaming_enabled": settings.ToolsStreamingEnabled,
		},
		"intent_pass": map[string]any{
			"enabled": settings.IntentPassEnabled,
		},
		"memory": map[string]any{
			"enabled":          settings.MemoryEnabled,
			"max_chunk_length": settings.MemoryMaxChunkLength,
			"max_results":      settings.MemoryMaxResults,
			"min_score":        settings.MemoryMinScore,
		},
		"memoryL0": map[string]any{
			"recent_window_turns":  settings.L0RecentWindowTurns,
			"recent_window_tokens": settings.L0RecentWindowTokens,
			"summary_threshold":    settings.L0SummaryThreshold,
			"summary_keep_turns":   settings.L0SummaryKeepTurns,
			"compress_provider":    settings.L0CompressProvider,
			"compress_model":       settings.L0CompressModel,
			"truncate_strategy":    settings.L0TruncateStrategy,
			"inject_l1":            settings.L0InjectL1,
			"inject_l3":            settings.L0InjectL3,
			"inject_l4":            settings.L0InjectL4,
			"l3_max_chunks":        settings.L0L3MaxChunks,
			"l4_max_paths":         settings.L0L4MaxPaths,
			"snapshot_mode":        settings.L0SnapshotMode,
		},
		"memoryWorker": map[string]any{
			"provider": settings.MemoryWorkerProvider,
			"model":    settings.MemoryWorkerModel,
		},
		"memoryL1": map[string]any{
			"enabled":                 settings.L1Enabled,
			"budget_tokens":           settings.L1BudgetTokens,
			"field_max_tokens":        settings.L1FieldMaxTokens,
			"history_keep_revisions":  settings.L1HistoryKeepRevisions,
			"default_schema_id":       settings.L1DefaultSchemaID,
			"archive_on_idle_minutes": settings.L1ArchiveOnIdleMinutes,
		},
		"memoryL2": map[string]any{
			"episode_enabled":        settings.L2EpisodeEnabled,
			"episode_min_importance": settings.L2EpisodeMinImportance,
			"index_enabled":          settings.L2IndexEnabled,
			"index_embedding_model":  settings.L2IndexEmbeddingModel,
			"recall_enabled":         settings.L2RecallEnabled,
			"recall_max":             settings.L2RecallMax,
			"retention_days":         settings.L2RetentionDays,
			"archive_after_days":     settings.L2ArchiveAfterDays,
		},
		"memoryL3": map[string]any{
			"enabled":              settings.L3Enabled,
			"recall_top_k":         settings.L3RecallTopK,
			"recall_min_score":     settings.L3RecallMinScore,
			"recall_scopes":        jsonList(settings.L3RecallScopesJSON),
			"embedding_model":      settings.L3EmbeddingModel,
			"decay_interval_hours": settings.L3DecayIntervalHours,
			"archive_threshold":    settings.L3ArchiveThreshold,
			"max_per_recall_chars": settings.L3MaxPerRecallChars,
		},
		"memoryL4": map[string]any{
			"enabled":                settings.L4Enabled,
			"graph_inject_neighbors": settings.L4GraphInjectNeighbors,
			"graph_max_neighbors":    settings.L4GraphMaxNeighbors,
			"graph_max_hops":         settings.L4GraphMaxHops,
			"identity_inject":        settings.L4IdentityInject,
			"strategy_inject":        settings.L4StrategyInject,
		},
		"evolutionSettings": map[string]any{
			"enabled":                    settings.EvoEnabled,
			"auto_apply":                 settings.EvoAutoApply,
			"min_episodes":               settings.EvoMinEpisodes,
			"min_negative_feedback":      settings.EvoMinNegativeFeedback,
			"throttle_hours":             settings.EvoThrottleHours,
			"proposal_ttl_days":          settings.EvoProposalTTLDays,
			"persona_max_chars":          settings.EvoPersonaMaxChars,
			"system_prompt_max_appends":  settings.EvoSystemPromptMaxAppends,
		},
		"heartbeat": map[string]any{
			"enabled":          settings.HeartbeatEnabled,
			"interval_minutes": settings.HeartbeatIntervalMinutes,
		},
		"evolution": map[string]any{
			"self_evolve":                   settings.EvolutionSelfEvolve,
			"skill_evolve":                  settings.EvolutionSkillEvolve,
			"evolution_metrics_enabled":     settings.EvolutionMetricsEnabled,
			"evolution_suggestions_enabled": settings.EvolutionSuggestionsEnabled,
		},
		"evolution_guardrails": map[string]any{
			"max_change_per_period":       settings.GuardrailMaxChangePerPeriod,
			"min_data_points":             settings.GuardrailMinDataPoints,
			"rollback_on_decline_percent": settings.GuardrailRollbackOnDeclinePercent,
		},
		"l0": map[string]any{
			"recent_window_turns":  settings.L0RecentWindowTurns,
			"recent_window_tokens": settings.L0RecentWindowTokens,
			"summary_threshold":    settings.L0SummaryThreshold,
			"summary_keep_turns":   settings.L0SummaryKeepTurns,
			"compress_min_gap_sec": settings.L0CompressMinGapSec,
			"compress_provider":    settings.L0CompressProvider,
			"compress_model":       settings.L0CompressModel,
			"worker_provider":      settings.MemoryWorkerProvider,
			"worker_model":         settings.MemoryWorkerModel,
			"truncate_strategy":    settings.L0TruncateStrategy,
			"inject_l1":            settings.L0InjectL1,
			"inject_l3":            settings.L0InjectL3,
			"inject_l4":            settings.L0InjectL4,
			"l3_max_chunks":        settings.L0L3MaxChunks,
			"l4_max_paths":         settings.L0L4MaxPaths,
			"snapshot_mode":        settings.L0SnapshotMode,
		},
		"files": files,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", kerrors.InternalServer("AGENT_SETTINGS", "config json marshal: "+err.Error())
	}
	return string(data), nil
}

// composePromptPreview generates a human-readable preview of what the system
// instruction will look like for the given mode. PGO-1-BIZ-03: shows the
// role_responsibility block when the inject flag is set and the agent has a
// position_id (populated by callers that pass it via agent.Files
// being pre-resolved or via a separate CategoryResponsibility field).
func composePromptPreview(agent Agent, mode string) string {
	var b strings.Builder
	settings := agent.Settings
	b.WriteString("# Agent 系统提示词\n\n")
	b.WriteString(fmt.Sprintf("模式: %s\n名称: %s\n标识: %s\n提供商: %s\n模型: %s\n\n", mode, agent.DisplayName, agent.AgentKey, agent.Provider, agent.Model))
	// Show role_responsibility block if available (injected by preview handler).
	if cr := strings.TrimSpace(agent.CategoryResponsibilityPreview); cr != "" {
		b.WriteString("## 角色职责（分类，L1）\n")
		b.WriteString(cr)
		b.WriteString("\n\n")
	}
	b.WriteString("## 描述\n")
	b.WriteString(strFallback(agent.AgentDescription, "未配置描述"))
	b.WriteString("\n\n")
	if mode != "none" {
		for _, file := range FilesForMode(agent.Files, mode) {
			b.WriteString(fmt.Sprintf("## %s\n", file.Name))
			b.WriteString(strings.TrimSpace(file.Body))
			b.WriteString("\n\n")
		}
	}
	if settings != nil && mode != "none" {
		b.WriteString("## 运行时设置\n")
		b.WriteString(fmt.Sprintf("- 自进化: %t\n", settings.SelfEvolve))
		b.WriteString(fmt.Sprintf("- 子代理: 启用=%t, 最大并发=%d, 最大深度=%d\n", settings.SubagentsEnabled, settings.SubagentsMaxConcurrency, settings.SubagentsMaxGenerationDepth))
		b.WriteString(fmt.Sprintf("- 工具: 启用=%t, 配置=%s, 允许=%s, 禁止=%s\n", settings.ToolsEnabled, settings.ToolsProfile, strings.Join(jsonList(settings.ToolsAllowJSON), ", "), strings.Join(jsonList(settings.ToolsDenyJSON), ", ")))
		b.WriteString(fmt.Sprintf("- 意图传递: %t\n", settings.IntentPassEnabled))
		b.WriteString(fmt.Sprintf("- 记忆: 启用=%t, 最大结果=%d, 最低分数=%.2f\n", settings.MemoryEnabled, settings.MemoryMaxResults, settings.MemoryMinScore))
		b.WriteString(fmt.Sprintf("- 心跳: 启用=%t, 间隔=%d 分钟\n", settings.HeartbeatEnabled, settings.HeartbeatIntervalMinutes))
		b.WriteString(fmt.Sprintf("- 进化: 风格=%t, 技能=%t, 指标=%t, 建议=%t\n", settings.EvolutionSelfEvolve, settings.EvolutionSkillEvolve, settings.EvolutionMetricsEnabled, settings.EvolutionSuggestionsEnabled))
	}
	return strings.TrimSpace(b.String())
}

// FilesForMode filters prompt files based on the agent's system_prompt_mode.
// PGO-1-BIZ-02: task mode no longer includes HEARTBEAT.md (moved to Settings).
// Whitelist per mode:
//   - complete / "": all files
//   - task:        AGENTS_CORE, IDENTITY, RULE, AGENTS_TASK, CAPABILITIES
//   - minimized:   AGENTS_CORE, RULE
//   - none:        empty
//   - unknown:     AGENTS_CORE, RULE (same as minimized, safe default)
func FilesForMode(files []AgentPromptFile, mode string) []AgentPromptFile {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "complete" {
		return files
	}
	if mode == "none" {
		return nil
	}
	allowed := map[string]bool{}
	switch mode {
	case "minimized":
		allowed["AGENTS_CORE.md"] = true
		allowed["RULE.md"] = true
	case "task":
		allowed["AGENTS_CORE.md"] = true
		allowed["IDENTITY.md"] = true
		allowed["RULE.md"] = true
		allowed["AGENTS_TASK.md"] = true
		allowed["CAPABILITIES.md"] = true
		// HEARTBEAT.md intentionally removed: heartbeat is now a runtime
		// Settings concern, not a static prompt file. PGO-1-BIZ-02.
	default:
		// Unknown modes fall back to minimized core rules to avoid leaking full prompt files.
		allowed["AGENTS_CORE.md"] = true
		allowed["RULE.md"] = true
	}
	result := []AgentPromptFile{}
	for _, file := range files {
		if allowed[file.Name] {
			result = append(result, file)
		}
	}
	return result
}

func mustJSON(values []string) string {
	if values == nil {
		return "[]"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func jsonList(raw string) []string {
	var result []string
	if json.Unmarshal([]byte(raw), &result) != nil {
		return []string{}
	}
	return result
}

func strFallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func settingsFromAgentInput(agent Agent) AgentRuntimeSettings {
	if agent.Settings != nil {
		return withSettingDefaults(*agent.Settings)
	}
	return settingsFromLegacyConfig(agent.ConfigJSON)
}

func filesFromAgentInput(agent Agent) []AgentPromptFile {
	if agent.Files != nil {
		return withFileDefaults(agent.Files)
	}
	files := filesFromLegacyConfig(agent.ConfigJSON)
	if len(files) == 0 {
		return defaultPromptFiles()
	}
	return files
}

func estimateTokensForFiles(files []AgentPromptFile) FileTokenEstimates {
	result := FileTokenEstimates{}
	for _, f := range files {
		tokens := len(f.Body) / 4
		if tokens == 0 && len(f.Body) > 0 {
			tokens = 1
		}
		result.FileEstimates = append(result.FileEstimates, FileTokenEstimate{
			FileID:          f.ID,
			FileName:        f.Name,
			EstimatedTokens: tokens,
		})
		result.TotalTokens += tokens
	}
	return result
}
