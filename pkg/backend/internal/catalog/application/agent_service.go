package application

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// AgentService 负责 Agent 编目 CRUD、运行时设置水合与提示预览（Catalog 应用层）。
type AgentService struct {
	repo repository.Store
}

// NewAgentService 在仓库上构建服务。
func NewAgentService(repo repository.Store) *AgentService {
	return &AgentService{repo: repo}
}

func (s *AgentService) List() ([]domain.Agent, error) {
	return s.repo.ListAgents()
}

func (s *AgentService) Search(query domain.AgentListQuery) (domain.AgentListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 24
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return s.repo.SearchAgents(query)
}

func (s *AgentService) Get(id string) (domain.Agent, error) {
	agent, err := s.repo.GetAgentByID(id)
	if err != nil {
		return domain.Agent{}, err
	}
	return s.hydrateAgentSettings(agent)
}

func (s *AgentService) Create(in domain.Agent) (domain.Agent, error) {
	in.ID = newID()
	settings := settingsFromAgentInput(in)
	files := filesFromAgentInput(in)
	in.ConfigJSON = configJSONFromSettings(settings, files)
	created, err := s.repo.CreateAgent(in)
	if err != nil {
		return domain.Agent{}, err
	}
	if _, err = s.saveAgentSettings(created.ID, settings, files, true); err != nil {
		return domain.Agent{}, err
	}
	return s.hydrateAgentSettings(created)
}

func (s *AgentService) Update(id string, in domain.Agent) (domain.Agent, error) {
	in.ID = id
	settings := settingsFromAgentInput(in)
	files := filesFromAgentInput(in)
	in.ConfigJSON = configJSONFromSettings(settings, files)
	updated, err := s.repo.UpdateAgent(in)
	if err != nil {
		return domain.Agent{}, err
	}
	if _, err = s.saveAgentSettings(updated.ID, settings, files, in.Files != nil); err != nil {
		return domain.Agent{}, err
	}
	return s.hydrateAgentSettings(updated)
}

func (s *AgentService) Delete(id string) error {
	return s.repo.DeleteAgent(id)
}

func (s *AgentService) PromptPreview(id string, mode string) (string, error) {
	agent, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(mode) == "" {
		mode = agent.SystemPromptMode
	}
	if strings.TrimSpace(mode) == "" {
		mode = "default"
	}

	return composePromptPreview(agent, mode), nil
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func (s *AgentService) hydrateAgentSettings(agent domain.Agent) (domain.Agent, error) {
	settings, err := s.repo.GetAgentRuntimeSettings(agent.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.Agent{}, err
		}
		settings = settingsFromLegacyConfig(agent.ConfigJSON)
		settings.AgentID = agent.ID
		if settings, err = s.repo.UpsertAgentRuntimeSettings(settings); err != nil {
			return domain.Agent{}, err
		}
	}
	files, err := s.repo.ListAgentPromptFiles(agent.ID)
	if err != nil {
		return domain.Agent{}, err
	}
	if len(files) == 0 {
		files = filesFromLegacyConfig(agent.ConfigJSON)
		if len(files) == 0 {
			files = defaultPromptFiles()
		}
		if files, err = s.repo.ReplaceAgentPromptFiles(agent.ID, files); err != nil {
			return domain.Agent{}, err
		}
	}
	agent.Settings = &settings
	agent.Files = files
	return agent, nil
}

func (s *AgentService) saveAgentSettings(agentID string, settings domain.AgentRuntimeSettings, files []domain.AgentPromptFile, replaceFiles bool) (domain.AgentRuntimeSettings, error) {
	settings.AgentID = agentID
	saved, err := s.repo.UpsertAgentRuntimeSettings(settings)
	if err != nil {
		return domain.AgentRuntimeSettings{}, err
	}
	if replaceFiles {
		if len(files) == 0 {
			files = defaultPromptFiles()
		}
		if _, err = s.repo.ReplaceAgentPromptFiles(agentID, files); err != nil {
			return domain.AgentRuntimeSettings{}, err
		}
	}
	return saved, nil
}

func settingsFromAgentInput(agent domain.Agent) domain.AgentRuntimeSettings {
	if agent.Settings != nil {
		return withSettingDefaults(*agent.Settings)
	}
	return settingsFromLegacyConfig(agent.ConfigJSON)
}

func filesFromAgentInput(agent domain.Agent) []domain.AgentPromptFile {
	if agent.Files != nil {
		return withFileDefaults(agent.Files)
	}
	files := filesFromLegacyConfig(agent.ConfigJSON)
	if len(files) == 0 {
		return defaultPromptFiles()
	}
	return files
}

func defaultRuntimeSettings() domain.AgentRuntimeSettings {
	return domain.DefaultAgentRuntimeSettings()
}

// withSettingDefaults 为不完整的 AgentRuntimeSettings 补全缺省数字/字符串。
// 此处有意不为布尔置默认值，以便显式关闭的功能在
// 创建/更新调用中仍被保留。
func withSettingDefaults(v domain.AgentRuntimeSettings) domain.AgentRuntimeSettings {
	d := defaultRuntimeSettings()
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

func settingsFromLegacyConfig(raw string) domain.AgentRuntimeSettings {
	settings := defaultRuntimeSettings()
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
	return settings
}

func filesFromLegacyConfig(raw string) []domain.AgentPromptFile {
	var parsed struct {
		Files []domain.AgentPromptFile `json:"files"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return nil
	}
	return withFileDefaults(parsed.Files)
}

func defaultPromptFiles() []domain.AgentPromptFile {
	return []domain.AgentPromptFile{
		{Name: "AGENTS_CORE.md", Body: "# AGENTS_CORE\n遵循用户语言，保存变更必须通过文件工具。", SortOrder: 10},
		{Name: "AGENTS_TASK.md", Body: "# AGENTS_TASK\n执行企业任务时保持可追踪、可恢复。", SortOrder: 20},
		{Name: "SOUL.md", Body: "# SOUL\n保持专业、清晰、克制。", SortOrder: 30},
		{Name: "IDENTITY.md", Body: "# IDENTITY\n我是企业级 Agent。", SortOrder: 40},
		{Name: "USER.md", Body: "# USER\n记录稳定偏好。", SortOrder: 50},
		{Name: "USER_PREDEFINED.md", Body: "# USER_PREDEFINED\n暂无。", SortOrder: 60},
		{Name: "CAPABILITIES.md", Body: "# CAPABILITIES\n可进行分析、执行和复盘。", SortOrder: 70},
		{Name: "RULE.md", Body: "# RULE\n不得越权操作。", SortOrder: 80},
		{Name: "HEARTBEAT.md", Body: "# 心跳检查清单\n- 检查待处理任务\n- 报告当前状态", SortOrder: 90},
	}
}

func withFileDefaults(files []domain.AgentPromptFile) []domain.AgentPromptFile {
	result := make([]domain.AgentPromptFile, 0, len(files))
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

func configJSONFromSettings(settings domain.AgentRuntimeSettings, files []domain.AgentPromptFile) string {
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

func composePromptPreview(agent domain.Agent, mode string) string {
	var b strings.Builder
	settings := agent.Settings
	b.WriteString("# Agent System Prompt\n\n")
	b.WriteString(fmt.Sprintf("Mode: %s\nName: %s\nKey: %s\nProvider: %s\nModel: %s\n\n", mode, agent.DisplayName, agent.AgentKey, agent.Provider, agent.Model))
	b.WriteString("## Description\n")
	b.WriteString(fallback(agent.AgentDescription, "No description configured."))
	b.WriteString("\n\n")
	if mode != "none" {
		for _, file := range filesForMode(agent.Files, mode) {
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
		b.WriteString(fmt.Sprintf("- Memory: enabled=%t, max_results=%d, min_score=%.2f\n", settings.MemoryEnabled, settings.MemoryMaxResults, settings.MemoryMinScore))
		b.WriteString(fmt.Sprintf("- Heartbeat: enabled=%t, interval=%d minutes\n", settings.HeartbeatEnabled, settings.HeartbeatIntervalMinutes))
		b.WriteString(fmt.Sprintf("- Evolution: style=%t, skill=%t, metrics=%t, suggestions=%t\n", settings.EvolutionSelfEvolve, settings.EvolutionSkillEvolve, settings.EvolutionMetricsEnabled, settings.EvolutionSuggestionsEnabled))
	}
	return strings.TrimSpace(b.String())
}

func filesForMode(files []domain.AgentPromptFile, mode string) []domain.AgentPromptFile {
	if mode == "complete" {
		return files
	}
	allowed := map[string]bool{"AGENTS_CORE.md": true, "IDENTITY.md": true, "RULE.md": true}
	if mode == "task" {
		allowed["AGENTS_TASK.md"] = true
		allowed["CAPABILITIES.md"] = true
		allowed["HEARTBEAT.md"] = true
	}
	result := []domain.AgentPromptFile{}
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
