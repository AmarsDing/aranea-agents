package data

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

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
		WorkspaceID:        a.WorkspaceID,
		MissionStatement:   a.MissionStatement,
		DomainPath:         a.DomainPath,
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
		AgentID:   e.ID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	s.ApplyIdentity(fromEntIdentity(e))
	s.ApplyReasoning(fromEntReasoning(e))
	s.ApplyMemory(fromEntMemory(e))
	s.ApplyTools(fromEntTools(e))
	s.ApplySkills(fromEntSkills(e))
	s.ApplyEvolution(fromEntEvolution(e))
	s.ApplyContext(fromEntContext(e))
	s.ApplyRalphLoop(fromEntRalphLoop(e))
	// Direct fields not owned by any sub-cfg.
	s.CodeExecutorType = e.CodeExecutorType
	s.MaxLLMCalls = e.MaxLlmCalls
	s.MaxToolIterations = e.MaxToolIterations
	s.LoopGuardToolLoadMax = e.LoopGuardToolLoadMax
	s.LoopGuardWallSoftSec = e.LoopGuardWallSoftSec
	s.LoopGuardWallHardSec = e.LoopGuardWallHardSec
	s.CompressionBufferAdaptive = e.CompressionBufferAdaptive
	s.EnableTokenTailoring = e.EnableTokenTailoring
	s.TokenTailoringStrategy = e.TokenTailoringStrategy
	s.TokenTailoringSafetyMargin = e.TokenTailoringSafetyMargin
	s.ReplyReminderEnabled = e.ReplyReminderEnabled
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
		L3RecallBudgetTokens:     e.L3RecallBudgetTokens,
		L2RecallBudgetTokens:     e.L2RecallBudgetTokens,
		L3InjectProvenance:       e.L3InjectProvenance,
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
		IntentSkipEnabled: e.IntentSkipEnabled,
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
		MemoryCompactEnabled:       e.MemoryCompactEnabled,
		ToolResultGateEnabled:      e.ToolResultGateEnabled,
		CompressLLMCacheEnabled:    e.CompressLlmCacheEnabled,
		CompressLLMCacheMaxEntries: e.CompressLlmCacheMaxEntries,
		CompressLLMCacheTTLSec:     e.CompressLlmCacheTTLSec,
		CompressionBufferRatio:     e.CompressionBufferRatio,
		SoftTriggerRatio:           e.SoftTriggerRatio,
		HardTriggerRatio:           e.HardTriggerRatio,
		AssemblyBudgetSoftTokens:   e.AssemblyBudgetSoftTokens,
		AssemblyBudgetHardTokens:   e.AssemblyBudgetHardTokens,
		SessionSummaryEnabled:      e.SessionSummaryEnabled,
		OutputSchemaJSON:           e.OutputSchemaJSON,
		ModelSelector:              e.ModelSelector,
		PlannerKind:                e.PlannerKind,
		PlannerConfigJSON:          e.PlannerConfigJSON,
		VerificationTruncateChars:  e.VerificationTruncateChars,
		ClarificationEnabled:       e.ClarificationEnabled,
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
		SetL3RecallBudgetTokens(v.L3RecallBudgetTokens).
		SetL2RecallBudgetTokens(v.L2RecallBudgetTokens).
		SetL3InjectProvenance(v.L3InjectProvenance).
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
		SetIntentSkipEnabled(v.IntentSkipEnabled).
		SetClarificationEnabled(v.ClarificationEnabled).
		SetReplyReminderEnabled(v.ReplyReminderEnabled).
		SetChannelID(v.ChannelID).
		SetChatID(v.ChatID).
		SetWorkspace(v.Workspace).
		SetReasoningMode(v.ReasoningMode).
		SetReasoningLevel(v.ReasoningLevel).
		SetVariablesJSON(normalizeJSONObj(v.VariablesJSON)).
		SetModelInstructionsJSON(normalizeJSONObj(v.ModelInstructionsJSON)).
		SetContextCompactionEnabled(v.ContextCompactionEnabled).
		SetMemoryCompactEnabled(v.MemoryCompactEnabled).
		SetToolResultGateEnabled(v.ToolResultGateEnabled).
		SetCompressLlmCacheEnabled(v.CompressLLMCacheEnabled).
		SetCompressLlmCacheMaxEntries(v.CompressLLMCacheMaxEntries).
		SetCompressLlmCacheTTLSec(v.CompressLLMCacheTTLSec).
		SetCompressionBufferRatio(v.CompressionBufferRatio).
		SetCompressionBufferAdaptive(v.CompressionBufferAdaptive).
		SetSoftTriggerRatio(v.SoftTriggerRatio).
		SetHardTriggerRatio(v.HardTriggerRatio).
		SetAssemblyBudgetSoftTokens(v.AssemblyBudgetSoftTokens).
		SetAssemblyBudgetHardTokens(v.AssemblyBudgetHardTokens).
		SetSessionSummaryEnabled(v.SessionSummaryEnabled).
		SetSkillLoadMode(v.SkillLoadMode).
		SetCodeExecutorType(v.CodeExecutorType).
		SetMaxLlmCalls(v.MaxLLMCalls).
		SetMaxToolIterations(v.MaxToolIterations).
		SetEnableTokenTailoring(v.EnableTokenTailoring).
		SetTokenTailoringStrategy(v.TokenTailoringStrategy).
		SetTokenTailoringSafetyMargin(v.TokenTailoringSafetyMargin).
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
		SetLoopGuardToolLoadMax(v.LoopGuardToolLoadMax).
		SetLoopGuardWallSoftSec(v.LoopGuardWallSoftSec).
		SetLoopGuardWallHardSec(v.LoopGuardWallHardSec).
		SetVerificationTruncateChars(v.VerificationTruncateChars).
		SetCreatedAt(v.CreatedAt).
		SetUpdatedAt(v.UpdatedAt)
}
