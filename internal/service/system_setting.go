package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SystemSettingService implements system_setting.v1.
type SystemSettingService struct {
	v1.UnimplementedSystemSettingServiceServer

	uc            *biz.SystemSettingUsecase
	a2aPublicBase *A2APublicBaseReloader
	crypto        *biz.CredentialCrypto
	// embedderAdmin hot-reloads in-memory knowledge embedder after DB persist (same path as KnowledgeService).
	embedderAdmin knowledge.EmbedderAdmin
	lg            loggateway.Logger
}

func NewSystemSettingService(
	uc *biz.SystemSettingUsecase,
	a2aPublicBase *A2APublicBaseReloader,
	crypto *biz.CredentialCrypto,
	embedder knowledge.Embedder,
	lg loggateway.Logger,
) *SystemSettingService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var admin knowledge.EmbedderAdmin
	if a, ok := embedder.(knowledge.EmbedderAdmin); ok {
		admin = a
	}
	return &SystemSettingService{
		uc:            uc,
		a2aPublicBase: a2aPublicBase,
		crypto:        crypto,
		embedderAdmin: admin,
		lg:            lg,
	}
}

func (s *SystemSettingService) GetSystemSettings(ctx context.Context, _ *emptypb.Empty) (*v1.SystemSettings, error) {
	row, err := s.uc.Get(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoSystemSettings(row), nil
}

func (s *SystemSettingService) UpdateSystemSettings(ctx context.Context, req *v1.UpdateSystemSettingsRequest) (*v1.SystemSettings, error) {
	patch := biz.SystemSettingAllPatch{
		RootDir:               req.GetRootDirectory(),
		WorkDir:               req.GetWorkDirectory(),
		GlobalMonthlyMicroUSD: req.GetGlobalMonthlyMicroUsd(),
		A2APublicBaseURL:      req.GetA2APublicBaseUrl(),
		MCPAllowAdHocHTTP:     req.GetMcpAllowAdhocHttp(),
	}
	if hasKnowledgeEmbedUpdate(req) {
		patch.KnowledgeEmbed = &biz.KnowledgeEmbedSetting{
			Provider: req.GetKnowledgeEmbedProvider(),
			BaseURL:  req.GetKnowledgeEmbedBaseUrl(),
			APIKey:   req.GetKnowledgeEmbedApiKey(),
			Model:    req.GetKnowledgeEmbedModel(),
			Dim:      int(req.GetKnowledgeEmbedDim()),
		}
		patch.KnowledgeEmbedUpdateKey = strings.TrimSpace(req.GetKnowledgeEmbedApiKey()) != ""
	}
	if hasEvalLLMUpdate(req) {
		patch.EvalLLM = &biz.EvalLLMSetting{
			SimProvider:   req.GetEvalSimProvider(),
			SimModel:      req.GetEvalSimModel(),
			JudgeProvider: req.GetEvalJudgeProvider(),
			JudgeModel:    req.GetEvalJudgeModel(),
		}
	}
	if hasWebResearchUpdate(req) {
		patch.WebResearch = &biz.WebResearchSetting{
			Provider:    req.GetWebResearchProvider(),
			APIKey:      req.GetWebResearchApiKey(),
			MaxResults:  int(req.GetWebResearchMaxResults()),
			FetchTop:    int(req.GetWebResearchFetchTop()),
			SearchDepth: req.GetWebResearchSearchDepth(),
			TimeoutSec:  int(req.GetWebResearchTimeoutSec()),
			HTTPProxy:   req.GetWebResearchHttpProxy(),
		}
		patch.WebResearchUpdateKey = strings.TrimSpace(req.GetWebResearchApiKey()) != ""
	}
	row, err := s.uc.UpdateAll(ctx, patch)
	if err != nil {
		return nil, err
	}
	if s.a2aPublicBase != nil {
		s.a2aPublicBase.Reload(row.A2APublicBaseURL)
	}
	// Keep in-memory knowledge embedder aligned with DB (authoritative for live RAG).
	if patch.KnowledgeEmbed != nil && s.embedderAdmin != nil {
		ke := row.KnowledgeEmbed
		apiKey := ""
		if patch.KnowledgeEmbedUpdateKey {
			apiKey = strings.TrimSpace(patch.KnowledgeEmbed.APIKey)
		}
		s.embedderAdmin.Update(ke.Provider, ke.BaseURL, apiKey, ke.Model, ke.Dim)
		s.lg.Info("system settings: knowledge embedder hot-reloaded after persist",
			loggateway.StepID("system_setting.embedder.reload"),
		)
	}
	return toProtoSystemSettings(row), nil
}

func hasWebResearchUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetWebResearchProvider() != "" ||
		req.GetWebResearchMaxResults() > 0 ||
		req.GetWebResearchFetchTop() > 0 ||
		req.GetWebResearchSearchDepth() != "" ||
		req.GetWebResearchTimeoutSec() > 0 ||
		req.GetWebResearchHttpProxy() != "" ||
		strings.TrimSpace(req.GetWebResearchApiKey()) != ""
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

func hasEvalLLMUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetEvalSimProvider() != "" ||
		req.GetEvalSimModel() != "" ||
		req.GetEvalJudgeProvider() != "" ||
		req.GetEvalJudgeModel() != ""
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
		WebResearch:                       toProtoWebResearch(row.WebResearch),
	}
}

func toProtoWebResearch(row biz.WebResearchSetting) *v1.WebResearchSettings {
	return &v1.WebResearchSettings{
		Provider:    row.Provider,
		MaxResults:  int32(row.MaxResults),
		FetchTop:    int32(row.FetchTop),
		SearchDepth: row.SearchDepth,
		TimeoutSec:  int32(row.TimeoutSec),
		HttpProxy:   row.HTTPProxy,
		Configured:  biz.WebResearchConfigured(row),
		HasApiKey:   row.HasAPIKey,
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

func (s *SystemSettingService) TestWebResearch(ctx context.Context, req *v1.TestWebResearchRequest) (*v1.TestWebResearchResponse, error) {
	if req == nil {
		req = &v1.TestWebResearchRequest{}
	}
	res, err := s.uc.TestWebResearch(ctx, biz.WebResearchSetting{
		Provider:    req.GetProvider(),
		MaxResults:  int(req.GetMaxResults()),
		FetchTop:    int(req.GetFetchTop()),
		SearchDepth: req.GetSearchDepth(),
		TimeoutSec:  int(req.GetTimeoutSec()),
		HTTPProxy:   req.GetHttpProxy(),
	}, req.GetApiKey())
	out := &v1.TestWebResearchResponse{
		Ok:              res.OK,
		Message:         res.Message,
		Provider:        res.Provider,
		ResultCount:     int32(res.ResultCount),
		LatencyMs:       int32(res.LatencyMS),
		ResponseTimeSec: res.ResponseTime,
	}
	if err != nil {
		if out.Message == "" {
			out.Message = err.Error()
		}
		return out, nil
	}
	return out, nil
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
