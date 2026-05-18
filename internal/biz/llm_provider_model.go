package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/provider/inspect"

	"github.com/go-kratos/kratos/v2/errors"
)

var llmRandFallback uint64

func newLLMID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&llmRandFallback, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

// ProviderModel matches legacy PlatformResource for llm-provider-models.
type ProviderModel struct {
	ID           string
	Key          string // model_key
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	Provider     string
	Model        string
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

// ModelPricingRule matches domain.ModelPricingRule (subset used for Upsert).
type ModelPricingRule struct {
	ID                            string
	ProviderCode                  string
	ModelAPIID                    string
	Currency                      string
	InputPriceMicroUSDPer1K       int64
	OutputPriceMicroUSDPer1K      int64
	CachedInputPriceMicroUSDPer1K int64
	ReasoningPriceMicroUSDPer1K   int64
	EmbeddingPriceMicroUSDPer1K   int64
	EffectiveFrom                 string
	EffectiveTo                   string
	IsActive                      bool
	Source                        string
	MetadataJSON                  string
}

// InspectMerge holds inspect request fields (legacy InspectProviderModelInput).
type InspectMerge struct {
	ResourceID   string
	ProviderCode string
	ProviderType string
	ModelAPIID   string
	APIBaseURL   string
	APIKey       string
}

type providerPricingConfig struct {
	InputPriceMicroUSDPer1K       int64 `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64 `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64 `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64 `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64 `json:"embedding_price_micro_usd_per_1k"`
}

// LlmProviderModelRepo is persistence + pricing upsert backing provider models catalog.
type LlmProviderModelRepo interface {
	ListProviderModels(ctx context.Context) ([]ProviderModel, error)
	GetProviderModel(ctx context.Context, id string) (ProviderModel, error)
	GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
	CreateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
	UpdateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
	DeleteProviderModel(ctx context.Context, id string) error
	ValidateProviderPair(ctx context.Context, provider, model string) (bool, error)
	UpsertModelPricingRule(ctx context.Context, rule ModelPricingRule) error
}

// LlmProviderModelUsecase is llm-provider-models + validate + inspect.
type LlmProviderModelUsecase struct {
	repo LlmProviderModelRepo
}

func NewLlmProviderModelUsecase(repo LlmProviderModelRepo) *LlmProviderModelUsecase {
	return &LlmProviderModelUsecase{repo: repo}
}

func (u *LlmProviderModelUsecase) List(ctx context.Context) ([]ProviderModel, error) {
	return u.repo.ListProviderModels(ctx)
}

func (u *LlmProviderModelUsecase) Get(ctx context.Context, id string) (ProviderModel, error) {
	return u.repo.GetProviderModel(ctx, id)
}

// GetByProviderAndModel loads a catalog row by provider + model_api_id.
func (u *LlmProviderModelUsecase) GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error) {
	return u.repo.GetProviderModelByProviderAndModel(ctx, provider, model)
}

func (u *LlmProviderModelUsecase) Create(ctx context.Context, in ProviderModel) (ProviderModel, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return ProviderModel{}, errors.BadRequest("LLM_PROVIDER_MODEL", "key and name are required")
	}
	if in.ID == "" {
		in.ID = newLLMID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	out, err := u.repo.CreateProviderModel(ctx, in)
	if err != nil {
		return ProviderModel{}, err
	}
	_ = u.syncProviderModelPricing(ctx, out)
	return out, nil
}

func (u *LlmProviderModelUsecase) Update(ctx context.Context, id string, patch ProviderModel) (ProviderModel, error) {
	cur, err := u.repo.GetProviderModel(ctx, id)
	if err != nil {
		return ProviderModel{}, err
	}
	merged := cur
	patch.ID = id
	if patch.Key != "" {
		merged.Key = patch.Key
	}
	if patch.Name != "" {
		merged.Name = patch.Name
	}
	if patch.Status != "" {
		merged.Status = patch.Status
	}
	merged.Description = patch.Description
	merged.Enabled = patch.Enabled
	merged.SortOrder = patch.SortOrder
	merged.Provider = patch.Provider
	merged.Model = patch.Model
	if strings.TrimSpace(patch.ConfigJSON) != "" {
		merged.ConfigJSON = patch.ConfigJSON
	}
	if strings.TrimSpace(patch.MetadataJSON) != "" {
		merged.MetadataJSON = patch.MetadataJSON
	}
	if merged.Key == "" {
		merged.Key = cur.Key
	}
	if merged.Name == "" {
		merged.Name = cur.Name
	}
	if merged.Status == "" {
		merged.Status = cur.Status
	}
	out, err := u.repo.UpdateProviderModel(ctx, merged)
	if err != nil {
		return ProviderModel{}, err
	}
	_ = u.syncProviderModelPricing(ctx, out)
	return out, nil
}

func (u *LlmProviderModelUsecase) Delete(ctx context.Context, id string) error {
	return u.repo.DeleteProviderModel(ctx, id)
}

func (u *LlmProviderModelUsecase) ValidatePair(ctx context.Context, provider, model string) (bool, string, error) {
	ok, err := u.repo.ValidateProviderPair(ctx, provider, model)
	if err != nil {
		return false, "", err
	}
	if ok {
		return true, "model is available", nil
	}
	return false, "provider/model is not enabled", nil
}

func (u *LlmProviderModelUsecase) Inspect(ctx context.Context, in InspectMerge) (inspect.Result, error) {
	in.ProviderCode = strings.TrimSpace(in.ProviderCode)
	in.ModelAPIID = strings.TrimSpace(in.ModelAPIID)
	in.ResourceID = strings.TrimSpace(in.ResourceID)

	if u.needInspectMerge(in) {
		if in.ResourceID != "" {
			row, err := u.repo.GetProviderModel(ctx, in.ResourceID)
			if err == nil && row.ConfigJSON != "" {
				mergeInspectConfigJSON(row.ConfigJSON, &in)
			}
		} else if in.ProviderCode != "" && in.ModelAPIID != "" {
			row, err := u.repo.GetProviderModelByProviderAndModel(ctx, in.ProviderCode, in.ModelAPIID)
			if err == nil && row.ConfigJSON != "" {
				mergeInspectConfigJSON(row.ConfigJSON, &in)
			}
		}
	}

	return inspect.Run(inspect.Input{
		ResourceID:   in.ResourceID,
		ProviderCode: in.ProviderCode,
		ProviderType: in.ProviderType,
		ModelAPIID:   in.ModelAPIID,
		APIBaseURL:   in.APIBaseURL,
		APIKey:       in.APIKey,
	})
}

func (u *LlmProviderModelUsecase) needInspectMerge(in InspectMerge) bool {
	return !(in.APIBaseURL != "" && in.APIKey != "" && in.ProviderType != "")
}

func mergeInspectConfigJSON(cfg string, in *InspectMerge) {
	var c struct {
		ProviderType string `json:"provider_type"`
		APIBaseURL   string `json:"api_base_url"`
		APIKey       string `json:"api_key"`
	}
	if json.Unmarshal([]byte(cfg), &c) != nil {
		return
	}
	if in.ProviderType == "" {
		in.ProviderType = c.ProviderType
	}
	if in.APIBaseURL == "" {
		in.APIBaseURL = c.APIBaseURL
	}
	if in.APIKey == "" {
		in.APIKey = c.APIKey
	}
}

func (u *LlmProviderModelUsecase) syncProviderModelPricing(ctx context.Context, row ProviderModel) error {
	var cfg providerPricingConfig
	if json.Unmarshal([]byte(row.ConfigJSON), &cfg) != nil {
		return nil
	}
	if cfg.InputPriceMicroUSDPer1K == 0 && cfg.OutputPriceMicroUSDPer1K == 0 && cfg.CachedInputPriceMicroUSDPer1K == 0 &&
		cfg.ReasoningPriceMicroUSDPer1K == 0 && cfg.EmbeddingPriceMicroUSDPer1K == 0 {
		return nil
	}
	return u.repo.UpsertModelPricingRule(ctx, ModelPricingRule{
		ProviderCode:                  row.Provider,
		ModelAPIID:                    row.Model,
		Currency:                      "USD",
		InputPriceMicroUSDPer1K:       cfg.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      cfg.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: cfg.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   cfg.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   cfg.EmbeddingPriceMicroUSDPer1K,
		Source:                        "model-inspect",
		MetadataJSON:                  "{}",
	})
}
