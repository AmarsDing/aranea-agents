package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/agentpromptfile"
	"aranea-agents/internal/data/ent/agentruntimesetting"
	"aranea-agents/internal/data/ent/predicate"

	entsql "entgo.io/ent/dialect/sql"
)

type agentRepo struct {
	data *Data
}

// NewAgentRepo implements biz.AgentRepository.
func NewAgentRepo(d *Data) biz.AgentRepository {
	return &agentRepo{data: d}
}

func normalizeJSONList(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	b, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(b)
}

func normalizeSkillRuntimeJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	return "{}"
}

func normalizeJSONObj(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	return "{}"
}

func sanitizePromptFileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}

func entAgentToBiz(a *ent.Agent) biz.Agent {
	if a == nil {
		return biz.Agent{}
	}
	agent := biz.Agent{
		ID:                 a.ID,
		AgentKey:           a.AgentKey,
		DisplayName:        a.DisplayName,
		Provider:           a.Provider,
		Model:              a.Model,
		Status:             a.Status,
		IsDefault:          a.IsDefault,
		IsFavorite:         a.IsFavorite,
		Icon:               a.Icon,
		AgentDescription:   a.AgentDescription,
		CategoryPositionID: a.CategoryPositionID,
		SystemPromptMode:   a.SystemPromptMode,
		ContextWindow:      a.ContextWindow,
		BudgetMonthlyCents: a.BudgetMonthlyCents,
		ConfigJSON:         a.ConfigJSON,
		CreatedBy:          a.CreatedBy,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
		DeletedAt:          a.DeletedAt,
	}
	_ = json.Unmarshal([]byte(a.RolesJSON), &agent.Roles)
	return agent
}

func entRuntimeToBiz(e *ent.AgentRuntimeSetting) biz.AgentRuntimeSettings {
	if e == nil {
		return biz.AgentRuntimeSettings{}
	}
	return biz.AgentRuntimeSettings{
		AgentID:                           e.ID,
		SelfEvolve:                        e.SelfEvolve,
		SubagentsEnabled:                  e.SubagentsEnabled,
		SubagentsMaxConcurrency:           e.SubagentsMaxConcurrency,
		SubagentsMaxGenerationDepth:       e.SubagentsMaxGenerationDepth,
		SubagentsMaxChildrenPerAgent:      e.SubagentsMaxChildrenPerAgent,
		SubagentsArchiveAfterMinutes:      e.SubagentsArchiveAfterMinutes,
		SubagentsMaxRetries:               e.SubagentsMaxRetries,
		SubagentsModelOverride:            e.SubagentsModelOverride,
		ToolsEnabled:                      e.ToolsEnabled,
		ToolsProfile:                      e.ToolsProfile,
		ToolsToolCallPrefix:               e.ToolsToolCallPrefix,
		ToolsAllowJSON:                    e.ToolsAllowJSON,
		ToolsDenyJSON:                     e.ToolsDenyJSON,
		ToolsConcurrentAllowJSON:          e.ToolsConcurrentAllowJSON,
		MemoryEnabled:                     e.MemoryEnabled,
		MemoryMaxChunkLength:              e.MemoryMaxChunkLength,
		MemoryMaxResults:                  e.MemoryMaxResults,
		MemoryMinScore:                    e.MemoryMinScore,
		HeartbeatEnabled:                  e.HeartbeatEnabled,
		HeartbeatIntervalMinutes:          e.HeartbeatIntervalMinutes,
		EvolutionSelfEvolve:               e.EvolutionSelfEvolve,
		EvolutionSkillEvolve:              e.EvolutionSkillEvolve,
		EvolutionMetricsEnabled:           e.EvolutionMetricsEnabled,
		EvolutionSuggestionsEnabled:       e.EvolutionSuggestionsEnabled,
		GuardrailMaxChangePerPeriod:       e.GuardrailMaxChangePerPeriod,
		GuardrailMinDataPoints:            e.GuardrailMinDataPoints,
		GuardrailRollbackOnDeclinePercent: e.GuardrailRollbackOnDeclinePercent,
		L0RecentWindowTurns:               e.L0RecentWindowTurns,
		L0RecentWindowTokens:              e.L0RecentWindowTokens,
		L0SummaryThreshold:                e.L0SummaryThreshold,
		L0SummaryKeepTurns:                e.L0SummaryKeepTurns,
		L0CompressProvider:                e.L0CompressProvider,
		L0CompressModel:                   e.L0CompressModel,
		MemoryWorkerProvider:              e.MemoryWorkerProvider,
		MemoryWorkerModel:                 e.MemoryWorkerModel,
		L0TruncateStrategy:                e.L0TruncateStrategy,
		L0InjectL1:                        e.L0InjectL1,
		L0InjectL3:                        e.L0InjectL3,
		L0InjectL4:                        e.L0InjectL4,
		L0L3MaxChunks:                     e.L0L3MaxChunks,
		L0L4MaxPaths:                      e.L0L4MaxPaths,
		L0SnapshotMode:                    e.L0SnapshotMode,
		L1Enabled:                         e.L1Enabled,
		L1BudgetTokens:                    e.L1BudgetTokens,
		L1FieldMaxTokens:                  e.L1FieldMaxTokens,
		L1HistoryKeepRevisions:            e.L1HistoryKeepRevisions,
		L1DefaultSchemaID:                 e.L1DefaultSchemaID,
		L1ArchiveOnIdleMinutes:            e.L1ArchiveOnIdleMinutes,
		L2EpisodeEnabled:                  e.L2EpisodeEnabled,
		L2EpisodeMinImportance:            e.L2EpisodeMinImportance,
		L2IndexEnabled:                    e.L2IndexEnabled,
		L2IndexEmbeddingModel:             e.L2IndexEmbeddingModel,
		L2RecallEnabled:                   e.L2RecallEnabled,
		L2RecallMax:                       e.L2RecallMax,
		L2RetentionDays:                   e.L2RetentionDays,
		L2ArchiveAfterDays:                e.L2ArchiveAfterDays,
		L3Enabled:                         e.L3Enabled,
		L3RecallTopK:                      e.L3RecallTopK,
		L3RecallMinScore:                  e.L3RecallMinScore,
		L3RecallScopesJSON:                e.L3RecallScopesJSON,
		L3EmbeddingModel:                  e.L3EmbeddingModel,
		L3DecayIntervalHours:              e.L3DecayIntervalHours,
		L3ArchiveThreshold:                e.L3ArchiveThreshold,
		L3MaxPerRecallChars:               e.L3MaxPerRecallChars,
		L4Enabled:                         e.L4Enabled,
		L4GraphInjectNeighbors:            e.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:               e.L4GraphMaxNeighbors,
		L4GraphMaxHops:                    e.L4GraphMaxHops,
		L4IdentityInject:                  e.L4IdentityInject,
		L4StrategyInject:                  e.L4StrategyInject,
		EvoEnabled:                        e.EvoEnabled,
		EvoAutoApply:                      e.EvoAutoApply,
		EvoMinEpisodes:                    e.EvoMinEpisodes,
		EvoMinNegativeFeedback:            e.EvoMinNegativeFeedback,
		EvoThrottleHours:                  e.EvoThrottleHours,
		EvoProposalTTLDays:                e.EvoProposalTTLDays,
		EvoPersonaMaxChars:                e.EvoPersonaMaxChars,
		EvoSystemPromptMaxAppends:         e.EvoSystemPromptMaxAppends,
		SkillRuntimeJSON:                  e.SkillRuntimeJSON,
		IntentPassEnabled:                 e.IntentPassEnabled,
		ChannelID:                         e.ChannelID,
		ChatID:                            e.ChatID,
		Workspace:                         e.Workspace,
		ReasoningMode:                     e.ReasoningMode,
		ReasoningLevel:                    e.ReasoningLevel,
		VariablesJSON:                     e.VariablesJSON,
		ModelInstructionsJSON:             e.ModelInstructionsJSON,
		ContextCompactionEnabled:          e.ContextCompactionEnabled,
		SessionSummaryEnabled:             e.SessionSummaryEnabled,
		SkillLoadMode:                     e.SkillLoadMode,
		CodeExecutorType:                  e.CodeExecutorType,
		PlannerKind:                       e.PlannerKind,
		PlannerConfigJSON:                 e.PlannerConfigJSON,
		RalphLoopMaxIterations:            e.RalphLoopMaxIterations,
		RalphLoopCompletionPromise:        e.RalphLoopCompletionPromise,
		RalphLoopVerifyCommand:            e.RalphLoopVerifyCommand,
		RalphLoopVerifyTimeoutSeconds:     e.RalphLoopVerifyTimeoutSeconds,
		RalphLoopPromiseTagOpen:           e.RalphLoopPromiseTagOpen,
		RalphLoopPromiseTagClose:          e.RalphLoopPromiseTagClose,
		RalphLoopVerifyWorkDir:            e.RalphLoopVerifyWorkDir,
		OutputSchemaJSON:                  e.OutputSchemaJSON,
		ModelSelector:                     e.ModelSelector,
		ToolsRetryEnabled:                 e.ToolsRetryEnabled,
		ToolsRetryMaxAttempts:             e.ToolsRetryMaxAttempts,
		ToolsRetryInitialIntervalMs:       e.ToolsRetryInitialIntervalMs,
		ToolsRetryBackoffFactor:           e.ToolsRetryBackoffFactor,
		ToolsRetryMaxIntervalMs:           e.ToolsRetryMaxIntervalMs,
		ToolsRetryJitter:                  e.ToolsRetryJitter,
		ToolsParallelEnabled:              e.ToolsParallelEnabled,
		ToolsStreamingEnabled:             e.ToolsStreamingEnabled,
		CreatedAt:                         e.CreatedAt,
		UpdatedAt:                         e.UpdatedAt,
	}
}

func entPromptToBiz(e *ent.AgentPromptFile) biz.AgentPromptFile {
	if e == nil {
		return biz.AgentPromptFile{}
	}
	return biz.AgentPromptFile{
		ID:        e.ID,
		AgentID:   e.AgentID,
		Name:      e.FileName,
		Body:      e.Body,
		SortOrder: e.SortOrder,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

func applyBizRuntimeToCreate(b *ent.AgentRuntimeSettingCreate, v biz.AgentRuntimeSettings) {
	b.SetSelfEvolve(v.SelfEvolve).
		SetSubagentsEnabled(v.SubagentsEnabled).
		SetSubagentsMaxConcurrency(v.SubagentsMaxConcurrency).
		SetSubagentsMaxGenerationDepth(v.SubagentsMaxGenerationDepth).
		SetSubagentsMaxChildrenPerAgent(v.SubagentsMaxChildrenPerAgent).
		SetSubagentsArchiveAfterMinutes(v.SubagentsArchiveAfterMinutes).
		SetSubagentsMaxRetries(v.SubagentsMaxRetries).
		SetSubagentsModelOverride(v.SubagentsModelOverride).
		SetToolsEnabled(v.ToolsEnabled).
		SetToolsProfile(v.ToolsProfile).
		SetToolsToolCallPrefix(v.ToolsToolCallPrefix).
		SetToolsAllowJSON(normalizeJSONList(v.ToolsAllowJSON)).
		SetToolsDenyJSON(normalizeJSONList(v.ToolsDenyJSON)).
		SetToolsConcurrentAllowJSON(normalizeJSONList(v.ToolsConcurrentAllowJSON)).
		SetMemoryEnabled(v.MemoryEnabled).
		SetMemoryMaxChunkLength(v.MemoryMaxChunkLength).
		SetMemoryMaxResults(v.MemoryMaxResults).
		SetMemoryMinScore(v.MemoryMinScore).
		SetHeartbeatEnabled(v.HeartbeatEnabled).
		SetHeartbeatIntervalMinutes(v.HeartbeatIntervalMinutes).
		SetEvolutionSelfEvolve(v.EvolutionSelfEvolve).
		SetEvolutionSkillEvolve(v.EvolutionSkillEvolve).
		SetEvolutionMetricsEnabled(v.EvolutionMetricsEnabled).
		SetEvolutionSuggestionsEnabled(v.EvolutionSuggestionsEnabled).
		SetGuardrailMaxChangePerPeriod(v.GuardrailMaxChangePerPeriod).
		SetGuardrailMinDataPoints(v.GuardrailMinDataPoints).
		SetGuardrailRollbackOnDeclinePercent(v.GuardrailRollbackOnDeclinePercent).
		SetL0RecentWindowTurns(v.L0RecentWindowTurns).
		SetL0RecentWindowTokens(v.L0RecentWindowTokens).
		SetL0SummaryThreshold(v.L0SummaryThreshold).
		SetL0SummaryKeepTurns(v.L0SummaryKeepTurns).
		SetL0CompressProvider(strings.TrimSpace(v.L0CompressProvider)).
		SetL0CompressModel(strings.TrimSpace(v.L0CompressModel)).
		SetMemoryWorkerProvider(strings.TrimSpace(v.MemoryWorkerProvider)).
		SetMemoryWorkerModel(strings.TrimSpace(v.MemoryWorkerModel)).
		SetL0TruncateStrategy(v.L0TruncateStrategy).
		SetL0InjectL1(v.L0InjectL1).
		SetL0InjectL3(v.L0InjectL3).
		SetL0InjectL4(v.L0InjectL4).
		SetL0L3MaxChunks(v.L0L3MaxChunks).
		SetL0L4MaxPaths(v.L0L4MaxPaths).
		SetL0SnapshotMode(v.L0SnapshotMode).
		SetL1Enabled(v.L1Enabled).
		SetL1BudgetTokens(v.L1BudgetTokens).
		SetL1FieldMaxTokens(v.L1FieldMaxTokens).
		SetL1HistoryKeepRevisions(v.L1HistoryKeepRevisions).
		SetL1DefaultSchemaID(v.L1DefaultSchemaID).
		SetL1ArchiveOnIdleMinutes(v.L1ArchiveOnIdleMinutes).
		SetL2EpisodeEnabled(v.L2EpisodeEnabled).
		SetL2EpisodeMinImportance(v.L2EpisodeMinImportance).
		SetL2IndexEnabled(v.L2IndexEnabled).
		SetL2IndexEmbeddingModel(v.L2IndexEmbeddingModel).
		SetL2RecallEnabled(v.L2RecallEnabled).
		SetL2RecallMax(v.L2RecallMax).
		SetL2RetentionDays(v.L2RetentionDays).
		SetL2ArchiveAfterDays(v.L2ArchiveAfterDays).
		SetL3Enabled(v.L3Enabled).
		SetL3RecallTopK(v.L3RecallTopK).
		SetL3RecallMinScore(v.L3RecallMinScore).
		SetL3RecallScopesJSON(normalizeJSONList(v.L3RecallScopesJSON)).
		SetL3EmbeddingModel(v.L3EmbeddingModel).
		SetL3DecayIntervalHours(v.L3DecayIntervalHours).
		SetL3ArchiveThreshold(v.L3ArchiveThreshold).
		SetL3MaxPerRecallChars(v.L3MaxPerRecallChars).
		SetL4Enabled(v.L4Enabled).
		SetL4GraphInjectNeighbors(v.L4GraphInjectNeighbors).
		SetL4GraphMaxNeighbors(v.L4GraphMaxNeighbors).
		SetL4GraphMaxHops(v.L4GraphMaxHops).
		SetL4IdentityInject(v.L4IdentityInject).
		SetL4StrategyInject(v.L4StrategyInject).
		SetEvoEnabled(v.EvoEnabled).
		SetEvoAutoApply(v.EvoAutoApply).
		SetEvoMinEpisodes(v.EvoMinEpisodes).
		SetEvoMinNegativeFeedback(v.EvoMinNegativeFeedback).
		SetEvoThrottleHours(v.EvoThrottleHours).
		SetEvoProposalTTLDays(v.EvoProposalTTLDays).
		SetEvoPersonaMaxChars(v.EvoPersonaMaxChars).
		SetEvoSystemPromptMaxAppends(v.EvoSystemPromptMaxAppends).
		SetSkillRuntimeJSON(normalizeSkillRuntimeJSON(v.SkillRuntimeJSON)).
		SetIntentPassEnabled(v.IntentPassEnabled).
		SetChannelID(v.ChannelID).
		SetChatID(v.ChatID).
		SetWorkspace(v.Workspace).
		SetReasoningMode(v.ReasoningMode).
		SetReasoningLevel(v.ReasoningLevel).
		SetVariablesJSON(normalizeJSONObj(v.VariablesJSON)).
		SetModelInstructionsJSON(normalizeJSONObj(v.ModelInstructionsJSON)).
		SetContextCompactionEnabled(v.ContextCompactionEnabled).
		SetSessionSummaryEnabled(v.SessionSummaryEnabled).
		SetSkillLoadMode(v.SkillLoadMode).
		SetCodeExecutorType(v.CodeExecutorType).
		SetPlannerKind(v.PlannerKind).
		SetPlannerConfigJSON(normalizeJSONObj(v.PlannerConfigJSON)).
		SetRalphLoopMaxIterations(v.RalphLoopMaxIterations).
		SetRalphLoopCompletionPromise(v.RalphLoopCompletionPromise).
		SetRalphLoopVerifyCommand(v.RalphLoopVerifyCommand).
		SetRalphLoopVerifyTimeoutSeconds(v.RalphLoopVerifyTimeoutSeconds).
		SetRalphLoopPromiseTagOpen(v.RalphLoopPromiseTagOpen).
		SetRalphLoopPromiseTagClose(v.RalphLoopPromiseTagClose).
		SetRalphLoopVerifyWorkDir(v.RalphLoopVerifyWorkDir).
		SetOutputSchemaJSON(v.OutputSchemaJSON).
		SetModelSelector(v.ModelSelector).
		SetToolsRetryEnabled(v.ToolsRetryEnabled).
		SetToolsRetryMaxAttempts(v.ToolsRetryMaxAttempts).
		SetToolsRetryInitialIntervalMs(v.ToolsRetryInitialIntervalMs).
		SetToolsRetryBackoffFactor(v.ToolsRetryBackoffFactor).
		SetToolsRetryMaxIntervalMs(v.ToolsRetryMaxIntervalMs).
		SetToolsRetryJitter(v.ToolsRetryJitter).
		SetToolsParallelEnabled(v.ToolsParallelEnabled).
		SetToolsStreamingEnabled(v.ToolsStreamingEnabled).
		SetCreatedAt(v.CreatedAt).
		SetUpdatedAt(v.UpdatedAt)
}

func (r *agentRepo) SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 24
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	preds := []predicate.Agent{agent.DeletedAtEQ("")}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		preds = append(preds, agent.Or(
			agent.AgentKeyContainsFold(kw),
			agent.DisplayNameContainsFold(kw),
			agent.ProviderContainsFold(kw),
			agent.ModelContainsFold(kw),
			agent.AgentDescriptionContainsFold(kw),
		))
	}
	if q.Status != "" {
		preds = append(preds, agent.StatusEQ(q.Status))
	}
	if q.Provider != "" {
		preds = append(preds, agent.ProviderEQ(q.Provider))
	}
	if q.CategoryID != "" {
		preds = append(preds, agent.CategoryPositionIDEQ(q.CategoryID))
	}
	if cb := strings.TrimSpace(q.CreatedBy); cb != "" {
		preds = append(preds, agent.CreatedByEQ(cb))
	}
	where := agent.And(preds...)
	c := r.data.entClient
	total, err := c.Agent.Query().Where(where).Count(ctx)
	if err != nil {
		return biz.AgentListResult{}, err
	}
	rows, err := c.Agent.Query().Where(where).
		Order(agent.ByIsDefault(entsql.OrderDesc()), agent.ByUpdatedAt(entsql.OrderDesc())).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.AgentListResult{}, err
	}
	items := make([]biz.Agent, 0, len(rows))
	for _, row := range rows {
		items = append(items, entAgentToBiz(row))
	}
	return biz.AgentListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *agentRepo) ListAgentCreators(ctx context.Context) ([]biz.AgentCreator, error) {
	rows, err := r.data.entClient.Agent.Query().
		Where(agent.DeletedAtEQ(""), agent.CreatedByNEQ("")).
		Select(agent.FieldCreatedBy).
		GroupBy(agent.FieldCreatedBy).
		Strings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.AgentCreator, 0, len(rows)+1)
	if biz.AgentCreatedByFromContext(ctx) != "" {
		out = append(out, biz.AgentCreator{UserID: biz.AgentListCreatedByMine, Label: "仅我的"})
	}
	seen := map[string]bool{}
	for _, id := range rows {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, biz.AgentCreator{UserID: id, Label: creatorLabel(id)})
	}
	return out, nil
}

func creatorLabel(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "未知"
	}
	return "用户 " + userID
}

func (r *agentRepo) GetAgentByID(ctx context.Context, id string) (biz.Agent, error) {
	row, err := r.data.entClient.Agent.Query().Where(agent.IDEQ(id), agent.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Agent{}, sql.ErrNoRows
		}
		return biz.Agent{}, err
	}
	return entAgentToBiz(row), nil
}

func (r *agentRepo) GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return biz.Agent{}, sql.ErrNoRows
	}
	row, err := r.data.entClient.Agent.Query().Where(agent.AgentKeyEQ(agentKey), agent.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Agent{}, sql.ErrNoRows
		}
		return biz.Agent{}, err
	}
	return entAgentToBiz(row), nil
}

func (r *agentRepo) CreateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
	if a.ID == "" || a.AgentKey == "" || a.DisplayName == "" || a.Provider == "" || a.Model == "" {
		return biz.Agent{}, fmt.Errorf("missing required fields")
	}
	now := nowRFC3339()
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if a.Status == "" {
		a.Status = "active"
	}
	_, err := r.data.entClient.Agent.Create().
		SetID(a.ID).
		SetAgentKey(a.AgentKey).
		SetDisplayName(a.DisplayName).
		SetProvider(a.Provider).
		SetModel(a.Model).
		SetStatus(a.Status).
		SetIsDefault(a.IsDefault).
		SetIsFavorite(a.IsFavorite).
		SetIcon(a.Icon).
		SetAgentDescription(a.AgentDescription).
		SetCategoryPositionID(a.CategoryPositionID).
		SetSystemPromptMode(a.SystemPromptMode).
		SetContextWindow(a.ContextWindow).
		SetBudgetMonthlyCents(a.BudgetMonthlyCents).
		SetConfigJSON(a.ConfigJSON).
		SetCreatedBy(a.CreatedBy).
		SetCreatedAt(a.CreatedAt).
		SetUpdatedAt(a.UpdatedAt).
		SetDeletedAt(a.DeletedAt).
		Save(ctx)
	if err != nil {
		return biz.Agent{}, err
	}
	return r.GetAgentByID(ctx, a.ID)
}

func (r *agentRepo) UpdateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
	if a.ID == "" {
		return biz.Agent{}, fmt.Errorf("id is required")
	}
	current, err := r.GetAgentByID(ctx, a.ID)
	if err != nil {
		return biz.Agent{}, err
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
	a.UpdatedAt = nowRFC3339()
	_, err = r.data.entClient.Agent.UpdateOneID(a.ID).
		SetDisplayName(a.DisplayName).
		SetProvider(a.Provider).
		SetModel(a.Model).
		SetStatus(a.Status).
		SetIsDefault(a.IsDefault).
		SetIsFavorite(a.IsFavorite).
		SetIcon(a.Icon).
		SetAgentDescription(a.AgentDescription).
		SetCategoryPositionID(a.CategoryPositionID).
		SetSystemPromptMode(a.SystemPromptMode).
		SetContextWindow(a.ContextWindow).
		SetBudgetMonthlyCents(a.BudgetMonthlyCents).
		SetConfigJSON(a.ConfigJSON).
		SetUpdatedAt(a.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Agent{}, err
	}
	return r.GetAgentByID(ctx, a.ID)
}

func (r *agentRepo) DeleteAgent(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	now := nowRFC3339()
	_, err := r.data.entClient.Agent.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *agentRepo) GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	row, err := r.data.entClient.AgentRuntimeSetting.Get(ctx, agentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AgentRuntimeSettings{}, sql.ErrNoRows
		}
		return biz.AgentRuntimeSettings{}, err
	}
	return entRuntimeToBiz(row), nil
}

func (r *agentRepo) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	if v.AgentID == "" {
		return biz.AgentRuntimeSettings{}, fmt.Errorf("agent id is required")
	}
	now := nowRFC3339()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	b := r.data.entClient.AgentRuntimeSetting.Create().SetID(v.AgentID)
	applyBizRuntimeToCreate(b, v)
	if err := b.OnConflict(
		entsql.ConflictColumns(agentruntimesetting.FieldID),
		entsql.ResolveWithNewValues(),
	).Exec(ctx); err != nil {
		return biz.AgentRuntimeSettings{}, err
	}
	row, err := r.data.entClient.AgentRuntimeSetting.Get(ctx, v.AgentID)
	if err != nil {
		return biz.AgentRuntimeSettings{}, err
	}
	return entRuntimeToBiz(row), nil
}

func (r *agentRepo) ListAgentPromptFiles(ctx context.Context, agentID string) ([]biz.AgentPromptFile, error) {
	rows, err := r.data.entClient.AgentPromptFile.Query().
		Where(agentpromptfile.AgentIDEQ(agentID)).
		Order(agentpromptfile.BySortOrder(), agentpromptfile.ByFileName()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.AgentPromptFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, entPromptToBiz(row))
	}
	return out, nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	tx, err := r.data.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(ctx); err != nil {
		return nil, err
	}
	now := nowRFC3339()
	for i, file := range files {
		if strings.TrimSpace(file.Name) == "" {
			continue
		}
		id := file.ID
		if id == "" {
			id = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
		}
		sortOrder := file.SortOrder
		if sortOrder == 0 {
			sortOrder = (i + 1) * 10
		}
		if _, err = tx.AgentPromptFile.Create().
			SetID(id).
			SetAgentID(agentID).
			SetFileName(strings.TrimSpace(file.Name)).
			SetBody(file.Body).
			SetSortOrder(sortOrder).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.ListAgentPromptFiles(ctx, agentID)
}

func (r *agentRepo) CreateAgentPromptFile(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	if f.AgentID == "" || strings.TrimSpace(f.Name) == "" {
		return biz.AgentPromptFile{}, fmt.Errorf("agent_id and name are required")
	}
	id := f.ID
	if id == "" {
		id = fmt.Sprintf("%s_%s", f.AgentID, sanitizePromptFileID(f.Name))
	}
	now := nowRFC3339()
	created, err := r.data.entClient.AgentPromptFile.Create().
		SetID(id).
		SetAgentID(f.AgentID).
		SetFileName(strings.TrimSpace(f.Name)).
		SetBody(f.Body).
		SetSortOrder(f.SortOrder).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.AgentPromptFile{}, err
	}
	return entPromptToBiz(created), nil
}

func (r *agentRepo) UpdateAgentPromptFile(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	if f.ID == "" || f.AgentID == "" {
		return biz.AgentPromptFile{}, fmt.Errorf("id and agent_id are required")
	}
	update := r.data.entClient.AgentPromptFile.UpdateOneID(f.ID).
		SetUpdatedAt(nowRFC3339())
	if strings.TrimSpace(f.Name) != "" {
		update = update.SetFileName(strings.TrimSpace(f.Name))
	}
	if f.Body != "" {
		update = update.SetBody(f.Body)
	}
	if f.SortOrder > 0 {
		update = update.SetSortOrder(f.SortOrder)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AgentPromptFile{}, sql.ErrNoRows
		}
		return biz.AgentPromptFile{}, err
	}
	return entPromptToBiz(updated), nil
}

func (r *agentRepo) DeleteAgentPromptFile(ctx context.Context, agentID, id string) error {
	if agentID == "" || id == "" {
		return fmt.Errorf("agent_id and id are required")
	}
	_, err := r.data.entClient.AgentPromptFile.Delete().
		Where(agentpromptfile.IDEQ(id), agentpromptfile.AgentIDEQ(agentID)).
		Exec(ctx)
	return err
}
