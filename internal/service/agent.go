package service

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// AgentService implements kratos agent.v1.
type AgentService struct {
	v1.UnimplementedAgentServiceServer

	uc    *biz.AgentUsecase
	evoUC *biz.EvolutionUsecase
	mon   *biz.MonitorUsecase
}

// NewAgentService constructs the service.
func NewAgentService(uc *biz.AgentUsecase, evoUC *biz.EvolutionUsecase, mon *biz.MonitorUsecase) *AgentService {
	return &AgentService{uc: uc, evoUC: evoUC, mon: mon}
}

func fromProtoRuntime(pb *v1.AgentRuntimeSettings) *biz.AgentRuntimeSettings {
	if pb == nil {
		return nil
	}
	return &biz.AgentRuntimeSettings{
		AgentID:                           pb.GetAgentId(),
		SelfEvolve:                        pb.GetSelfEvolve(),
		SubagentsEnabled:                  pb.GetSubagentsEnabled(),
		SubagentsMaxConcurrency:           int(pb.GetSubagentsMaxConcurrency()),
		SubagentsMaxGenerationDepth:       int(pb.GetSubagentsMaxGenerationDepth()),
		SubagentsMaxChildrenPerAgent:      int(pb.GetSubagentsMaxChildrenPerAgent()),
		SubagentsArchiveAfterMinutes:      int(pb.GetSubagentsArchiveAfterMinutes()),
		SubagentsMaxRetries:               int(pb.GetSubagentsMaxRetries()),
		SubagentsModelOverride:            pb.GetSubagentsModelOverride(),
		ToolsEnabled:                      pb.GetToolsEnabled(),
		ToolsProfile:                      pb.GetToolsProfile(),
		ToolsToolCallPrefix:               pb.GetToolsToolCallPrefix(),
		ToolsAllowJSON:                    pb.GetToolsAllowJson(),
		ToolsDenyJSON:                     pb.GetToolsDenyJson(),
		ToolsConcurrentAllowJSON:          pb.GetToolsConcurrentAllowJson(),
		MemoryEnabled:                     pb.GetMemoryEnabled(),
		MemoryMaxChunkLength:              int(pb.GetMemoryMaxChunkLength()),
		MemoryMaxResults:                  int(pb.GetMemoryMaxResults()),
		MemoryMinScore:                    pb.GetMemoryMinScore(),
		HeartbeatEnabled:                  pb.GetHeartbeatEnabled(),
		HeartbeatIntervalMinutes:          int(pb.GetHeartbeatIntervalMinutes()),
		EvolutionSelfEvolve:               pb.GetEvolutionSelfEvolve(),
		EvolutionSkillEvolve:              pb.GetEvolutionSkillEvolve(),
		EvolutionMetricsEnabled:           pb.GetEvolutionMetricsEnabled(),
		EvolutionSuggestionsEnabled:       pb.GetEvolutionSuggestionsEnabled(),
		GuardrailMaxChangePerPeriod:       pb.GetGuardrailMaxChangePerPeriod(),
		GuardrailMinDataPoints:            int(pb.GetGuardrailMinDataPoints()),
		GuardrailRollbackOnDeclinePercent: int(pb.GetGuardrailRollbackOnDeclinePercent()),
		L0RecentWindowTurns:               int(pb.GetL0RecentWindowTurns()),
		L0RecentWindowTokens:              int(pb.GetL0RecentWindowTokens()),
		L0SummaryThreshold:                pb.GetL0SummaryThreshold(),
		L0SummaryKeepTurns:                int(pb.GetL0SummaryKeepTurns()),
		L0TruncateStrategy:                pb.GetL0TruncateStrategy(),
		L0InjectL1:                        pb.GetL0InjectL1(),
		L0InjectL3:                        pb.GetL0InjectL3(),
		L0InjectL4:                        pb.GetL0InjectL4(),
		L0L3MaxChunks:                     int(pb.GetL0L3MaxChunks()),
		L0L4MaxPaths:                      int(pb.GetL0L4MaxPaths()),
		L0SnapshotMode:                    pb.GetL0SnapshotMode(),
		L1Enabled:                         pb.GetL1Enabled(),
		L1BudgetTokens:                    int(pb.GetL1BudgetTokens()),
		L1FieldMaxTokens:                  int(pb.GetL1FieldMaxTokens()),
		L1HistoryKeepRevisions:            int(pb.GetL1HistoryKeepRevisions()),
		L1DefaultSchemaID:                 pb.GetL1DefaultSchemaId(),
		L1ArchiveOnIdleMinutes:            int(pb.GetL1ArchiveOnIdleMinutes()),
		L2EpisodeEnabled:                  pb.GetL2EpisodeEnabled(),
		L2EpisodeMinImportance:            pb.GetL2EpisodeMinImportance(),
		L2IndexEnabled:                    pb.GetL2IndexEnabled(),
		L2IndexEmbeddingModel:             pb.GetL2IndexEmbeddingModel(),
		L2RecallEnabled:                   pb.GetL2RecallEnabled(),
		L2RecallMax:                       int(pb.GetL2RecallMax()),
		L2RetentionDays:                   int(pb.GetL2RetentionDays()),
		L2ArchiveAfterDays:                int(pb.GetL2ArchiveAfterDays()),
		L3Enabled:                         pb.GetL3Enabled(),
		L3RecallTopK:                      int(pb.GetL3RecallTopK()),
		L3RecallMinScore:                  pb.GetL3RecallMinScore(),
		L3RecallScopesJSON:                pb.GetL3RecallScopesJson(),
		L3EmbeddingModel:                  pb.GetL3EmbeddingModel(),
		L3DecayIntervalHours:              int(pb.GetL3DecayIntervalHours()),
		L3ArchiveThreshold:                pb.GetL3ArchiveThreshold(),
		L3MaxPerRecallChars:               int(pb.GetL3MaxPerRecallChars()),
		L4Enabled:                         pb.GetL4Enabled(),
		L4GraphInjectNeighbors:            pb.GetL4GraphInjectNeighbors(),
		L4GraphMaxNeighbors:               int(pb.GetL4GraphMaxNeighbors()),
		L4GraphMaxHops:                    int(pb.GetL4GraphMaxHops()),
		L4IdentityInject:                  pb.GetL4IdentityInject(),
		L4StrategyInject:                  pb.GetL4StrategyInject(),
		EvoEnabled:                        pb.GetEvoEnabled(),
		EvoAutoApply:                      pb.GetEvoAutoApply(),
		EvoMinEpisodes:                    int(pb.GetEvoMinEpisodes()),
		EvoMinNegativeFeedback:            int(pb.GetEvoMinNegativeFeedback()),
		EvoThrottleHours:                  int(pb.GetEvoThrottleHours()),
		EvoProposalTTLDays:                int(pb.GetEvoProposalTtlDays()),
		EvoPersonaMaxChars:                int(pb.GetEvoPersonaMaxChars()),
		EvoSystemPromptMaxAppends:         int(pb.GetEvoSystemPromptMaxAppends()),
		CreatedAt:                         pb.GetCreatedAt(),
		UpdatedAt:                         pb.GetUpdatedAt(),
		SkillRuntimeJSON:                  pb.GetSkillRuntimeJson(),
		IntentPassEnabled:                 pb.GetIntentPassEnabled(),
		ChannelID:                         pb.GetChannelId(),
		ChatID:                            pb.GetChatId(),
		Workspace:                         pb.GetWorkspace(),
		ReasoningMode:                     pb.GetReasoningMode(),
		ReasoningLevel:                    pb.GetReasoningLevel(),
		VariablesJSON:                     pb.GetVariablesJson(),
		ModelInstructionsJSON:             pb.GetModelInstructionsJson(),
		ContextCompactionEnabled:          pb.GetContextCompactionEnabled(),
		SessionSummaryEnabled:             pb.GetSessionSummaryEnabled(),
		SkillLoadMode:                     pb.GetSkillLoadMode(),
		OutputSchemaJSON:                  pb.GetOutputSchemaJson(),
		ModelSelector:                     pb.GetModelSelector(),
		ToolsRetryEnabled:                 pb.GetToolsRetryEnabled(),
		ToolsRetryMaxAttempts:             int(pb.GetToolsRetryMaxAttempts()),
		ToolsRetryInitialIntervalMs:       int(pb.GetToolsRetryInitialIntervalMs()),
		ToolsRetryBackoffFactor:           pb.GetToolsRetryBackoffFactor(),
		ToolsRetryMaxIntervalMs:           int(pb.GetToolsRetryMaxIntervalMs()),
		ToolsRetryJitter:                  pb.GetToolsRetryJitter(),
		ToolsParallelEnabled:              pb.GetToolsParallelEnabled(),
		ToolsStreamingEnabled:             pb.GetToolsStreamingEnabled(),
		PlannerKind:                       pb.GetPlannerKind(),
	}
}

func toProtoRuntime(b *biz.AgentRuntimeSettings) *v1.AgentRuntimeSettings {
	if b == nil {
		return nil
	}
	return &v1.AgentRuntimeSettings{
		AgentId:                           b.AgentID,
		SelfEvolve:                        b.SelfEvolve,
		SubagentsEnabled:                  b.SubagentsEnabled,
		SubagentsMaxConcurrency:           int32(b.SubagentsMaxConcurrency),
		SubagentsMaxGenerationDepth:       int32(b.SubagentsMaxGenerationDepth),
		SubagentsMaxChildrenPerAgent:      int32(b.SubagentsMaxChildrenPerAgent),
		SubagentsArchiveAfterMinutes:      int32(b.SubagentsArchiveAfterMinutes),
		SubagentsMaxRetries:               int32(b.SubagentsMaxRetries),
		SubagentsModelOverride:            b.SubagentsModelOverride,
		ToolsEnabled:                      b.ToolsEnabled,
		ToolsProfile:                      b.ToolsProfile,
		ToolsToolCallPrefix:               b.ToolsToolCallPrefix,
		ToolsAllowJson:                    b.ToolsAllowJSON,
		ToolsDenyJson:                     b.ToolsDenyJSON,
		ToolsConcurrentAllowJson:          b.ToolsConcurrentAllowJSON,
		MemoryEnabled:                     b.MemoryEnabled,
		MemoryMaxChunkLength:              int32(b.MemoryMaxChunkLength),
		MemoryMaxResults:                  int32(b.MemoryMaxResults),
		MemoryMinScore:                    b.MemoryMinScore,
		HeartbeatEnabled:                  b.HeartbeatEnabled,
		HeartbeatIntervalMinutes:          int32(b.HeartbeatIntervalMinutes),
		EvolutionSelfEvolve:               b.EvolutionSelfEvolve,
		EvolutionSkillEvolve:              b.EvolutionSkillEvolve,
		EvolutionMetricsEnabled:           b.EvolutionMetricsEnabled,
		EvolutionSuggestionsEnabled:       b.EvolutionSuggestionsEnabled,
		GuardrailMaxChangePerPeriod:       b.GuardrailMaxChangePerPeriod,
		GuardrailMinDataPoints:            int32(b.GuardrailMinDataPoints),
		GuardrailRollbackOnDeclinePercent: int32(b.GuardrailRollbackOnDeclinePercent),
		L0RecentWindowTurns:               int32(b.L0RecentWindowTurns),
		L0RecentWindowTokens:              int32(b.L0RecentWindowTokens),
		L0SummaryThreshold:                b.L0SummaryThreshold,
		L0SummaryKeepTurns:                int32(b.L0SummaryKeepTurns),
		L0TruncateStrategy:                b.L0TruncateStrategy,
		L0InjectL1:                        b.L0InjectL1,
		L0InjectL3:                        b.L0InjectL3,
		L0InjectL4:                        b.L0InjectL4,
		L0L3MaxChunks:                     int32(b.L0L3MaxChunks),
		L0L4MaxPaths:                      int32(b.L0L4MaxPaths),
		L0SnapshotMode:                    b.L0SnapshotMode,
		L1Enabled:                         b.L1Enabled,
		L1BudgetTokens:                    int32(b.L1BudgetTokens),
		L1FieldMaxTokens:                  int32(b.L1FieldMaxTokens),
		L1HistoryKeepRevisions:            int32(b.L1HistoryKeepRevisions),
		L1DefaultSchemaId:                 b.L1DefaultSchemaID,
		L1ArchiveOnIdleMinutes:            int32(b.L1ArchiveOnIdleMinutes),
		L2EpisodeEnabled:                  b.L2EpisodeEnabled,
		L2EpisodeMinImportance:            b.L2EpisodeMinImportance,
		L2IndexEnabled:                    b.L2IndexEnabled,
		L2IndexEmbeddingModel:             b.L2IndexEmbeddingModel,
		L2RecallEnabled:                   b.L2RecallEnabled,
		L2RecallMax:                       int32(b.L2RecallMax),
		L2RetentionDays:                   int32(b.L2RetentionDays),
		L2ArchiveAfterDays:                int32(b.L2ArchiveAfterDays),
		L3Enabled:                         b.L3Enabled,
		L3RecallTopK:                      int32(b.L3RecallTopK),
		L3RecallMinScore:                  b.L3RecallMinScore,
		L3RecallScopesJson:                b.L3RecallScopesJSON,
		L3EmbeddingModel:                  b.L3EmbeddingModel,
		L3DecayIntervalHours:              int32(b.L3DecayIntervalHours),
		L3ArchiveThreshold:                b.L3ArchiveThreshold,
		L3MaxPerRecallChars:               int32(b.L3MaxPerRecallChars),
		L4Enabled:                         b.L4Enabled,
		L4GraphInjectNeighbors:            b.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:               int32(b.L4GraphMaxNeighbors),
		L4GraphMaxHops:                    int32(b.L4GraphMaxHops),
		L4IdentityInject:                  b.L4IdentityInject,
		L4StrategyInject:                  b.L4StrategyInject,
		EvoEnabled:                        b.EvoEnabled,
		EvoAutoApply:                      b.EvoAutoApply,
		EvoMinEpisodes:                    int32(b.EvoMinEpisodes),
		EvoMinNegativeFeedback:            int32(b.EvoMinNegativeFeedback),
		EvoThrottleHours:                  int32(b.EvoThrottleHours),
		EvoProposalTtlDays:                int32(b.EvoProposalTTLDays),
		EvoPersonaMaxChars:                int32(b.EvoPersonaMaxChars),
		EvoSystemPromptMaxAppends:         int32(b.EvoSystemPromptMaxAppends),
		CreatedAt:                         b.CreatedAt,
		UpdatedAt:                         b.UpdatedAt,
		SkillRuntimeJson:                  b.SkillRuntimeJSON,
		IntentPassEnabled:                 b.IntentPassEnabled,
		ChannelId:                         b.ChannelID,
		ChatId:                            b.ChatID,
		Workspace:                         b.Workspace,
		ReasoningMode:                     b.ReasoningMode,
		ReasoningLevel:                    b.ReasoningLevel,
		VariablesJson:                     b.VariablesJSON,
		ModelInstructionsJson:             b.ModelInstructionsJSON,
		ContextCompactionEnabled:          b.ContextCompactionEnabled,
		SessionSummaryEnabled:             b.SessionSummaryEnabled,
		SkillLoadMode:                     b.SkillLoadMode,
		OutputSchemaJson:                  b.OutputSchemaJSON,
		ModelSelector:                     b.ModelSelector,
		ToolsRetryEnabled:                 b.ToolsRetryEnabled,
		ToolsRetryMaxAttempts:             int32(b.ToolsRetryMaxAttempts),
		ToolsRetryInitialIntervalMs:       int32(b.ToolsRetryInitialIntervalMs),
		ToolsRetryBackoffFactor:           b.ToolsRetryBackoffFactor,
		ToolsRetryMaxIntervalMs:           int32(b.ToolsRetryMaxIntervalMs),
		ToolsRetryJitter:                  b.ToolsRetryJitter,
		ToolsParallelEnabled:              b.ToolsParallelEnabled,
		ToolsStreamingEnabled:             b.ToolsStreamingEnabled,
		PlannerKind:                       b.PlannerKind,
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
		CategoryPositionID: pb.GetCategoryPositionId(),
		SystemPromptMode:   pb.GetSystemPromptMode(),
		ContextWindow:      int(pb.GetContextWindow()),
		BudgetMonthlyCents: int(pb.GetBudgetMonthlyCents()),
		ConfigJSON:         pb.GetConfigJson(),
		CreatedAt:          pb.GetCreatedAt(),
		UpdatedAt:          pb.GetUpdatedAt(),
		DeletedAt:          pb.GetDeletedAt(),
	}
	if s := fromProtoRuntime(pb.GetSettings()); s != nil {
		a.Settings = s
	}
	for _, f := range pb.GetFiles() {
		a.Files = append(a.Files, fromProtoFile(f))
	}
	return a
}

func toProtoAgent(b biz.Agent) *v1.Agent {
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
		CategoryPositionId: b.CategoryPositionID,
		SystemPromptMode:   b.SystemPromptMode,
		ContextWindow:      int32(b.ContextWindow),
		BudgetMonthlyCents: int32(b.BudgetMonthlyCents),
		ConfigJson:         b.ConfigJSON,
		CreatedAt:          b.CreatedAt,
		UpdatedAt:          b.UpdatedAt,
		DeletedAt:          b.DeletedAt,
		Settings:           toProtoRuntime(b.Settings),
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
		CategoryPositionID: req.GetCategoryPositionId(),
		SystemPromptMode:   req.GetSystemPromptMode(),
		ContextWindow:      int(req.GetContextWindow()),
		BudgetMonthlyCents: int(req.GetBudgetMonthlyCents()),
		ConfigJSON:         req.GetConfigJson(),
	}
	if s := fromProtoRuntime(req.GetSettings()); s != nil {
		a.Settings = s
	}
	for _, f := range req.GetFiles() {
		a.Files = append(a.Files, fromProtoFile(f))
	}
	return a
}

// ListAgents implements GET /v1/agents.
func (s *AgentService) ListAgents(ctx context.Context, req *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
	page, err := s.uc.List(ctx, biz.AgentListQuery{
		Keyword:    req.GetKeyword(),
		Status:     req.GetStatus(),
		Provider:   req.GetProvider(),
		CategoryID: req.GetCategoryId(),
		Limit:      int(req.GetLimit()),
		Offset:     int(req.GetOffset()),
	})
	if err != nil {
		return nil, err
	}
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
	biz.RecordAdminAudit(ctx, s.mon, "agent.create", "agent", created.ID, fmt.Sprintf("key=%s", created.AgentKey))
	return toProtoAgent(created), nil
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
	return toProtoAgent(a), nil
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
	biz.RecordAdminAudit(ctx, s.mon, "agent.update", "agent", a.ID, fmt.Sprintf("key=%s", a.AgentKey))
	return toProtoAgent(a), nil
}

// DeleteAgent implements DELETE /v1/agents/{id}.
func (s *AgentService) DeleteAgent(ctx context.Context, req *v1.DeleteAgentRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	biz.RecordAdminAudit(ctx, s.mon, "agent.delete", "agent", req.GetId(), "")
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
	return toProtoAgent(a), nil
}

// GetAgentPromptPreview implements GET /v1/agents/{id}/system-prompt/preview.
func (s *AgentService) GetAgentPromptPreview(ctx context.Context, req *v1.GetAgentPromptPreviewRequest) (*v1.GetAgentPromptPreviewResponse, error) {
	text, err := s.uc.PromptPreview(ctx, req.GetId(), req.GetMode())
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT", "agent not found")
		}
		return nil, err
	}
	return &v1.GetAgentPromptPreviewResponse{Preview: text}, nil
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
