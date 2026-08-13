package service

import (
	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"

	"google.golang.org/protobuf/proto"
)

func fromProtoRuntime(pb *v1.AgentRuntimeSettings) *biz.AgentRuntimeSettings {
	if pb == nil {
		return nil
	}
	s := &biz.AgentRuntimeSettings{
		AgentID:                       pb.GetAgentId(),
		CreatedAt:                     pb.GetCreatedAt(),
		UpdatedAt:                     pb.GetUpdatedAt(),
		CodeExecutorType:              pb.GetCodeExecutorType(),
		RalphLoopMaxIterations:        int(pb.GetRalphLoopMaxIterations()),
		RalphLoopCompletionPromise:    pb.GetRalphLoopCompletionPromise(),
		RalphLoopVerifyCommand:        pb.GetRalphLoopVerifyCommand(),
		RalphLoopVerifyTimeoutSeconds: int(pb.GetRalphLoopVerifyTimeoutSeconds()),
		RalphLoopPromiseTagOpen:       pb.GetRalphLoopPromiseTagOpen(),
		RalphLoopPromiseTagClose:      pb.GetRalphLoopPromiseTagClose(),
		RalphLoopVerifyWorkDir:        pb.GetRalphLoopVerifyWorkDir(),
	}
	s.ApplyIdentity(fromProtoIdentity(pb))
	s.ApplyReasoning(fromProtoReasoning(pb))
	s.ApplyMemory(fromProtoMemory(pb))
	s.ApplyTools(fromProtoTools(pb))
	s.ApplySkills(fromProtoSkills(pb))
	s.ApplyEvolution(fromProtoEvolution(pb))
	s.ApplyContext(fromProtoContext(pb))
	return s
}

func fromProtoIdentity(pb *v1.AgentRuntimeSettings) biz.IdentityCfg {
	return biz.IdentityCfg{
		AgentID:               pb.GetAgentId(),
		ChannelID:             pb.GetChannelId(),
		ChatID:                pb.GetChatId(),
		Workspace:             pb.GetWorkspace(),
		VariablesJSON:         pb.GetVariablesJson(),
		ModelInstructionsJSON: pb.GetModelInstructionsJson(),
	}
}

func fromProtoReasoning(pb *v1.AgentRuntimeSettings) biz.ReasoningCfg {
	return biz.ReasoningCfg{
		Mode:  pb.GetReasoningMode(),
		Level: pb.GetReasoningLevel(),
	}
}

func fromProtoMemory(pb *v1.AgentRuntimeSettings) biz.MemoryCfg {
	return biz.MemoryCfg{
		Enabled:                  pb.GetMemoryEnabled(),
		MaxChunkLength:           int(pb.GetMemoryMaxChunkLength()),
		MaxResults:               int(pb.GetMemoryMaxResults()),
		MinScore:                 pb.GetMemoryMinScore(),
		HeartbeatEnabled:         pb.GetHeartbeatEnabled(),
		HeartbeatIntervalMinutes: int(pb.GetHeartbeatIntervalMinutes()),
		L0RecentWindowTurns:      int(pb.GetL0RecentWindowTurns()),
		L0RecentWindowTokens:     int(pb.GetL0RecentWindowTokens()),
		L0SummaryThreshold:       pb.GetL0SummaryThreshold(),
		L0SummaryKeepTurns:       int(pb.GetL0SummaryKeepTurns()),
		L0CompressProvider:       pb.GetL0CompressProvider(),
		L0CompressModel:          pb.GetL0CompressModel(),
		MemoryWorkerProvider:     pb.GetMemoryWorkerProvider(),
		MemoryWorkerModel:        pb.GetMemoryWorkerModel(),
		L0TruncateStrategy:       pb.GetL0TruncateStrategy(),
		L0InjectL1:               pb.GetL0InjectL1(),
		L0InjectL3:               pb.GetL0InjectL3(),
		L0InjectL4:               pb.GetL0InjectL4(),
		L0L3MaxChunks:            int(pb.GetL0L3MaxChunks()),
		L0L4MaxPaths:             int(pb.GetL0L4MaxPaths()),
		L0SnapshotMode:           pb.GetL0SnapshotMode(),
		L0SnapshotEnabled:        pb.GetL0SnapshotEnabled(),
		L1Enabled:                pb.GetL1Enabled(),
		L1BudgetTokens:           int(pb.GetL1BudgetTokens()),
		L1FieldMaxTokens:         int(pb.GetL1FieldMaxTokens()),
		L1HistoryKeepRevisions:   int(pb.GetL1HistoryKeepRevisions()),
		L1HistoryEnabled:         pb.GetL1HistoryEnabled(),
		L1DefaultSchemaID:        pb.GetL1DefaultSchemaId(),
		L1ArchiveOnIdleMinutes:   int(pb.GetL1ArchiveOnIdleMinutes()),
		L2EpisodeEnabled:         pb.GetL2EpisodeEnabled(),
		L2EpisodeMinImportance:   pb.GetL2EpisodeMinImportance(),
		L2IndexEnabled:           pb.GetL2IndexEnabled(),
		L2IndexEmbeddingModel:    pb.GetL2IndexEmbeddingModel(),
		L2RecallEnabled:          pb.GetL2RecallEnabled(),
		L2RecallMax:              int(pb.GetL2RecallMax()),
		L2RetentionDays:          int(pb.GetL2RetentionDays()),
		L2ArchiveAfterDays:       int(pb.GetL2ArchiveAfterDays()),
		L3Enabled:                pb.GetL3Enabled(),
		L3RecallTopK:             int(pb.GetL3RecallTopK()),
		L3RecallMinScore:         pb.GetL3RecallMinScore(),
		L3RecallScopesJSON:       pb.GetL3RecallScopesJson(),
		L3EmbeddingModel:         pb.GetL3EmbeddingModel(),
		L3DecayIntervalHours:     int(pb.GetL3DecayIntervalHours()),
		L3ArchiveThreshold:       pb.GetL3ArchiveThreshold(),
		L3MaxPerRecallChars:      int(pb.GetL3MaxPerRecallChars()),
		L3RecallBudgetTokens:     int(pb.GetL3RecallBudgetTokens()),
		L4Enabled:                pb.GetL4Enabled(),
		L4GraphInjectNeighbors:   pb.GetL4GraphInjectNeighbors(),
		L4GraphMaxNeighbors:      int(pb.GetL4GraphMaxNeighbors()),
		L4GraphMaxHops:           int(pb.GetL4GraphMaxHops()),
		L4IdentityInject:         pb.GetL4IdentityInject(),
		L4StrategyInject:         pb.GetL4StrategyInject(),
	}
}

func fromProtoTools(pb *v1.AgentRuntimeSettings) biz.ToolsCfg {
	return biz.ToolsCfg{
		Enabled:                     pb.GetToolsEnabled(),
		Profile:                     pb.GetToolsProfile(),
		ToolCallPrefix:              pb.GetToolsToolCallPrefix(),
		AllowJSON:                   pb.GetToolsAllowJson(),
		DenyJSON:                    pb.GetToolsDenyJson(),
		ConcurrentAllowJSON:         pb.GetToolsConcurrentAllowJson(),
		RetryEnabled:                pb.GetToolsRetryEnabled(),
		RetryMaxAttempts:            int(pb.GetToolsRetryMaxAttempts()),
		RetryInitialIntervalMs:      int(pb.GetToolsRetryInitialIntervalMs()),
		RetryBackoffFactor:          pb.GetToolsRetryBackoffFactor(),
		RetryMaxIntervalMs:          int(pb.GetToolsRetryMaxIntervalMs()),
		RetryJitter:                 pb.GetToolsRetryJitter(),
		ParallelEnabled:             pb.GetToolsParallelEnabled(),
		StreamingEnabled:            pb.GetToolsStreamingEnabled(),
		CircuitBreakerEnabled:       pb.GetToolsCircuitBreakerEnabled(),
		CircuitBreakerOverridesJSON: pb.GetToolsCircuitBreakerOverridesJson(),
		DeferredJSON:                pb.GetToolsDeferredJson(),
		CommandSafetyEnabled:        pb.GetToolsCommandSafetyEnabled(),
	}
}

func fromProtoSkills(pb *v1.AgentRuntimeSettings) biz.SkillsCfg {
	// P1-1: intent_pass absent → default true (matches DefaultAgentRuntimeSettings and Ent schema default).
	// Explicit true/false from caller is respected.
	intentPass := true
	if pb.IntentPassEnabled != nil {
		intentPass = *pb.IntentPassEnabled
	}
	return biz.SkillsCfg{
		RuntimeJSON:       pb.GetSkillRuntimeJson(),
		LoadMode:          pb.GetSkillLoadMode(),
		IntentPassEnabled: intentPass,
	}
}

func fromProtoEvolution(pb *v1.AgentRuntimeSettings) biz.EvolutionCfg {
	return biz.EvolutionCfg{
		SelfEvolve:                        pb.GetSelfEvolve(),
		SubagentsEnabled:                  pb.GetSubagentsEnabled(),
		SubagentsMaxConcurrency:           int(pb.GetSubagentsMaxConcurrency()),
		SubagentsMaxGenerationDepth:       int(pb.GetSubagentsMaxGenerationDepth()),
		SubagentsMaxChildrenPerAgent:      int(pb.GetSubagentsMaxChildrenPerAgent()),
		SubagentsArchiveAfterMinutes:      int(pb.GetSubagentsArchiveAfterMinutes()),
		SubagentsMaxRetries:               int(pb.GetSubagentsMaxRetries()),
		SubagentsModelOverride:            pb.GetSubagentsModelOverride(),
		SkillEvolve:                       pb.GetEvolutionSkillEvolve(),
		MetricsEnabled:                    pb.GetEvolutionMetricsEnabled(),
		SuggestionsEnabled:                pb.GetEvolutionSuggestionsEnabled(),
		GuardrailMaxChangePerPeriod:       pb.GetGuardrailMaxChangePerPeriod(),
		GuardrailMinDataPoints:            int(pb.GetGuardrailMinDataPoints()),
		GuardrailRollbackOnDeclinePercent: int(pb.GetGuardrailRollbackOnDeclinePercent()),
		EvoEnabled:                        pb.GetEvoEnabled(),
		EvoAutoApply:                      pb.GetEvoAutoApply(),
		EvoMinEpisodes:                    int(pb.GetEvoMinEpisodes()),
		EvoMinNegativeFeedback:            int(pb.GetEvoMinNegativeFeedback()),
		EvoThrottleHours:                  int(pb.GetEvoThrottleHours()),
		EvoProposalTTLDays:                int(pb.GetEvoProposalTtlDays()),
		EvoPersonaMaxChars:                int(pb.GetEvoPersonaMaxChars()),
		EvoSystemPromptMaxAppends:         int(pb.GetEvoSystemPromptMaxAppends()),
	}
}

func fromProtoContext(pb *v1.AgentRuntimeSettings) biz.ContextCfg {
	return biz.ContextCfg{
		CompactionEnabled:          pb.GetContextCompactionEnabled(),
		MemoryCompactEnabled:       pb.GetMemoryCompactEnabled(),
		ToolResultGateEnabled:      pb.GetToolResultGateEnabled(),
		CompressLLMCacheEnabled:    pb.GetCompressLlmCacheEnabled(),
		CompressLLMCacheMaxEntries: int(pb.GetCompressLlmCacheMaxEntries()),
		CompressLLMCacheTTLSec:     int(pb.GetCompressLlmCacheTtlSec()),
		CompressionBufferRatio:     pb.GetCompressionBufferRatio(),
		SoftTriggerRatio:           pb.GetSoftTriggerRatio(),
		HardTriggerRatio:           pb.GetHardTriggerRatio(),
		SessionSummaryEnabled:      pb.GetSessionSummaryEnabled(),
		OutputSchemaJSON:           pb.GetOutputSchemaJson(),
		ModelSelector:              pb.GetModelSelector(),
		PlannerKind:                pb.GetPlannerKind(),
		PlannerConfigJSON:          pb.GetPlannerConfigJson(),
	}
}

func toProtoRuntime(b *biz.AgentRuntimeSettings) *v1.AgentRuntimeSettings {
	if b == nil {
		return nil
	}
	id := b.GetIdentity()
	rsn := b.GetReasoning()
	mem := b.GetMemory()
	tools := b.GetTools()
	skills := b.GetSkills()
	evo := b.GetEvolution()
	ctx := b.GetContext()
	return &v1.AgentRuntimeSettings{
		AgentId:                           id.AgentID,
		SelfEvolve:                        evo.SelfEvolve,
		SubagentsEnabled:                  evo.SubagentsEnabled,
		SubagentsMaxConcurrency:           int32(evo.SubagentsMaxConcurrency),
		SubagentsMaxGenerationDepth:       int32(evo.SubagentsMaxGenerationDepth),
		SubagentsMaxChildrenPerAgent:      int32(evo.SubagentsMaxChildrenPerAgent),
		SubagentsArchiveAfterMinutes:      int32(evo.SubagentsArchiveAfterMinutes),
		SubagentsMaxRetries:               int32(evo.SubagentsMaxRetries),
		SubagentsModelOverride:            evo.SubagentsModelOverride,
		ToolsEnabled:                      tools.Enabled,
		ToolsProfile:                      tools.Profile,
		ToolsToolCallPrefix:               tools.ToolCallPrefix,
		ToolsAllowJson:                    tools.AllowJSON,
		ToolsDenyJson:                     tools.DenyJSON,
		ToolsConcurrentAllowJson:          tools.ConcurrentAllowJSON,
		MemoryEnabled:                     mem.Enabled,
		MemoryMaxChunkLength:              int32(mem.MaxChunkLength),
		MemoryMaxResults:                  int32(mem.MaxResults),
		MemoryMinScore:                    mem.MinScore,
		HeartbeatEnabled:                  mem.HeartbeatEnabled,
		HeartbeatIntervalMinutes:          int32(mem.HeartbeatIntervalMinutes),
		EvolutionSelfEvolve:               evo.SelfEvolve,
		EvolutionSkillEvolve:              evo.SkillEvolve,
		EvolutionMetricsEnabled:           evo.MetricsEnabled,
		EvolutionSuggestionsEnabled:       evo.SuggestionsEnabled,
		GuardrailMaxChangePerPeriod:       evo.GuardrailMaxChangePerPeriod,
		GuardrailMinDataPoints:            int32(evo.GuardrailMinDataPoints),
		GuardrailRollbackOnDeclinePercent: int32(evo.GuardrailRollbackOnDeclinePercent),
		L0RecentWindowTurns:               int32(mem.L0RecentWindowTurns),
		L0RecentWindowTokens:              int32(mem.L0RecentWindowTokens),
		L0SummaryThreshold:                mem.L0SummaryThreshold,
		L0SummaryKeepTurns:                int32(mem.L0SummaryKeepTurns),
		L0CompressProvider:                mem.L0CompressProvider,
		L0CompressModel:                   mem.L0CompressModel,
		MemoryWorkerProvider:              mem.MemoryWorkerProvider,
		MemoryWorkerModel:                 mem.MemoryWorkerModel,
		L0TruncateStrategy:                mem.L0TruncateStrategy,
		L0InjectL1:                        mem.L0InjectL1,
		L0InjectL3:                        mem.L0InjectL3,
		L0InjectL4:                        mem.L0InjectL4,
		L0L3MaxChunks:                     int32(mem.L0L3MaxChunks),
		L0L4MaxPaths:                      int32(mem.L0L4MaxPaths),
		L0SnapshotMode:                    mem.L0SnapshotMode,
		L0SnapshotEnabled:                 mem.L0SnapshotEnabled,
		L1Enabled:                         mem.L1Enabled,
		L1BudgetTokens:                    int32(mem.L1BudgetTokens),
		L1FieldMaxTokens:                  int32(mem.L1FieldMaxTokens),
		L1HistoryKeepRevisions:            int32(mem.L1HistoryKeepRevisions),
		L1HistoryEnabled:                  mem.L1HistoryEnabled,
		L1DefaultSchemaId:                 mem.L1DefaultSchemaID,
		L1ArchiveOnIdleMinutes:            int32(mem.L1ArchiveOnIdleMinutes),
		L2EpisodeEnabled:                  mem.L2EpisodeEnabled,
		L2EpisodeMinImportance:            mem.L2EpisodeMinImportance,
		L2IndexEnabled:                    mem.L2IndexEnabled,
		L2IndexEmbeddingModel:             mem.L2IndexEmbeddingModel,
		L2RecallEnabled:                   mem.L2RecallEnabled,
		L2RecallMax:                       int32(mem.L2RecallMax),
		L2RetentionDays:                   int32(mem.L2RetentionDays),
		L2ArchiveAfterDays:                int32(mem.L2ArchiveAfterDays),
		L3Enabled:                         mem.L3Enabled,
		L3RecallTopK:                      int32(mem.L3RecallTopK),
		L3RecallMinScore:                  mem.L3RecallMinScore,
		L3RecallScopesJson:                mem.L3RecallScopesJSON,
		L3EmbeddingModel:                  mem.L3EmbeddingModel,
		L3DecayIntervalHours:              int32(mem.L3DecayIntervalHours),
		L3ArchiveThreshold:                mem.L3ArchiveThreshold,
		L3MaxPerRecallChars:               int32(mem.L3MaxPerRecallChars),
		L3RecallBudgetTokens:              int32(mem.L3RecallBudgetTokens),
		L4Enabled:                         mem.L4Enabled,
		L4GraphInjectNeighbors:            mem.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:               int32(mem.L4GraphMaxNeighbors),
		L4GraphMaxHops:                    int32(mem.L4GraphMaxHops),
		L4IdentityInject:                  mem.L4IdentityInject,
		L4StrategyInject:                  mem.L4StrategyInject,
		EvoEnabled:                        evo.EvoEnabled,
		EvoAutoApply:                      evo.EvoAutoApply,
		EvoMinEpisodes:                    int32(evo.EvoMinEpisodes),
		EvoMinNegativeFeedback:            int32(evo.EvoMinNegativeFeedback),
		EvoThrottleHours:                  int32(evo.EvoThrottleHours),
		EvoProposalTtlDays:                int32(evo.EvoProposalTTLDays),
		EvoPersonaMaxChars:                int32(evo.EvoPersonaMaxChars),
		EvoSystemPromptMaxAppends:         int32(evo.EvoSystemPromptMaxAppends),
		CreatedAt:                         b.CreatedAt,
		UpdatedAt:                         b.UpdatedAt,
		SkillRuntimeJson:                  skills.RuntimeJSON,
		IntentPassEnabled:                 proto.Bool(skills.IntentPassEnabled),
		ChannelId:                         id.ChannelID,
		ChatId:                            id.ChatID,
		Workspace:                         id.Workspace,
		ReasoningMode:                     rsn.Mode,
		ReasoningLevel:                    rsn.Level,
		VariablesJson:                     id.VariablesJSON,
		ModelInstructionsJson:             id.ModelInstructionsJSON,
		ContextCompactionEnabled:          ctx.CompactionEnabled,
		MemoryCompactEnabled:              ctx.MemoryCompactEnabled,
		ToolResultGateEnabled:             ctx.ToolResultGateEnabled,
		CompressLlmCacheEnabled:           ctx.CompressLLMCacheEnabled,
		CompressLlmCacheMaxEntries:        int32(ctx.CompressLLMCacheMaxEntries),
		CompressLlmCacheTtlSec:            int32(ctx.CompressLLMCacheTTLSec),
		CompressionBufferRatio:            ctx.CompressionBufferRatio,
		SoftTriggerRatio:                  ctx.SoftTriggerRatio,
		HardTriggerRatio:                  ctx.HardTriggerRatio,
		SessionSummaryEnabled:             ctx.SessionSummaryEnabled,
		SkillLoadMode:                     skills.LoadMode,
		CodeExecutorType:                  b.CodeExecutorType,
		OutputSchemaJson:                  ctx.OutputSchemaJSON,
		ModelSelector:                     ctx.ModelSelector,
		ToolsRetryEnabled:                 tools.RetryEnabled,
		ToolsRetryMaxAttempts:             int32(tools.RetryMaxAttempts),
		ToolsRetryInitialIntervalMs:       int32(tools.RetryInitialIntervalMs),
		ToolsRetryBackoffFactor:           tools.RetryBackoffFactor,
		ToolsRetryMaxIntervalMs:           int32(tools.RetryMaxIntervalMs),
		ToolsRetryJitter:                  tools.RetryJitter,
		ToolsParallelEnabled:              tools.ParallelEnabled,
		ToolsStreamingEnabled:             tools.StreamingEnabled,
		ToolsCircuitBreakerEnabled:        tools.CircuitBreakerEnabled,
		ToolsCircuitBreakerOverridesJson:  tools.CircuitBreakerOverridesJSON,
		ToolsDeferredJson:                 tools.DeferredJSON,
		ToolsCommandSafetyEnabled:         tools.CommandSafetyEnabled,
		PlannerKind:                       ctx.PlannerKind,
		PlannerConfigJson:                 ctx.PlannerConfigJSON,
		RalphLoopMaxIterations:            int32(b.RalphLoopMaxIterations),
		RalphLoopCompletionPromise:        b.RalphLoopCompletionPromise,
		RalphLoopVerifyCommand:            b.RalphLoopVerifyCommand,
		RalphLoopVerifyTimeoutSeconds:     int32(b.RalphLoopVerifyTimeoutSeconds),
		RalphLoopPromiseTagOpen:           b.RalphLoopPromiseTagOpen,
		RalphLoopPromiseTagClose:          b.RalphLoopPromiseTagClose,
		RalphLoopVerifyWorkDir:            b.RalphLoopVerifyWorkDir,
	}
}
