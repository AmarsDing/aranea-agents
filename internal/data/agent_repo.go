package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/organization"
	"aranea-agents/internal/data/ent/agentpromptfile"
	"aranea-agents/internal/data/ent/agentruntimesetting"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect/sql/sqlgraph"
	entsql "entgo.io/ent/dialect/sql"
)

type agentRepo struct {
	data *Data
}

var _ biz.AgentRepository = (*agentRepo)(nil)

// NewAgentRepo implements biz.AgentRepository.
func NewAgentRepo(d *Data) biz.AgentRepository {
	return &agentRepo{data: d}
}

func normalizeJSONList(value string, lg loggateway.Logger) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	b, err := json.Marshal([]string{value})
	if err != nil {
		lg.Warn("normalize json list marshal failed", loggateway.StepID("data.agent.normalize_json"), loggateway.Err(err))
		return "[]"
	}
	return string(b)
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

func entAgentToBiz(a *ent.Agent, lg loggateway.Logger) biz.Agent {
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
		IsDefault:          biz.BoolPtr(a.IsDefault),
		IsFavorite:         biz.BoolPtr(a.IsFavorite),
		Icon:               a.Icon,
		AgentDescription:   a.AgentDescription,
		PositionID:         a.PositionID,
		PositionKey:        a.PositionKey,
		AgentVariant:       a.AgentVariant,
		VariantDescription: a.VariantDescription,
		SystemPromptMode:   a.SystemPromptMode,
		ContextWindow:      a.ContextWindow,
		BudgetMonthlyCents: a.BudgetMonthlyCents,
		ConfigJSON:         a.ConfigJSON,
		CreatedBy:          a.CreatedBy,
		Readonly:           a.Readonly,
		Kind:               string(a.Kind),
		Source:             string(a.Source),
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
		DeletedAt:          a.DeletedAt,
	}
	if err := json.Unmarshal([]byte(a.RolesJSON), &agent.Roles); err != nil {
		lg.Warn("agent roles json unmarshal failed", loggateway.StepID("data.agent"), loggateway.Err(err))
	}
	return agent
}

func entRuntimeToBiz(e *ent.AgentRuntimeSetting) biz.AgentRuntimeSettings {
	if e == nil {
		return biz.AgentRuntimeSettings{}
	}
	s := &biz.AgentRuntimeSettings{
		AgentID:    e.ID,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
	s.ApplyIdentity(fromEntIdentity(e))
	s.ApplyReasoning(fromEntReasoning(e))
	s.ApplyMemory(fromEntMemory(e))
	s.ApplyTools(fromEntTools(e))
	s.ApplySkills(fromEntSkills(e))
	s.ApplyEvolution(fromEntEvolution(e))
	s.ApplyContext(fromEntContext(e))
	s.ApplyRalphLoop(fromEntRalphLoop(e))
	return *s
}

func fromEntIdentity(e *ent.AgentRuntimeSetting) biz.IdentityCfg {
	return biz.IdentityCfg{
		AgentID:               e.ID,
		ChannelID:             e.ChannelID,
		ChatID:                e.ChatID,
		Workspace:             e.Workspace,
		VariablesJSON:         e.VariablesJSON,
		ModelInstructionsJSON: e.ModelInstructionsJSON,
	}
}

func fromEntReasoning(e *ent.AgentRuntimeSetting) biz.ReasoningCfg {
	return biz.ReasoningCfg{Mode: e.ReasoningMode, Level: e.ReasoningLevel}
}

func fromEntMemory(e *ent.AgentRuntimeSetting) biz.MemoryCfg {
	return biz.MemoryCfg{
		Enabled:                  e.MemoryEnabled,
		MaxChunkLength:           e.MemoryMaxChunkLength,
		MaxResults:               e.MemoryMaxResults,
		MinScore:                 e.MemoryMinScore,
		HeartbeatEnabled:         e.HeartbeatEnabled,
		HeartbeatIntervalMinutes: e.HeartbeatIntervalMinutes,
		L0RecentWindowTurns:      e.L0RecentWindowTurns,
		L0RecentWindowTokens:     e.L0RecentWindowTokens,
		L0SummaryThreshold:       e.L0SummaryThreshold,
		L0SummaryKeepTurns:       e.L0SummaryKeepTurns,
		L0CompressMinGapSec:      e.L0CompressMinGapSec,
		L0CompressProvider:       e.L0CompressProvider,
		L0CompressModel:          e.L0CompressModel,
		MemoryWorkerProvider:     e.MemoryWorkerProvider,
		MemoryWorkerModel:        e.MemoryWorkerModel,
		L0TruncateStrategy:       e.L0TruncateStrategy,
		L0InjectL1:               e.L0InjectL1,
		L0InjectL3:               e.L0InjectL3,
		L0InjectL4:               e.L0InjectL4,
		L0L3MaxChunks:            e.L0L3MaxChunks,
		L0L4MaxPaths:             e.L0L4MaxPaths,
		L0SnapshotMode:           e.L0SnapshotMode,
		L0SnapshotEnabled:        e.L0SnapshotEnabled,
		L1Enabled:                e.L1Enabled,
		L1BudgetTokens:           e.L1BudgetTokens,
		L1FieldMaxTokens:         e.L1FieldMaxTokens,
		L1HistoryKeepRevisions:   e.L1HistoryKeepRevisions,
		L1HistoryEnabled:         e.L1HistoryEnabled,
		L1DefaultSchemaID:        e.L1DefaultSchemaID,
		L1ArchiveOnIdleMinutes:   e.L1ArchiveOnIdleMinutes,
		L2EpisodeEnabled:         e.L2EpisodeEnabled,
		L2EpisodeMinImportance:   e.L2EpisodeMinImportance,
		L2IndexEnabled:           e.L2IndexEnabled,
		L2IndexEmbeddingModel:    e.L2IndexEmbeddingModel,
		L2RecallEnabled:          e.L2RecallEnabled,
		L2RecallMax:              e.L2RecallMax,
		L2RetentionDays:          e.L2RetentionDays,
		L2ArchiveAfterDays:       e.L2ArchiveAfterDays,
		L3Enabled:                e.L3Enabled,
		L3RecallTopK:             e.L3RecallTopK,
		L3RecallMinScore:         e.L3RecallMinScore,
		L3RecallScopesJSON:       e.L3RecallScopesJSON,
		L3EmbeddingModel:         e.L3EmbeddingModel,
		L3DecayIntervalHours:     e.L3DecayIntervalHours,
		L3ArchiveThreshold:       e.L3ArchiveThreshold,
		L3MaxPerRecallChars:      e.L3MaxPerRecallChars,
		L4Enabled:                e.L4Enabled,
		L4GraphInjectNeighbors:   e.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:      e.L4GraphMaxNeighbors,
		L4GraphMaxHops:           e.L4GraphMaxHops,
		L4IdentityInject:         e.L4IdentityInject,
		L4StrategyInject:         e.L4StrategyInject,
		L4DecayIntervalHours:     e.L4DecayIntervalHours,
		L4DecayOverridesJSON:     e.L4DecayOverridesJSON,
		ForgetConfigJSON:         e.ForgetPolicyJSON,
	}
}

func fromEntTools(e *ent.AgentRuntimeSetting) biz.ToolsCfg {
	return biz.ToolsCfg{
		Enabled:                     e.ToolsEnabled,
		Profile:                     e.ToolsProfile,
		ToolCallPrefix:              e.ToolsToolCallPrefix,
		AllowJSON:                   e.ToolsAllowJSON,
		DenyJSON:                    e.ToolsDenyJSON,
		ConcurrentAllowJSON:         e.ToolsConcurrentAllowJSON,
		RetryEnabled:                e.ToolsRetryEnabled,
		RetryMaxAttempts:            e.ToolsRetryMaxAttempts,
		RetryInitialIntervalMs:      e.ToolsRetryInitialIntervalMs,
		RetryBackoffFactor:          e.ToolsRetryBackoffFactor,
		RetryMaxIntervalMs:          e.ToolsRetryMaxIntervalMs,
		RetryJitter:                 e.ToolsRetryJitter,
		ParallelEnabled:             e.ToolsParallelEnabled,
		StreamingEnabled:            e.ToolsStreamingEnabled,
		CircuitBreakerEnabled:       e.ToolsCircuitBreakerEnabled,
		CircuitBreakerOverridesJSON: e.ToolsCircuitBreakerOverridesJSON,
		DeferredJSON:                e.ToolsDeferredJSON,
		CommandSafetyEnabled:        e.ToolsCommandSafetyEnabled,
		ExecutionTimeoutSec:         e.ToolsExecutionTimeoutSec,
		ToolWeightJSON:              e.ToolWeightJSON,
	}
}

func fromEntSkills(e *ent.AgentRuntimeSetting) biz.SkillsCfg {
	return biz.SkillsCfg{
		RuntimeJSON:       e.SkillRuntimeJSON,
		LoadMode:          e.SkillLoadMode,
		IntentPassEnabled: e.IntentPassEnabled,
	}
}

func fromEntEvolution(e *ent.AgentRuntimeSetting) biz.EvolutionCfg {
	return biz.EvolutionCfg{
		SelfEvolve:                        e.SelfEvolve,
		SubagentsEnabled:                  e.SubagentsEnabled,
		SubagentsMaxConcurrency:           e.SubagentsMaxConcurrency,
		SubagentsMaxGenerationDepth:       e.SubagentsMaxGenerationDepth,
		SubagentsMaxChildrenPerAgent:      e.SubagentsMaxChildrenPerAgent,
		SubagentsArchiveAfterMinutes:      e.SubagentsArchiveAfterMinutes,
		SubagentsMaxRetries:               e.SubagentsMaxRetries,
		SubagentsModelOverride:            e.SubagentsModelOverride,
		SubagentsStoredResultRunes:        e.SubagentsStoredResultRunes,
		SubagentsStoredSummaryRunes:       e.SubagentsStoredSummaryRunes,
		SkillEvolve:                       e.EvolutionSkillEvolve,
		MetricsEnabled:                    e.EvolutionMetricsEnabled,
		SuggestionsEnabled:                e.EvolutionSuggestionsEnabled,
		GuardrailMaxChangePerPeriod:       e.GuardrailMaxChangePerPeriod,
		GuardrailMinDataPoints:            e.GuardrailMinDataPoints,
		GuardrailRollbackOnDeclinePercent: e.GuardrailRollbackOnDeclinePercent,
		EvoEnabled:                        e.EvoEnabled,
		EvoAutoApply:                      e.EvoAutoApply,
		EvoMinEpisodes:                    e.EvoMinEpisodes,
		EvoMinNegativeFeedback:            e.EvoMinNegativeFeedback,
		EvoThrottleHours:                  e.EvoThrottleHours,
		EvoProposalTTLDays:                e.EvoProposalTTLDays,
		EvoPersonaMaxChars:                e.EvoPersonaMaxChars,
		EvoSystemPromptMaxAppends:         e.EvoSystemPromptMaxAppends,
		DreamSnapshotJSON:                 e.DreamSnapshotJSON,
	}
}

func fromEntContext(e *ent.AgentRuntimeSetting) biz.ContextCfg {
	return biz.ContextCfg{
		CompactionEnabled:          e.ContextCompactionEnabled,
		MicroCompactEnabled:        e.MicroCompactEnabled,
		MemoryCompactEnabled:       e.MemoryCompactEnabled,
		ToolResultGateEnabled:      e.ToolResultGateEnabled,
		CompressLLMCacheEnabled:    e.CompressLlmCacheEnabled,
		CompressLLMCacheMaxEntries: e.CompressLlmCacheMaxEntries,
		CompressLLMCacheTTLSec:     e.CompressLlmCacheTTLSec,
		CompressionBufferRatio:     e.CompressionBufferRatio,
		SoftTriggerRatio:           e.SoftTriggerRatio,
		HardTriggerRatio:           e.HardTriggerRatio,
		SessionSummaryEnabled:      e.SessionSummaryEnabled,
		OutputSchemaJSON:           e.OutputSchemaJSON,
		ModelSelector:              e.ModelSelector,
		PlannerKind:                e.PlannerKind,
		PlannerConfigJSON:          e.PlannerConfigJSON,
		VerificationTruncateChars:  e.VerificationTruncateChars,
	}
}

func fromEntRalphLoop(e *ent.AgentRuntimeSetting) biz.RalphLoopCfg {
	return biz.RalphLoopCfg{
		MaxIterations:        e.RalphLoopMaxIterations,
		CompletionPromise:    e.RalphLoopCompletionPromise,
		VerifyCommand:        e.RalphLoopVerifyCommand,
		VerifyTimeoutSeconds: e.RalphLoopVerifyTimeoutSeconds,
		PromiseTagOpen:       e.RalphLoopPromiseTagOpen,
		PromiseTagClose:      e.RalphLoopPromiseTagClose,
		VerifyWorkDir:        e.RalphLoopVerifyWorkDir,
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

func applyBizRuntimeToCreate(b *ent.AgentRuntimeSettingCreate, v biz.AgentRuntimeSettings, lg loggateway.Logger) {
	b.SetSelfEvolve(v.SelfEvolve).
		SetSubagentsEnabled(v.SubagentsEnabled).
		SetSubagentsMaxConcurrency(v.SubagentsMaxConcurrency).
		SetSubagentsMaxGenerationDepth(v.SubagentsMaxGenerationDepth).
		SetSubagentsMaxChildrenPerAgent(v.SubagentsMaxChildrenPerAgent).
		SetSubagentsArchiveAfterMinutes(v.SubagentsArchiveAfterMinutes).
		SetSubagentsMaxRetries(v.SubagentsMaxRetries).
		SetSubagentsModelOverride(v.SubagentsModelOverride).
		SetSubagentsStoredResultRunes(v.SubagentsStoredResultRunes).
		SetSubagentsStoredSummaryRunes(v.SubagentsStoredSummaryRunes).
		SetToolsEnabled(v.ToolsEnabled).
		SetToolsProfile(v.ToolsProfile).
		SetToolsToolCallPrefix(v.ToolsToolCallPrefix).
		SetToolsAllowJSON(normalizeJSONList(v.ToolsAllowJSON, lg)).
		SetToolsDenyJSON(normalizeJSONList(v.ToolsDenyJSON, lg)).
		SetToolsConcurrentAllowJSON(normalizeJSONList(v.ToolsConcurrentAllowJSON, lg)).
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
		SetL0CompressMinGapSec(v.L0CompressMinGapSec).
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
		SetL0SnapshotEnabled(v.L0SnapshotEnabled).
		SetL1Enabled(v.L1Enabled).
		SetL1BudgetTokens(v.L1BudgetTokens).
		SetL1FieldMaxTokens(v.L1FieldMaxTokens).
		SetL1HistoryKeepRevisions(v.L1HistoryKeepRevisions).
		SetL1HistoryEnabled(v.L1HistoryEnabled).
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
		SetL3RecallScopesJSON(normalizeJSONList(v.L3RecallScopesJSON, lg)).
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
		SetL4DecayIntervalHours(v.L4DecayIntervalHours).
		SetL4DecayOverridesJSON(v.L4DecayOverridesJSON).
		SetEvoEnabled(v.EvoEnabled).
		SetEvoAutoApply(v.EvoAutoApply).
		SetEvoMinEpisodes(v.EvoMinEpisodes).
		SetEvoMinNegativeFeedback(v.EvoMinNegativeFeedback).
		SetEvoThrottleHours(v.EvoThrottleHours).
		SetEvoProposalTTLDays(v.EvoProposalTTLDays).
		SetEvoPersonaMaxChars(v.EvoPersonaMaxChars).
		SetEvoSystemPromptMaxAppends(v.EvoSystemPromptMaxAppends).
		SetSkillRuntimeJSON(normalizeJSONObj(v.SkillRuntimeJSON)).
		SetForgetPolicyJSON(normalizeJSONObj(v.ForgetConfigJSON)).
		SetToolWeightJSON(normalizeJSONObj(v.ToolWeightJSON)).
		SetDreamSnapshotJSON(v.DreamSnapshotJSON).
		SetIntentPassEnabled(v.IntentPassEnabled).
		SetChannelID(v.ChannelID).
		SetChatID(v.ChatID).
		SetWorkspace(v.Workspace).
		SetReasoningMode(v.ReasoningMode).
		SetReasoningLevel(v.ReasoningLevel).
		SetVariablesJSON(normalizeJSONObj(v.VariablesJSON)).
		SetModelInstructionsJSON(normalizeJSONObj(v.ModelInstructionsJSON)).
		SetContextCompactionEnabled(v.ContextCompactionEnabled).
		SetMicroCompactEnabled(v.MicroCompactEnabled).
		SetMemoryCompactEnabled(v.MemoryCompactEnabled).
		SetToolResultGateEnabled(v.ToolResultGateEnabled).
		SetCompressLlmCacheEnabled(v.CompressLLMCacheEnabled).
		SetCompressLlmCacheMaxEntries(v.CompressLLMCacheMaxEntries).
		SetCompressLlmCacheTTLSec(v.CompressLLMCacheTTLSec).
		SetCompressionBufferRatio(v.CompressionBufferRatio).
		SetSoftTriggerRatio(v.SoftTriggerRatio).
		SetHardTriggerRatio(v.HardTriggerRatio).
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
		SetToolsCircuitBreakerEnabled(v.ToolsCircuitBreakerEnabled).
		SetToolsCircuitBreakerOverridesJSON(v.ToolsCircuitBreakerOverridesJSON).
		SetToolsDeferredJSON(v.ToolsDeferredJSON).
		SetToolsCommandSafetyEnabled(v.ToolsCommandSafetyEnabled).
		SetToolsExecutionTimeoutSec(v.ToolsExecutionTimeoutSec).
		SetVerificationTruncateChars(v.VerificationTruncateChars).
		SetCreatedAt(v.CreatedAt).
		SetUpdatedAt(v.UpdatedAt)
}

func (r *agentRepo) categoryPositionIDsForFilter(ctx context.Context, categoryID string) ([]string, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, nil
	}
	node, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.IDEQ(categoryID),
			organization.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return []string{}, nil
		}
		return nil, entErrToBizErr(err, "AGENT")
	}
	switch node.Level {
	case "position":
		return []string{node.ID}, nil
	case "department":
		ids, err := r.data.RW().Read(ctx).Organization.Query().
			Where(
				organization.ParentIDEQ(categoryID),
				organization.LevelEQ("position"),
				organization.DeletedAtEQ(""),
			).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		return ids, nil
	case "company":
		deptIDs, err := r.data.RW().Read(ctx).Organization.Query().
			Where(
				organization.ParentIDEQ(categoryID),
				organization.LevelEQ("department"),
				organization.DeletedAtEQ(""),
			).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		if len(deptIDs) == 0 {
			return []string{}, nil
		}
		posIDs, err := r.data.RW().Read(ctx).Organization.Query().
			Where(
				organization.ParentIDIn(deptIDs...),
				organization.LevelEQ("position"),
				organization.DeletedAtEQ(""),
			).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		return posIDs, nil
	default:
		return []string{}, nil
	}
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
	if q.OrgNodeID != "" {
		positionIDs, err := r.categoryPositionIDsForFilter(ctx, q.OrgNodeID)
		if err != nil {
			return biz.AgentListResult{}, err
		}
		if len(positionIDs) == 0 {
			preds = append(preds, agent.PositionIDEQ("__no_such_category__"))
		} else if len(positionIDs) == 1 {
			preds = append(preds, agent.PositionIDEQ(positionIDs[0]))
		} else {
			preds = append(preds, agent.PositionIDIn(positionIDs...))
		}
	}
	if cb := strings.TrimSpace(q.CreatedBy); cb != "" {
		preds = append(preds, agent.CreatedByEQ(cb))
	}
	if role := strings.TrimSpace(q.Role); role != "" {
		preds = append(preds, agent.RolesJSONContains(role))
	}
	if q.Kind != "" {
		preds = append(preds, agent.KindEQ(agent.Kind(q.Kind)))
	}
	where := agent.And(preds...)
	c := r.data.RW().Read(ctx)
	total, err := c.Agent.Query().Where(where).Count(ctx)
	if err != nil {
		return biz.AgentListResult{}, entErrToBizErr(err, "AGENT")
	}
	rows, err := c.Agent.Query().Where(where).
		Order(agent.ByIsDefault(entsql.OrderDesc()), agent.ByUpdatedAt(entsql.OrderDesc())).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.AgentListResult{}, entErrToBizErr(err, "AGENT")
	}
	items := make([]biz.Agent, 0, len(rows))
	for _, row := range rows {
		items = append(items, entAgentToBiz(row, r.data.lg))
	}
	return biz.AgentListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *agentRepo) ListAgentCreators(ctx context.Context) ([]biz.AgentCreator, error) {
	rows, err := r.data.RW().Read(ctx).Agent.Query().
		Where(agent.DeletedAtEQ(""), agent.CreatedByNEQ("")).
		Select(agent.FieldCreatedBy).
		GroupBy(agent.FieldCreatedBy).
		Strings(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
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
	row, err := r.data.RW().Read(ctx).Agent.Query().Where(agent.IDEQ(id), agent.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Agent{}, shared.ErrNotFound
		}
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	return entAgentToBiz(row, r.data.lg), nil
}

func (r *agentRepo) GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return biz.Agent{}, shared.ErrNotFound
	}
	row, err := r.data.RW().Read(ctx).Agent.Query().Where(agent.AgentKeyEQ(agentKey), agent.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Agent{}, shared.ErrNotFound
		}
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	return entAgentToBiz(row, r.data.lg), nil
}

func (r *agentRepo) CreateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
	if a.AgentKey == "" || a.DisplayName == "" || a.Provider == "" || a.Model == "" {
		return biz.Agent{}, apierror.BadRequest("AGENT", "missing required fields")
	}
	if a.ID == "" {
		a.ID = generateCatalogID()
	}
	now := nowRFC3339()
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if a.Status == "" {
		a.Status = "active"
	}
	_, err := r.data.RW().Write(ctx).Agent.Create().
		SetID(a.ID).
		SetAgentKey(a.AgentKey).
		SetDisplayName(a.DisplayName).
		SetProvider(a.Provider).
		SetModel(a.Model).
		SetStatus(a.Status).
		SetIsDefault(biz.BoolVal(a.IsDefault)).
		SetIsFavorite(biz.BoolVal(a.IsFavorite)).
		SetIcon(a.Icon).
		SetAgentDescription(a.AgentDescription).
		SetPositionID(a.PositionID).
		SetPositionKey(a.PositionKey).
		SetAgentVariant(a.AgentVariant).
		SetVariantDescription(a.VariantDescription).
		SetSystemPromptMode(a.SystemPromptMode).
		SetContextWindow(a.ContextWindow).
		SetBudgetMonthlyCents(a.BudgetMonthlyCents).
		SetConfigJSON(a.ConfigJSON).
		SetRolesJSON(mustMarshalString(a.Roles)).
		SetCreatedBy(a.CreatedBy).
		SetReadonly(a.Readonly).
		SetKind(agent.Kind(a.Kind)).
		SetSource(agent.Source(a.Source)).
		SetCreatedAt(a.CreatedAt).
		SetUpdatedAt(a.UpdatedAt).
		SetDeletedAt(a.DeletedAt).
		Save(ctx)
	if err != nil {
		if sqlgraph.IsConstraintError(err) {
			return biz.Agent{}, shared.ErrAgentKeyConflict
		}
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	return r.GetAgentByID(ctx, a.ID)
}

func (r *agentRepo) UpdateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
	if a.ID == "" {
		return biz.Agent{}, apierror.BadRequest("AGENT", "id is required")
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
	_, err = r.data.RW().Write(ctx).Agent.UpdateOneID(a.ID).
		SetDisplayName(a.DisplayName).
		SetProvider(a.Provider).
		SetModel(a.Model).
		SetStatus(a.Status).
		SetIsDefault(biz.BoolVal(a.IsDefault)).
		SetIsFavorite(biz.BoolVal(a.IsFavorite)).
		SetIcon(a.Icon).
		SetAgentDescription(a.AgentDescription).
		SetPositionID(a.PositionID).
		SetPositionKey(a.PositionKey).
		SetAgentVariant(a.AgentVariant).
		SetVariantDescription(a.VariantDescription).
		SetSystemPromptMode(a.SystemPromptMode).
		SetContextWindow(a.ContextWindow).
		SetBudgetMonthlyCents(a.BudgetMonthlyCents).
		SetConfigJSON(a.ConfigJSON).
		SetRolesJSON(mustMarshalString(a.Roles)).
		SetReadonly(a.Readonly).
		SetKind(agent.Kind(a.Kind)).
		SetSource(agent.Source(a.Source)).
		SetUpdatedAt(a.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	out, err := r.GetAgentByID(ctx, a.ID)
	return out, err
}

func (r *agentRepo) DeleteAgent(ctx context.Context, id string) error {
	if id == "" {
		return apierror.BadRequest("AGENT", "id is required")
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := nowRFC3339()
		if _, err := r.data.RW().Write(txCtx).Agent.UpdateOneID(id).
			SetDeletedAt(now).
			SetStatus("deleted").
			SetUpdatedAt(now).
			Save(txCtx); err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		return cascadeDeleteByAgent(txCtx, r.data, id)
	})
}

// ToggleFavorite atomically flips the is_favorite flag using
// UPDATE agents SET is_favorite = NOT is_favorite WHERE id = ?,
// then reads back the updated row. This avoids the read-then-write
// race condition where two concurrent requests could both read the
// same value and flip to the same result.
func (r *agentRepo) ToggleFavorite(ctx context.Context, id string) (biz.Agent, error) {
	if id == "" {
		return biz.Agent{}, apierror.BadRequest("AGENT", "id is required")
	}
	now := nowRFC3339()
	result, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders("UPDATE agents SET is_favorite = NOT is_favorite, updated_at = ? WHERE id = ? AND deleted_at = ''"),
		now, id,
	)
	if err != nil {
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	if affected == 0 {
		return biz.Agent{}, shared.ErrNotFound
	}
	return r.GetAgentByID(ctx, id)
}

func (r *agentRepo) GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	row, err := r.data.RW().Read(ctx).AgentRuntimeSetting.Get(ctx, agentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AgentRuntimeSettings{}, shared.ErrNotFound
		}
		return biz.AgentRuntimeSettings{}, entErrToBizErr(err, "AGENT")
	}
	return entRuntimeToBiz(row), nil
}

func (r *agentRepo) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	if v.AgentID == "" {
		return biz.AgentRuntimeSettings{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	now := nowRFC3339()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	b := r.data.RW().Write(ctx).AgentRuntimeSetting.Create().SetID(v.AgentID)
	applyBizRuntimeToCreate(b, v, r.data.lg)
	if err := b.OnConflict(
		entsql.ConflictColumns(agentruntimesetting.FieldID),
		entsql.ResolveWithNewValues(),
	).Exec(ctx); err != nil {
		return biz.AgentRuntimeSettings{}, entErrToBizErr(err, "AGENT")
	}
	row, err := r.data.RW().Write(ctx).AgentRuntimeSetting.Get(ctx, v.AgentID)
	if err != nil {
		return biz.AgentRuntimeSettings{}, entErrToBizErr(err, "AGENT")
	}
	return entRuntimeToBiz(row), nil
}

func (r *agentRepo) ListAgentPromptFiles(ctx context.Context, agentID string) ([]biz.AgentPromptFile, error) {
	rows, err := r.data.RW().Read(ctx).AgentPromptFile.Query().
		Where(agentpromptfile.AgentIDEQ(agentID)).
		Order(agentpromptfile.BySortOrder(), agentpromptfile.ByFileName()).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	out := make([]biz.AgentPromptFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, entPromptToBiz(row))
	}
	return out, nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	if agentID == "" {
		return nil, apierror.BadRequest("AGENT", "agent id is required")
	}
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := r.data.RW().Write(txCtx).AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(txCtx); err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		now := nowRFC3339()
		builders := make([]*ent.AgentPromptFileCreate, 0, len(files))
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
			builders = append(builders, r.data.RW().Write(txCtx).AgentPromptFile.Create().
				SetID(id).
				SetAgentID(agentID).
				SetFileName(strings.TrimSpace(file.Name)).
				SetBody(file.Body).
				SetSortOrder(sortOrder).
				SetCreatedAt(now).
				SetUpdatedAt(now))
		}
		if len(builders) > 0 {
			if _, err := r.data.RW().Write(txCtx).AgentPromptFile.CreateBulk(builders...).Save(txCtx); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out, err := r.ListAgentPromptFiles(ctx, agentID)
	return out, err
}

// --- 高阶原子化方法（供 pack.Importer 使用）---

// CreateAgentAtomic 创建 agent + 写入 prompt files + upsert runtime settings，
// 三步包在同一个 ExecInTx 中，任意一步失败则整体回滚。
// 比 Pack 手工调用 CreateAgent + ReplaceFiles + UpsertSettings 强一档。
func (r *agentRepo) CreateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error) {
	var created biz.Agent
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		var err error
		created, err = r.CreateAgent(txCtx, a)
		if err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		settings.AgentID = created.ID
		if _, err = r.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		if len(files) > 0 {
			if _, err = r.ReplaceAgentPromptFiles(txCtx, created.ID, files); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		return nil
	})
	return created, err
}

// UpdateAgentAtomic 覆盖 agent + 替换 prompt files + upsert runtime settings，
// 三步包在同一个 ExecInTx 中，任意一步失败则整体回滚到调用前状态。
// 对应 Pack 场景下的 ConflictOverwrite 路径；之前的"半新半旧"问题由此修复。
func (r *agentRepo) UpdateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error) {
	var updated biz.Agent
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		var err error
		updated, err = r.UpdateAgent(txCtx, a)
		if err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		if settings != nil {
			if _, err = r.UpsertAgentRuntimeSettings(txCtx, *settings); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		if files != nil {
			if _, err = r.ReplaceAgentPromptFiles(txCtx, updated.ID, files); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		return nil
	})
	return updated, err
}

func (r *agentRepo) CreateAgentPromptFile(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	if f.AgentID == "" || strings.TrimSpace(f.Name) == "" {
		return biz.AgentPromptFile{}, apierror.BadRequest("AGENT", "agent_id and name are required")
	}
	id := f.ID
	if id == "" {
		id = fmt.Sprintf("%s_%s", f.AgentID, sanitizePromptFileID(f.Name))
	}
	now := nowRFC3339()
	created, err := r.data.RW().Write(ctx).AgentPromptFile.Create().
		SetID(id).
		SetAgentID(f.AgentID).
		SetFileName(strings.TrimSpace(f.Name)).
		SetBody(f.Body).
		SetSortOrder(f.SortOrder).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.AgentPromptFile{}, entErrToBizErr(err, "AGENT")
	}
	return entPromptToBiz(created), nil
}

func (r *agentRepo) UpdateAgentPromptFile(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	if f.ID == "" || f.AgentID == "" {
		return biz.AgentPromptFile{}, apierror.BadRequest("AGENT", "id and agent_id are required")
	}
	update := r.data.RW().Write(ctx).AgentPromptFile.UpdateOneID(f.ID).
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
			return biz.AgentPromptFile{}, shared.ErrNotFound
		}
		return biz.AgentPromptFile{}, entErrToBizErr(err, "AGENT")
	}
	return entPromptToBiz(updated), nil
}

func (r *agentRepo) DeleteAgentPromptFile(ctx context.Context, agentID, id string) error {
	if agentID == "" || id == "" {
		return apierror.BadRequest("AGENT", "agent_id and id are required")
	}
	_, err := r.data.RW().Write(ctx).AgentPromptFile.Delete().
		Where(agentpromptfile.IDEQ(id), agentpromptfile.AgentIDEQ(agentID)).
		Exec(ctx)
	return entErrToBizErr(err, "AGENT")
}

func (r *agentRepo) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.data.ExecInTx(ctx, fn)
}

func (r *agentRepo) ReorderAgents(ctx context.Context, ids []string) error {
	return nil
}

// CountAgentsByProviderAndModel counts agents referencing a given provider+model.
func (r *agentRepo) CountAgentsByProviderAndModel(ctx context.Context, provider, model string) (int, error) {
	if r.data == nil {
		return 0, nil
	}
	n, err := r.data.RW().Read(ctx).Agent.Query().
		Where(agent.ProviderEQ(provider), agent.ModelEQ(model), agent.DeletedAtEQ("")).
		Count(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "AGENT")
	}
	return n, nil
}

// ClearPositionByDepartment clears the position_id field for all agents
// whose position belongs to the given department. Used during department deletion cascade.
func (r *agentRepo) ClearPositionByDepartment(ctx context.Context, deptID string) (int, error) {
	if deptID == "" || r.data == nil {
		return 0, nil
	}
	// Find all position IDs under this department
	positions, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.ParentIDEQ(deptID),
			organization.LevelEQ("position"),
			organization.DeletedAtEQ(""),
		).
		All(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "AGENT")
	}
	if len(positions) == 0 {
		return 0, nil
	}
	positionIDs := make([]string, 0, len(positions))
	for _, p := range positions {
		positionIDs = append(positionIDs, p.ID)
	}
	// Clear position_id for agents in those positions
	n, err := r.data.RW().Write(ctx).Agent.Update().
		Where(
			agent.PositionIDIn(positionIDs...),
			agent.DeletedAtEQ(""),
		).
		SetPositionID("").
		Save(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "AGENT")
	}
	return n, nil
}

// mustMarshalString serializes v to a JSON string. Returns "[]" on nil or error.
func mustMarshalString(v any) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
