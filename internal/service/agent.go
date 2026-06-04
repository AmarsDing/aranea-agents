package service

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"strings"

	v1 "aranea-agents/api/kratos/agent/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// AgentService implements kratos agent.v1.
type AgentService struct {
	v1.UnimplementedAgentServiceServer

	uc              *biz.AgentUsecase
	evoUC           *biz.EvolutionUsecase
	mon             *biz.MonitorUsecase
	a2aUC           *biz.A2AUsecase
	promptAI        *PromptFileAIEditor
	agentTemplateUC *biz.AgentTemplateUsecase
	lg              loggateway.Logger
}

func NewAgentService(uc *biz.AgentUsecase, evoUC *biz.EvolutionUsecase, mon *biz.MonitorUsecase, a2aUC *biz.A2AUsecase, promptAI *PromptFileAIEditor, agentTemplateUC *biz.AgentTemplateUsecase, lg loggateway.Logger) *AgentService {
	return &AgentService{uc: uc, evoUC: evoUC, mon: mon, a2aUC: a2aUC, promptAI: promptAI, agentTemplateUC: agentTemplateUC, lg: lg}
}

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
		L1Enabled:                pb.GetL1Enabled(),
		L1BudgetTokens:           int(pb.GetL1BudgetTokens()),
		L1FieldMaxTokens:         int(pb.GetL1FieldMaxTokens()),
		L1HistoryKeepRevisions:   int(pb.GetL1HistoryKeepRevisions()),
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
		Enabled:                pb.GetToolsEnabled(),
		Profile:                pb.GetToolsProfile(),
		ToolCallPrefix:         pb.GetToolsToolCallPrefix(),
		AllowJSON:              pb.GetToolsAllowJson(),
		DenyJSON:               pb.GetToolsDenyJson(),
		ConcurrentAllowJSON:    pb.GetToolsConcurrentAllowJson(),
		RetryEnabled:           pb.GetToolsRetryEnabled(),
		RetryMaxAttempts:       int(pb.GetToolsRetryMaxAttempts()),
		RetryInitialIntervalMs: int(pb.GetToolsRetryInitialIntervalMs()),
		RetryBackoffFactor:     pb.GetToolsRetryBackoffFactor(),
		RetryMaxIntervalMs:     int(pb.GetToolsRetryMaxIntervalMs()),
		RetryJitter:            pb.GetToolsRetryJitter(),
		ParallelEnabled:        pb.GetToolsParallelEnabled(),
		StreamingEnabled:       pb.GetToolsStreamingEnabled(),
		CircuitBreakerEnabled:         pb.GetToolsCircuitBreakerEnabled(),
		CircuitBreakerOverridesJSON:   pb.GetToolsCircuitBreakerOverridesJson(),
		DeferredJSON:                  pb.GetToolsDeferredJson(),
		CommandSafetyEnabled:          pb.GetToolsCommandSafetyEnabled(),
	}
}

func fromProtoSkills(pb *v1.AgentRuntimeSettings) biz.SkillsCfg {
	return biz.SkillsCfg{
		RuntimeJSON:       pb.GetSkillRuntimeJson(),
		LoadMode:          pb.GetSkillLoadMode(),
		IntentPassEnabled: pb.GetIntentPassEnabled(),
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
		MicroCompactEnabled:        pb.GetMicroCompactEnabled(),
		MemoryCompactEnabled:       pb.GetMemoryCompactEnabled(),
		ToolResultGateEnabled:      pb.GetToolResultGateEnabled(),
		CompressLLMCacheEnabled:    pb.GetCompressLlmCacheEnabled(),
		CompressLLMCacheMaxEntries: int(pb.GetCompressLlmCacheMaxEntries()),
		CompressLLMCacheTTLSec:     int(pb.GetCompressLlmCacheTtlSec()),
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
		L1Enabled:                         mem.L1Enabled,
		L1BudgetTokens:                    int32(mem.L1BudgetTokens),
		L1FieldMaxTokens:                  int32(mem.L1FieldMaxTokens),
		L1HistoryKeepRevisions:            int32(mem.L1HistoryKeepRevisions),
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
		IntentPassEnabled:                 skills.IntentPassEnabled,
		ChannelId:                         id.ChannelID,
		ChatId:                            id.ChatID,
		Workspace:                         id.Workspace,
		ReasoningMode:                     rsn.Mode,
		ReasoningLevel:                    rsn.Level,
		VariablesJson:                     id.VariablesJSON,
		ModelInstructionsJson:             id.ModelInstructionsJSON,
		ContextCompactionEnabled:          ctx.CompactionEnabled,
		MicroCompactEnabled:               ctx.MicroCompactEnabled,
		MemoryCompactEnabled:              ctx.MemoryCompactEnabled,
		ToolResultGateEnabled:             ctx.ToolResultGateEnabled,
		CompressLlmCacheEnabled:           ctx.CompressLLMCacheEnabled,
		CompressLlmCacheMaxEntries:        int32(ctx.CompressLLMCacheMaxEntries),
		CompressLlmCacheTtlSec:            int32(ctx.CompressLLMCacheTTLSec),
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
		ToolsCircuitBreakerEnabled:         tools.CircuitBreakerEnabled,
		ToolsCircuitBreakerOverridesJson:    tools.CircuitBreakerOverridesJSON,
		ToolsDeferredJson:                   tools.DeferredJSON,
		ToolsCommandSafetyEnabled:           tools.CommandSafetyEnabled,
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

func fromProtoFile(pb *v1.AgentPromptFile) biz.AgentPromptFile {
	if pb == nil {
		return biz.AgentPromptFile{}
	}
	return biz.AgentPromptFile{
		ID:        pb.GetId(),
		AgentID:   pb.GetAgentId(),
		Name:      pb.GetName(),
		Body:      pb.GetBody(),
		SortOrder: int(pb.GetSortOrder()),
		CreatedAt: pb.GetCreatedAt(),
		UpdatedAt: pb.GetUpdatedAt(),
	}
}

func toProtoFile(b biz.AgentPromptFile) *v1.AgentPromptFile {
	return &v1.AgentPromptFile{
		Id:        b.ID,
		AgentId:   b.AgentID,
		Name:      b.Name,
		Body:      b.Body,
		SortOrder: int32(b.SortOrder),
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

func fromProtoA2AProxy(pb *v1.A2AProxyConfig) *biz.A2AProxyConfig {
	if pb == nil {
		return nil
	}
	cfg := &biz.A2AProxyConfig{
		RemoteURL:       pb.GetRemoteUrl(),
		AgentCardURL:    pb.GetAgentCardUrl(),
		EnableStreaming: pb.GetEnableStreaming(),
		AuthType:        pb.GetAuthType(),
		AuthConfigJSON:  pb.GetAuthConfigJson(),
		TimeoutSeconds:  int(pb.GetTimeoutSeconds()),
	}
	if cfg.RemoteURL == "" && cfg.AgentCardURL == "" {
		return nil
	}
	return cfg
}

func toProtoA2AProxy(cfg *biz.A2AProxyConfig) *v1.A2AProxyConfig {
	if cfg == nil {
		return nil
	}
	return &v1.A2AProxyConfig{
		RemoteUrl:       cfg.RemoteURL,
		AgentCardUrl:    cfg.AgentCardURL,
		EnableStreaming: cfg.EnableStreaming,
		AuthType:        cfg.AuthType,
		AuthConfigJson:  cfg.AuthConfigJSON,
		TimeoutSeconds:  int32(cfg.TimeoutSeconds),
	}
}

func fromProtoAgent(pb *v1.Agent) biz.Agent {
	if pb == nil {
		return biz.Agent{}
	}
	a := biz.Agent{
		ID:                 pb.GetId(),
		AgentKey:           pb.GetAgentKey(),
		DisplayName:        pb.GetDisplayName(),
		Provider:           pb.GetProvider(),
		Model:              pb.GetModel(),
		Status:             pb.GetStatus(),
		IsDefault:          pb.GetIsDefault(),
		IsFavorite:         pb.GetIsFavorite(),
		Icon:               pb.GetIcon(),
		AgentDescription:   pb.GetAgentDescription(),
		TaxonomyPositionID: pb.GetCategoryPositionId(),
		PositionKey:        pb.GetPositionKey(),
		AgentVariant:       pb.GetAgentVariant(),
		VariantDescription: pb.GetVariantDescription(),
		SystemPromptMode:   pb.GetSystemPromptMode(),
		ContextWindow:      int(pb.GetContextWindow()),
		BudgetMonthlyCents: int(pb.GetBudgetMonthlyCents()),
		ConfigJSON:         pb.GetConfigJson(),
		CreatedAt:          pb.GetCreatedAt(),
		UpdatedAt:          pb.GetUpdatedAt(),
		DeletedAt:          pb.GetDeletedAt(),
		Kind:               pb.GetAgentKind(),
		A2AProxy:           fromProtoA2AProxy(pb.GetA2AProxyConfig()),
		Readonly:           pb.GetReadonly(),
	}
	biz.HydrateAgentKind(&a)
	if s := fromProtoRuntime(pb.GetSettings()); s != nil {
		a.Settings = s
	}
	for _, f := range pb.GetFiles() {
		a.Files = append(a.Files, fromProtoFile(f))
	}
	return a
}

func toProtoAgent(b biz.Agent) *v1.Agent {
	biz.HydrateAgentKind(&b)
	out := &v1.Agent{
		Id:                 b.ID,
		AgentKey:           b.AgentKey,
		DisplayName:        b.DisplayName,
		Provider:           b.Provider,
		Model:              b.Model,
		Status:             b.Status,
		IsDefault:          b.IsDefault,
		IsFavorite:         b.IsFavorite,
		Icon:               b.Icon,
		AgentDescription:   b.AgentDescription,
		CategoryPositionId: b.TaxonomyPositionID,
		PositionKey:        b.PositionKey,
		AgentVariant:       b.AgentVariant,
		VariantDescription: b.VariantDescription,
		SystemPromptMode:   b.SystemPromptMode,
		ContextWindow:      int32(b.ContextWindow),
		BudgetMonthlyCents: int32(b.BudgetMonthlyCents),
		ConfigJson:         b.ConfigJSON,
		CreatedAt:          b.CreatedAt,
		UpdatedAt:          b.UpdatedAt,
		DeletedAt:          b.DeletedAt,
		Settings:           toProtoRuntime(b.Settings),
		AgentKind:          b.Kind,
		A2AProxyConfig:     toProtoA2AProxy(b.A2AProxy),
		A2AEndpointEnabled:    b.A2AEndpointEnabled,
		LastRunStatus:         b.LastRunStatus,
		LastRunAt:             b.LastRunAt,
		PendingEvolutionCount: int32(b.PendingEvolutionCount),
		CreatedBy:             b.CreatedBy,
		Readonly:              b.Readonly,
		Source:                b.Source,
	}
	for i := range b.Files {
		out.Files = append(out.Files, toProtoFile(b.Files[i]))
	}
	return out
}

func fromProtoCreate(req *v1.CreateAgentRequest) biz.Agent {
	if req == nil {
		return biz.Agent{}
	}
	a := biz.Agent{
		AgentKey:           req.GetAgentKey(),
		DisplayName:        req.GetDisplayName(),
		Provider:           req.GetProvider(),
		Model:              req.GetModel(),
		Icon:               req.GetIcon(),
		AgentDescription:   req.GetAgentDescription(),
		TaxonomyPositionID: req.GetCategoryPositionId(),
		PositionKey:        req.GetPositionKey(),
		AgentVariant:       req.GetAgentVariant(),
		VariantDescription: req.GetVariantDescription(),
		SystemPromptMode:   req.GetSystemPromptMode(),
		ContextWindow:      int(req.GetContextWindow()),
		BudgetMonthlyCents: int(req.GetBudgetMonthlyCents()),
		ConfigJSON:         req.GetConfigJson(),
		Kind:               req.GetAgentKind(),
		A2AProxy:           fromProtoA2AProxy(req.GetA2AProxyConfig()),
	}
	biz.HydrateAgentKind(&a)
	if s := fromProtoRuntime(req.GetSettings()); s != nil {
		a.Settings = s
	}
	for _, f := range req.GetFiles() {
		a.Files = append(a.Files, fromProtoFile(f))
	}
	return a
}

func (s *AgentService) enrichEndpointFlags(ctx context.Context, agents []biz.Agent) {
	if s == nil || s.a2aUC == nil || len(agents) == 0 {
		return
	}
	ids := make([]string, 0, len(agents))
	for i := range agents {
		if id := strings.TrimSpace(agents[i].ID); id != "" {
			ids = append(ids, id)
		}
	}
	enabled, err := s.a2aUC.MapEndpointEnabled(ctx, ids)
	if err != nil {
		return
	}
	for i := range agents {
		agents[i].A2AEndpointEnabled = enabled[agents[i].ID]
	}
}

func (s *AgentService) enrichAgentEndpoint(ctx context.Context, a *biz.Agent) {
	if s == nil || s.a2aUC == nil || a == nil || strings.TrimSpace(a.ID) == "" {
		return
	}
	enabled, err := s.a2aUC.MapEndpointEnabled(ctx, []string{a.ID})
	if err != nil {
		return
	}
	a.A2AEndpointEnabled = enabled[a.ID]
}

func (s *AgentService) toProtoAgentEnriched(ctx context.Context, a biz.Agent) *v1.Agent {
	s.enrichAgentEndpoint(ctx, &a)
	return toProtoAgent(a)
}

// ListAgents implements GET /v1/agents.
// CheckAgentKey GET /v1/agent-keys/check?agent_key=
func (s *AgentService) CheckAgentKey(ctx context.Context, req *v1.CheckAgentKeyRequest) (*v1.CheckAgentKeyResponse, error) {
	available, msg, err := s.uc.CheckAgentKeyAvailability(ctx, req.GetAgentKey())
	if err != nil {
		return nil, err
	}
	return &v1.CheckAgentKeyResponse{Available: available, Message: msg}, nil
}

func (s *AgentService) ListAgents(ctx context.Context, req *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
	page, err := s.uc.List(ctx, biz.AgentListQuery{
		Keyword:    req.GetKeyword(),
		Status:     req.GetStatus(),
		Provider:   req.GetProvider(),
		CategoryID: req.GetCategoryId(),
		CreatedBy:  biz.ResolveListCreatedByFilter(ctx, req.GetCreatedBy()),
		Limit:      int(req.GetLimit()),
		Offset:     int(req.GetOffset()),
	})
	if err != nil {
		return nil, err
	}
	s.enrichEndpointFlags(ctx, page.Items)
	out := &v1.ListAgentsResponse{
		Total:  int32(page.Total),
		Limit:  int32(page.Limit),
		Offset: int32(page.Offset),
	}
	for i := range page.Items {
		out.Items = append(out.Items, toProtoAgent(page.Items[i]))
	}
	return out, nil
}

// CreateAgent implements POST /v1/agents.
func (s *AgentService) CreateAgent(ctx context.Context, req *v1.CreateAgentRequest) (*v1.Agent, error) {
	created, err := s.uc.Create(ctx, fromProtoCreate(req))
	if err != nil {
		return nil, err
	}
	s.mon.RecordAdminAudit(ctx, "agent.create", "agent", created.ID, fmt.Sprintf("key=%s", created.AgentKey))
	return s.toProtoAgentEnriched(ctx, created), nil
}

// GetAgent implements GET /v1/agents/{id}.
func (s *AgentService) GetAgent(ctx context.Context, req *v1.GetAgentRequest) (*v1.Agent, error) {
	a, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	return s.toProtoAgentEnriched(ctx, a), nil
}

// UpdateAgent implements PATCH /v1/agents/{id}.
func (s *AgentService) UpdateAgent(ctx context.Context, req *v1.UpdateAgentRequest) (*v1.Agent, error) {
	if req.GetAgent() == nil {
		return nil, kerrors.BadRequest("AGENT", "agent body is required")
	}
	patch := fromProtoAgent(req.GetAgent())
	a, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	s.mon.RecordAdminAudit(ctx, "agent.update", "agent", a.ID, fmt.Sprintf("key=%s", a.AgentKey))
	invalidateAgentBuildCache(a.ID)
	return s.toProtoAgentEnriched(ctx, a), nil
}

// DeleteAgent implements DELETE /v1/agents/{id}.
func (s *AgentService) DeleteAgent(ctx context.Context, req *v1.DeleteAgentRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	invalidateAgentBuildCache(req.GetId())
	s.mon.RecordAdminAudit(ctx, "agent.delete", "agent", req.GetId(), "")
	return &emptypb.Empty{}, nil
}

// ToggleFavorite implements PATCH /v1/agents/{id}/favorite.
func (s *AgentService) ToggleFavorite(ctx context.Context, req *v1.ToggleFavoriteRequest) (*v1.Agent, error) {
	a, err := s.uc.ToggleFavorite(ctx, req.GetId())
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	return s.toProtoAgentEnriched(ctx, a), nil
}

// GetAgentPromptPreview implements GET /v1/agents/{id}/system-prompt/preview.
func (s *AgentService) GetAgentPromptPreview(ctx context.Context, req *v1.GetAgentPromptPreviewRequest) (*v1.GetAgentPromptPreviewResponse, error) {
	a, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	mode := strings.TrimSpace(req.GetMode())
	report := chatagent.BuildPreviewReport(ctx, a, mode, chatagent.Deps{AgentUC: s.uc, LG: s.lg})
	sections := make([]*v1.PromptSectionEstimate, 0, len(report.Sections))
	for _, sec := range report.Sections {
		sections = append(sections, &v1.PromptSectionEstimate{
			Key:       sec.Key,
			Label:     sec.Label,
			EstTokens: int32(sec.EstTokens),
			Source:    sec.Source,
		})
	}
	return &v1.GetAgentPromptPreviewResponse{
		Preview:                 report.Summary,
		Instruction:             report.Instruction,
		Sections:              sections,
		StaticTotalTokens:     int32(report.StaticTotalTokens),
		RuntimeOverlayEstTokens: int32(report.RuntimeOverlayEst),
		RuntimeNote:           report.RuntimeNote,
	}, nil
}

func bizEffectiveToolsToProto(in biz.AgentEffectiveTools) *v1.AgentEffectiveToolsView {
	items := make([]*v1.EffectiveAgentTool, 0, len(in.Items))
	for _, row := range in.Items {
		items = append(items, &v1.EffectiveAgentTool{
			ToolKey:        row.ToolKey,
			DisplayName:    row.DisplayName,
			Category:       row.Category,
			Source:         row.Source,
			Enabled:        row.Enabled,
			EffectiveState: row.EffectiveState,
			Reason:         row.Reason,
		})
	}
	return &v1.AgentEffectiveToolsView{
		ToolsEnabled: in.ToolsEnabled,
		Profile:      in.Profile,
		Allow:        append([]string(nil), in.Allow...),
		Deny:         append([]string(nil), in.Deny...),
		Items:        items,
	}
}

// GetAgentEffectiveTools implements GET /v1/agents/{agent_id}/tools/effective.
func (s *AgentService) GetAgentEffectiveTools(ctx context.Context, req *v1.GetAgentEffectiveToolsRequest) (*v1.AgentEffectiveToolsView, error) {
	out, err := s.uc.GetEffectiveTools(ctx, req.GetAgentId())
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	return bizEffectiveToolsToProto(out), nil
}

// UpdateAgentToolPolicy implements PUT /v1/agents/{agent_id}/tools/policy.
func (s *AgentService) UpdateAgentToolPolicy(ctx context.Context, req *v1.UpdateAgentToolPolicyRequest) (*v1.AgentEffectiveToolsView, error) {
	in := biz.AgentToolPolicyInput{
		ToolsEnabled: req.GetToolsEnabled(),
		Profile:      req.GetProfile(),
		Allow:        req.GetAllow(),
		Deny:         req.GetDeny(),
	}
	out, err := s.uc.UpdateAgentToolPolicy(ctx, req.GetAgentId(), in)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return bizEffectiveToolsToProto(out), nil
}

// CreateAgentPromptFile implements POST /v1/agents/{agent_id}/files.
func (s *AgentService) CreateAgentPromptFile(ctx context.Context, req *v1.CreateAgentPromptFileRequest) (*v1.AgentPromptFile, error) {
	f := biz.AgentPromptFile{
		AgentID:   req.GetAgentId(),
		Name:      req.GetName(),
		Body:      req.GetBody(),
		SortOrder: int(req.GetSortOrder()),
	}
	created, err := s.uc.CreatePromptFile(ctx, f)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return toProtoFile(created), nil
}

// UpdateAgentPromptFile implements PATCH /v1/agents/{agent_id}/files/{id}.
func (s *AgentService) UpdateAgentPromptFile(ctx context.Context, req *v1.UpdateAgentPromptFileRequest) (*v1.AgentPromptFile, error) {
	f := biz.AgentPromptFile{
		ID:        req.GetId(),
		AgentID:   req.GetAgentId(),
		Name:      req.GetName(),
		Body:      req.GetBody(),
		SortOrder: int(req.GetSortOrder()),
	}
	updated, err := s.uc.UpdatePromptFile(ctx, f)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT_FILE", "prompt file not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return toProtoFile(updated), nil
}

// DeleteAgentPromptFile implements DELETE /v1/agents/{agent_id}/files/{id}.
func (s *AgentService) DeleteAgentPromptFile(ctx context.Context, req *v1.DeleteAgentPromptFileRequest) (*emptypb.Empty, error) {
	if err := s.uc.DeletePromptFile(ctx, req.GetAgentId(), req.GetId()); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT_FILE", "prompt file not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return &emptypb.Empty{}, nil
}

// EstimateTokens implements POST /v1/agents/{agent_id}/files/estimate-tokens.
func (s *AgentService) EstimateTokens(ctx context.Context, req *v1.EstimateTokensRequest) (*v1.EstimateTokensResponse, error) {
	estimates, err := s.uc.EstimateTokens(ctx, req.GetAgentId())
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	resp := &v1.EstimateTokensResponse{
		TotalTokens: int32(estimates.TotalTokens),
	}
	for _, fe := range estimates.FileEstimates {
		resp.FileEstimates = append(resp.FileEstimates, &v1.FileTokenEstimate{
			FileId:          fe.FileID,
			FileName:        fe.FileName,
			EstimatedTokens: int32(fe.EstimatedTokens),
		})
	}
	return resp, nil
}

// EditPromptFileByAI implements POST /v1/agents/{agent_id}/files/{file_id}/ai-edit.
func (s *AgentService) EditPromptFileByAI(ctx context.Context, req *v1.EditPromptFileByAIRequest) (*v1.EditPromptFileByAIResponse, error) {
	if s.promptAI == nil {
		return nil, kerrors.InternalServer("AGENT_FILE", "prompt file AI editor not configured")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	fileID := strings.TrimSpace(req.GetFileId())
	instruction := strings.TrimSpace(req.GetInstruction())
	if agentID == "" || fileID == "" || instruction == "" {
		return nil, kerrors.BadRequest("AGENT_FILE", "agent_id, file_id and instruction are required")
	}
	a, err := s.uc.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	var target *biz.AgentPromptFile
	for i := range a.Files {
		if a.Files[i].ID == fileID {
			target = &a.Files[i]
			break
		}
	}
	if target == nil {
		return nil, kerrors.NotFound("AGENT_FILE", "prompt file not found")
	}
	revised, err := s.promptAI.Revise(ctx, a.Provider, a.Model, target.Name, target.Body, instruction)
	if err != nil {
		return nil, mapPromptFileAIError(err)
	}
	target.Body = revised
	updated, err := s.uc.UpdatePromptFile(ctx, *target)
	if err != nil {
		return nil, err
	}
	invalidateAgentBuildCache(agentID)
	s.lg.Info("AI 修订提示文件完成", loggateway.StepID("agent.prompt.ai_edit"), loggateway.Str("flow_status", "done"), loggateway.Str("agent_id", agentID), loggateway.Str("file_id", fileID))
	return &v1.EditPromptFileByAIResponse{File: toProtoFile(updated)}, nil
}

// ListAgentTemplates implements GET /v1/agent-templates.
func (s *AgentService) ListAgentTemplates(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentTemplatesResponse, error) {
	items, err := s.agentTemplateUC.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentTemplatesResponse{Items: make([]*v1.AgentTemplate, 0, len(items))}
	for _, t := range items {
		out.Items = append(out.Items, &v1.AgentTemplate{
			Key:         t.Key,
			Label:       t.Label,
			Icon:        t.Icon,
			Description: t.Description,
			DisplayName: t.DisplayName,
			Provider:    t.Provider,
			Model:       t.Model,
		})
	}
	return out, nil
}

// ListAgentCreators implements GET /v1/agents/creators.
func (s *AgentService) ListAgentCreators(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCreatorsResponse, error) {
	items, err := s.uc.ListAgentCreators(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentCreatorsResponse{Items: make([]*v1.AgentCreator, 0, len(items))}
	for _, c := range items {
		out.Items = append(out.Items, &v1.AgentCreator{UserId: c.UserID, Label: c.Label})
	}
	return out, nil
}

// DuplicateAgent implements POST /v1/agents/{id}/duplicate.
func (s *AgentService) DuplicateAgent(ctx context.Context, req *v1.DuplicateAgentRequest) (*v1.Agent, error) {
	dup, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return s.toProtoAgentEnriched(ctx, dup), nil
}
