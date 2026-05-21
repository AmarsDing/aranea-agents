package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SystemSettingService implements system_setting.v1.
type SystemSettingService struct {
	v1.UnimplementedSystemSettingServiceServer

	uc              *biz.SystemSettingUsecase
	a2aPublicBase   *A2APublicBaseReloader
}

func NewSystemSettingService(uc *biz.SystemSettingUsecase, a2aPublicBase *A2APublicBaseReloader) *SystemSettingService {
	return &SystemSettingService{uc: uc, a2aPublicBase: a2aPublicBase}
}

func (s *SystemSettingService) GetSystemSettings(ctx context.Context, _ *emptypb.Empty) (*v1.SystemSettings, error) {
	row, err := s.uc.Get(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoSystemSettings(row), nil
}

func (s *SystemSettingService) UpdateSystemSettings(ctx context.Context, req *v1.UpdateSystemSettingsRequest) (*v1.SystemSettings, error) {
	row, err := s.uc.Update(ctx, req.GetRootDirectory(), req.GetWorkDirectory(), req.GetGlobalMonthlyMicroUsd(), req.GetA2APublicBaseUrl(), req.GetMcpAllowAdhocHttp())
	if err != nil {
		return nil, err
	}
	if hasKnowledgeEmbedUpdate(req) {
		embed, err := s.uc.UpdateKnowledgeEmbed(ctx, biz.KnowledgeEmbedSetting{
			Provider: req.GetKnowledgeEmbedProvider(),
			BaseURL:  req.GetKnowledgeEmbedBaseUrl(),
			APIKey:   req.GetKnowledgeEmbedApiKey(),
			Model:    req.GetKnowledgeEmbedModel(),
			Dim:      int(req.GetKnowledgeEmbedDim()),
		}, strings.TrimSpace(req.GetKnowledgeEmbedApiKey()) != "")
		if err != nil {
			return nil, err
		}
		row.KnowledgeEmbed = embed
	}
	evalLLM, err := s.uc.UpdateEvalLLM(ctx, biz.EvalLLMSetting{
		SimProvider:   req.GetEvalSimProvider(),
		SimModel:      req.GetEvalSimModel(),
		JudgeProvider: req.GetEvalJudgeProvider(),
		JudgeModel:    req.GetEvalJudgeModel(),
	})
	if err != nil {
		return nil, err
	}
	row.EvalLLM = evalLLM
	if s.a2aPublicBase != nil {
		s.a2aPublicBase.Reload(row.A2APublicBaseURL)
	}
	return toProtoSystemSettings(row), nil
}

func hasKnowledgeEmbedUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetKnowledgeEmbedProvider() != "" ||
		req.GetKnowledgeEmbedBaseUrl() != "" ||
		req.GetKnowledgeEmbedModel() != "" ||
		req.GetKnowledgeEmbedDim() > 0 ||
		strings.TrimSpace(req.GetKnowledgeEmbedApiKey()) != ""
}

func toProtoSystemSettings(row biz.SystemSetting) *v1.SystemSettings {
	return &v1.SystemSettings{
		WorkDirectory:                     row.WorkDirectory,
		UpdateTime:                        timestamppb.New(row.UpdateTime),
		RootDirectory:                     row.RootDirectory,
		GlobalMonthlyMicroUsd:             row.GlobalMonthlyMicroUSD,
		A2APublicBaseUrl:                  row.A2APublicBaseURL,
		CredentialEncryptionKeyConfigured: row.CredentialEncryptionKeyConfigured,
		KnowledgeEmbed:                    toProtoKnowledgeEmbed(row.KnowledgeEmbed),
		EvalLlm:                           toProtoEvalLLM(row.EvalLLM),
		McpAllowAdhocHttp:                 row.MCPAllowAdHocHTTP,
	}
}

func toProtoKnowledgeEmbed(row biz.KnowledgeEmbedSetting) *v1.KnowledgeEmbedSettings {
	return &v1.KnowledgeEmbedSettings{
		Provider:   row.Provider,
		BaseUrl:    row.BaseURL,
		Model:      row.Model,
		Dim:        int32(row.Dim),
		Configured: biz.KnowledgeEmbedConfigured(row),
		HasApiKey:  row.HasAPIKey,
	}
}

func toProtoEvalLLM(row biz.EvalLLMSetting) *v1.EvalLLMSettings {
	return &v1.EvalLLMSettings{
		SimProvider:   row.SimProvider,
		SimModel:      row.SimModel,
		JudgeProvider: row.JudgeProvider,
		JudgeModel:    row.JudgeModel,
		Configured:    row.SimConfigured() || row.JudgeConfigured(),
	}
}
