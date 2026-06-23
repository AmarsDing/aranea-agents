package service

import (
	"context"

	v1 "aranea-agents/api/kratos/llm_provider_model/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// LlmProviderModelService implements kratos llm_provider_model.v1.
type LlmProviderModelService struct {
	v1.UnimplementedLlmProviderModelServiceServer

	uc *biz.LlmProviderModelUsecase
	lg loggateway.Logger
}

func NewLlmProviderModelService(uc *biz.LlmProviderModelUsecase, lg loggateway.Logger) *LlmProviderModelService {
	return &LlmProviderModelService{uc: uc, lg: lg}
}

func toProtoPM(m biz.ProviderModel) *v1.ProviderModel {
	caps := provider.CapabilitiesForProviderModel(m)
	return &v1.ProviderModel{
		Id:           m.ID,
		Key:          m.Key,
		Name:         m.Name,
		Description:  m.Description,
		Status:       m.Status,
		Enabled:      m.Enabled,
		SortOrder:    int32(m.SortOrder),
		Provider:     m.Provider,
		Model:        m.Model,
		ConfigJson:   m.ConfigJSON,
		MetadataJson: m.MetadataJSON,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		DeletedAt:    m.DeletedAt,
		Capabilities: &v1.ModelCapabilities{
			Text:     caps.Text,
			Vision:   caps.Vision,
			Audio:    caps.Audio,
			File:     caps.File,
			ToolCall: caps.ToolCall,
			Cache:    caps.Cache,
			Thinking: caps.Thinking,
			TextOnly: caps.TextOnly,
		},
		PricingConfigured: m.PricingConfigured,
	}
}

func patchFromProto(pb *v1.ProviderModel) biz.ProviderModel {
	if pb == nil {
		return biz.ProviderModel{}
	}
	out := biz.ProviderModel{
		Key:          pb.GetKey(),
		Name:         pb.GetName(),
		Description:  pb.GetDescription(),
		Status:       pb.GetStatus(),
		Enabled:      pb.GetEnabled(),
		SortOrder:    int(pb.GetSortOrder()),
		Provider:     pb.GetProvider(),
		Model:        pb.GetModel(),
		ConfigJSON:   pb.GetConfigJson(),
		MetadataJSON: pb.GetMetadataJson(),
	}
	if caps := pb.GetCapabilities(); caps != nil {
		out.Capabilities = capabilitiesFromProto(caps)
		out.CapabilitiesExplicit = hasExplicitBizCapabilities(out.Capabilities)
	}
	return out
}

func capabilitiesFromProto(caps *v1.ModelCapabilities) biz.ModelCapabilities {
	if caps == nil {
		return biz.ModelCapabilities{}
	}
	return biz.ModelCapabilities{
		Text:     caps.GetText(),
		Vision:   caps.GetVision(),
		Audio:    caps.GetAudio(),
		File:     caps.GetFile(),
		ToolCall: caps.GetToolCall(),
		Cache:    caps.GetCache(),
		Thinking: caps.GetThinking(),
		TextOnly: caps.GetTextOnly(),
	}
}

func hasExplicitBizCapabilities(c biz.ModelCapabilities) bool {
	return c.Text || c.Vision || c.Audio || c.File || c.ToolCall || c.Cache || c.Thinking || c.TextOnly
}

// ListProviderModels GET /v1/llm-provider-models.
func (s *LlmProviderModelService) ListProviderModels(ctx context.Context, _ *emptypb.Empty) (*v1.ListProviderModelsResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListProviderModelsResponse{Items: make([]*v1.ProviderModel, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoPM(items[i]))
	}
	return resp, nil
}

// CreateProviderModel POST /v1/llm-provider-models.
func (s *LlmProviderModelService) CreateProviderModel(ctx context.Context, req *v1.CreateProviderModelRequest) (*v1.ProviderModel, error) {
	in := biz.ProviderModel{
		Key:          req.GetKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		Enabled:      req.GetEnabled(),
		SortOrder:    int(req.GetSortOrder()),
		Provider:     req.GetProvider(),
		Model:        req.GetModel(),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
	}
	if caps := req.GetCapabilities(); caps != nil {
		in.Capabilities = capabilitiesFromProto(caps)
		in.CapabilitiesExplicit = hasExplicitBizCapabilities(in.Capabilities)
	}
	out, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoPM(out), nil
}

// GetProviderModel GET /v1/llm-provider-models/{id}.
func (s *LlmProviderModelService) GetProviderModel(ctx context.Context, req *v1.GetProviderModelRequest) (*v1.ProviderModel, error) {
	m, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
		}
		return nil, err
	}
	return toProtoPM(m), nil
}

// RevealProviderModelCredentials GET /v1/llm-provider-models/{id}/credentials.
func (s *LlmProviderModelService) RevealProviderModelCredentials(ctx context.Context, req *v1.RevealProviderModelCredentialsRequest) (*v1.RevealProviderModelCredentialsResponse, error) {
	resourceID := req.GetId()
	out, err := s.uc.RevealCredentials(ctx, resourceID)
	if err != nil {
		logRevealCredentialsDenied(ctx, resourceID, err, s.lg)
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
		}
		return nil, err
	}
	s.lg.Warn("管理员查看 Provider 模型凭据明文",
		loggateway.StepID("admin.provider.credentials_reveal"),
		loggateway.Str("resource_id", resourceID),
		loggateway.Any("has_api_key", out.HasAPIKey),
		loggateway.Any("has_secret_key", out.HasSecretKey),
		loggateway.Int("ha_candidate_count", len(out.HACandidates)),
	)
	resp := &v1.RevealProviderModelCredentialsResponse{
		ApiKey:       out.APIKey,
		SecretKey:    out.SecretKey,
		HasApiKey:    out.HasAPIKey,
		HasSecretKey: out.HasSecretKey,
	}
	for _, ha := range out.HACandidates {
		resp.HaCandidates = append(resp.HaCandidates, &v1.HACandidateCredential{
			Name:   ha.Name,
			ApiKey: ha.APIKey,
		})
	}
	return resp, nil
}

// UpdateProviderModel PATCH /v1/llm-provider-models/{id}.
func (s *LlmProviderModelService) UpdateProviderModel(ctx context.Context, req *v1.UpdateProviderModelRequest) (*v1.ProviderModel, error) {
	if req.GetProviderModel() == nil {
		return nil, apierror.BadRequest("LLM_PROVIDER_MODEL", "provider_model body is required")
	}
	out, err := s.uc.Update(ctx, req.GetId(), patchFromProto(req.GetProviderModel()))
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
		}
		return nil, err
	}
	return toProtoPM(out), nil
}

// DeleteProviderModel DELETE /v1/llm-provider-models/{id}.
func (s *LlmProviderModelService) DeleteProviderModel(ctx context.Context, req *v1.DeleteProviderModelRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// InspectProviderModel POST /v1/llm-provider-models/inspect.
func (s *LlmProviderModelService) InspectProviderModel(ctx context.Context, req *v1.InspectProviderModelRequest) (*v1.InspectProviderModelResponse, error) {
	out, err := s.uc.Inspect(ctx, biz.InspectMerge{
		ResourceID:   req.GetResourceId(),
		ProviderCode: req.GetProviderCode(),
		ProviderType: req.GetProviderType(),
		ModelAPIID:   req.GetModelApiId(),
		APIBaseURL:   req.GetApiBaseUrl(),
		APIKey:       req.GetApiKey(),
		Variant:      req.GetVariant(),
		SecretID:     req.GetSecretId(),
		SecretKey:    req.GetSecretKey(),
		AWSRegion:    req.GetAwsRegion(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.InspectProviderModelResponse{
		Ok:                             out.OK,
		Message:                        out.Message,
		ProviderCode:                   out.ProviderCode,
		ProviderType:                   out.ProviderType,
		ModelApiId:                     out.ModelAPIID,
		ModelDisplayName:               out.ModelDisplayName,
		ModelSizeLabel:                 out.ModelSizeLabel,
		ContextWindowK:                 int32(out.ContextWindowK),
		MaxOutputTokens:                int32(out.MaxOutputTokens),
		InputPriceMicroUsdPer_1K:       out.InputPriceMicroUSDPer1K,
		OutputPriceMicroUsdPer_1K:      out.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUsdPer_1K: out.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUsdPer_1K:   out.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUsdPer_1K:   out.EmbeddingPriceMicroUSDPer1K,
		Source:                         out.Source,
		RawMetadataJson:                out.RawMetadataJSON,
		Variant:                        out.Variant,
		EnableTokenTailoring:           out.EnableTokenTailoring,
		SupportsCache:                  out.SupportsCache,
		SupportsThinking:               out.SupportsThinking,
	}, nil
}

// ValidateProviderPair POST /v1/agents/validate-model.
func (s *LlmProviderModelService) ValidateProviderPair(ctx context.Context, req *v1.ValidateProviderPairRequest) (*v1.ValidateProviderPairResponse, error) {
	ok, msg, err := s.uc.ValidatePair(ctx, req.GetProvider(), req.GetModel())
	if err != nil {
		return nil, err
	}
	return &v1.ValidateProviderPairResponse{Ok: ok, Message: msg}, nil
}

func logRevealCredentialsDenied(ctx context.Context, resourceID string, err error, lg loggateway.Logger) {
	reason := "error"
	switch {
	case apierror.IsCode(err, apierror.CodeNotFound):
		reason = "not_found"
	default:
		if se, ok := apierror.From(err); ok {
			switch se.Code {
			case apierror.CodeBadRequest:
				reason = "bad_request"
			}
		}
	}
	lg.Warn("Provider 凭据查看被拒绝或失败",
		loggateway.StepID("admin.provider.credentials_reveal_denied"),
		loggateway.Str("resource_id", resourceID),
		loggateway.Str("reason", reason),
		loggateway.Err(err),
	)
}
