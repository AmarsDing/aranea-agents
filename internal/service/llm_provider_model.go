package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/llm_provider_model/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// LlmProviderModelService implements kratos llm_provider_model.v1.
type LlmProviderModelService struct {
	v1.UnimplementedLlmProviderModelServiceServer

	uc *biz.LlmProviderModelUsecase
}

func NewLlmProviderModelService(uc *biz.LlmProviderModelUsecase) *LlmProviderModelService {
	return &LlmProviderModelService{uc: uc}
}

func toProtoPM(m biz.ProviderModel) *v1.ProviderModel {
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
	}
}

func patchFromProto(pb *v1.ProviderModel) biz.ProviderModel {
	if pb == nil {
		return biz.ProviderModel{}
	}
	return biz.ProviderModel{
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
		}
		return nil, err
	}
	return toProtoPM(m), nil
}

// UpdateProviderModel PATCH /v1/llm-provider-models/{id}.
func (s *LlmProviderModelService) UpdateProviderModel(ctx context.Context, req *v1.UpdateProviderModelRequest) (*v1.ProviderModel, error) {
	if req.GetProviderModel() == nil {
		return nil, kerrors.BadRequest("LLM_PROVIDER_MODEL", "provider_model body is required")
	}
	out, err := s.uc.Update(ctx, req.GetId(), patchFromProto(req.GetProviderModel()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
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
