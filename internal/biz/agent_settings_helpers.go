package biz

import (
	"encoding/json"
	"fmt"
	"strings"
)

// withSettingDefaults fills missing numeric/string fields in AgentRuntimeSettings without turning unset booleans into forced defaults.
func withSettingDefaults(v AgentRuntimeSettings) AgentRuntimeSettings {
	d := DefaultAgentRuntimeSettings()
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

func defaultPromptFiles() []AgentPromptFile {
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

func configJSONFromSettings(settings AgentRuntimeSettings, files []AgentPromptFile) string {
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
		return "{}"
	}
	return string(data)
}

func composePromptPreview(agent Agent, mode string) string {
	var b strings.Builder
	settings := agent.Settings
	b.WriteString("# Agent System Prompt\n\n")
	b.WriteString(fmt.Sprintf("Mode: %s\nName: %s\nKey: %s\nProvider: %s\nModel: %s\n\n", mode, agent.DisplayName, agent.AgentKey, agent.Provider, agent.Model))
	b.WriteString("## Description\n")
	b.WriteString(strFallback(agent.AgentDescription, "No description configured."))
	b.WriteString("\n\n")
	if mode != "none" {
		for _, file := range FilesForMode(agent.Files, mode) {
			b.WriteString(fmt.Sprintf("## %s\n", file.Name))
			b.WriteString(strings.TrimSpace(file.Body))
			b.WriteString("\n\n")
		}
	}
	if settings != nil && mode != "none" {
		b.WriteString("## Runtime Settings\n")
		b.WriteString(fmt.Sprintf("- Self evolve: %t\n", settings.SelfEvolve))
		b.WriteString(fmt.Sprintf("- Subagents: enabled=%t, max_concurrency=%d, max_depth=%d\n", settings.SubagentsEnabled, settings.SubagentsMaxConcurrency, settings.SubagentsMaxGenerationDepth))
		b.WriteString(fmt.Sprintf("- Tools: enabled=%t, profile=%s, allow=%s, deny=%s\n", settings.ToolsEnabled, settings.ToolsProfile, strings.Join(jsonList(settings.ToolsAllowJSON), ", "), strings.Join(jsonList(settings.ToolsDenyJSON), ", ")))
		b.WriteString(fmt.Sprintf("- Intent pass: %t\n", settings.IntentPassEnabled))
		b.WriteString(fmt.Sprintf("- Memory: enabled=%t, max_results=%d, min_score=%.2f\n", settings.MemoryEnabled, settings.MemoryMaxResults, settings.MemoryMinScore))
		b.WriteString(fmt.Sprintf("- Heartbeat: enabled=%t, interval=%d minutes\n", settings.HeartbeatEnabled, settings.HeartbeatIntervalMinutes))
		b.WriteString(fmt.Sprintf("- Evolution: style=%t, skill=%t, metrics=%t, suggestions=%t\n", settings.EvolutionSelfEvolve, settings.EvolutionSkillEvolve, settings.EvolutionMetricsEnabled, settings.EvolutionSuggestionsEnabled))
	}
	return strings.TrimSpace(b.String())
}

func FilesForMode(files []AgentPromptFile, mode string) []AgentPromptFile {
	if mode == "" || mode == "complete" {
		return files
	}
	if mode == "none" {
		return nil
	}
	allowed := map[string]bool{"AGENTS_CORE.md": true, "IDENTITY.md": true, "RULE.md": true}
	if mode == "task" {
		allowed["AGENTS_TASK.md"] = true
		allowed["CAPABILITIES.md"] = true
		allowed["HEARTBEAT.md"] = true
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
