package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/modelcatalog"
	"aranea-agents/pkg/outboundguard"

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
	ID                   string
	Key                  string // model_key
	Name                 string
	Description          string
	Status               string
	Enabled              bool
	SortOrder            int
	Provider             string
	Model                string
	ConfigJSON           string
	MetadataJSON         string
	Capabilities         ModelCapabilities
	CapabilitiesExplicit bool
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
}

// ModelCapabilities is the persisted provider-model capability catalog.
type ModelCapabilities struct {
	Text     bool
	Vision   bool
	Audio    bool
	File     bool
	ToolCall bool
	Cache    bool
	Thinking bool
	TextOnly bool
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
	CacheWritePriceMicroUSDPer1K  int64
	ReasoningPriceMicroUSDPer1K   int64
	EmbeddingPriceMicroUSDPer1K   int64
	InputPriceUSDPer1M            float64
	OutputPriceUSDPer1M           float64
	CachedInputPriceUSDPer1M      float64
	CacheWritePriceUSDPer1M       float64
	ReasoningPriceUSDPer1M        float64
	EmbeddingPriceUSDPer1M        float64
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
	Variant      string
	SecretID     string
	SecretKey    string
	AWSRegion    string
}

type providerPricingConfig struct {
	Cost                          providerCostBlock `json:"cost"`
	InputPriceMicroUSDPer1K       int64             `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64             `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64             `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64             `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64             `json:"embedding_price_micro_usd_per_1k"`
}

type providerCostBlock struct {
	InputUSDPer1M      float64 `json:"input_usd_per_1m"`
	OutputUSDPer1M     float64 `json:"output_usd_per_1m"`
	CacheReadUSDPer1M  float64 `json:"cache_read_usd_per_1m"`
	CacheWriteUSDPer1M float64 `json:"cache_write_usd_per_1m"`
	ReasoningUSDPer1M  float64 `json:"reasoning_usd_per_1m"`
	EmbeddingUSDPer1M  float64 `json:"embedding_usd_per_1m"`
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

// LLMInspectResult is the domain-level result of an LLM provider inspect.
type LLMInspectResult struct {
	OK                            bool
	Message                       string
	ProviderCode                  string
	ProviderType                  string
	ModelAPIID                    string
	ModelDisplayName              string
	ModelSizeLabel                string
	ContextWindowK                int
	MaxOutputTokens               int
	InputPriceMicroUSDPer1K       int64
	OutputPriceMicroUSDPer1K      int64
	CachedInputPriceMicroUSDPer1K int64
	ReasoningPriceMicroUSDPer1K   int64
	EmbeddingPriceMicroUSDPer1K   int64
	Source                        string
	RawMetadataJSON               string
	Variant                       string
	EnableTokenTailoring          bool
	SupportsCache                 bool
	SupportsThinking              bool
}

// LLMInspector abstracts the LLM provider metadata inspection.
type LLMInspector interface {
	Run(in InspectMerge) (LLMInspectResult, error)
}

// LlmProviderModelUsecase is llm-provider-models + validate + inspect.
type LlmProviderModelUsecase struct {
	repo      LlmProviderModelRepo
	inspector LLMInspector
}

func NewLlmProviderModelUsecase(repo LlmProviderModelRepo) *LlmProviderModelUsecase {
	return &LlmProviderModelUsecase{repo: repo}
}

func (u *LlmProviderModelUsecase) SetInspector(inspector LLMInspector) {
	u.inspector = inspector
}

func (u *LlmProviderModelUsecase) List(ctx context.Context) ([]ProviderModel, error) {
	items, err := u.repo.ListProviderModels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i] = sanitizeProviderModelForAPI(items[i])
	}
	return items, nil
}

func (u *LlmProviderModelUsecase) Get(ctx context.Context, id string) (ProviderModel, error) {
	m, err := u.repo.GetProviderModel(ctx, id)
	if err != nil {
		return ProviderModel{}, err
	}
	return sanitizeProviderModelForAPI(m), nil
}

// GetByProviderAndModel loads a catalog row by provider + model_api_id (decrypted for runtime).
func (u *LlmProviderModelUsecase) GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error) {
	m, err := u.repo.GetProviderModelByProviderAndModel(ctx, provider, model)
	if err != nil {
		return ProviderModel{}, err
	}
	return prepareProviderModelForRuntime(ctx, m), nil
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
	if err := requireCredentialKeyForPlaintext(ctx, in.ConfigJSON); err != nil {
		return ProviderModel{}, err
	}
	var err error
	in.ConfigJSON, err = processConfigJSONForStorage(ctx, in.ConfigJSON)
	if err != nil {
		return ProviderModel{}, err
	}
	out, err := u.repo.CreateProviderModel(ctx, in)
	if err != nil {
		return ProviderModel{}, err
	}
	_ = u.syncProviderModelPricing(ctx, out)
	return sanitizeProviderModelForAPI(out), nil
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
	if p := strings.TrimSpace(patch.Provider); p != "" {
		merged.Provider = p
	}
	if m := strings.TrimSpace(patch.Model); m != "" {
		merged.Model = m
	}
	if strings.TrimSpace(patch.ConfigJSON) != "" {
		mergedCfg, err := mergeConfigJSONForUpdate(cur.ConfigJSON, patch.ConfigJSON)
		if err != nil {
			return ProviderModel{}, err
		}
		if err := requireCredentialKeyForPlaintext(ctx, mergedCfg); err != nil {
			return ProviderModel{}, err
		}
		processed, err := processConfigJSONForStorage(ctx, mergedCfg)
		if err != nil {
			return ProviderModel{}, err
		}
		merged.ConfigJSON = processed
	}
	if strings.TrimSpace(patch.MetadataJSON) != "" {
		merged.MetadataJSON = patch.MetadataJSON
	}
	if patch.CapabilitiesExplicit {
		merged.Capabilities = patch.Capabilities
		merged.CapabilitiesExplicit = true
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
	return sanitizeProviderModelForAPI(out), nil
}

func (u *LlmProviderModelUsecase) Delete(ctx context.Context, id string) error {
	return u.repo.DeleteProviderModel(ctx, id)
}

// RevealCredentials returns decrypted credentials for admin edit UI (never logged).
func (u *LlmProviderModelUsecase) RevealCredentials(ctx context.Context, id string) (ProviderCredentialsReveal, error) {
	id, err := requireNonEmpty(id, "LLM_PROVIDER_MODEL", "id")
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	m, err := u.repo.GetProviderModel(ctx, id)
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	return revealCredentialsFromConfig(ctx, m.ConfigJSON)
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

func (u *LlmProviderModelUsecase) Inspect(ctx context.Context, in InspectMerge) (LLMInspectResult, error) {
	in.ProviderCode = strings.TrimSpace(in.ProviderCode)
	in.ModelAPIID = strings.TrimSpace(in.ModelAPIID)
	in.ResourceID = strings.TrimSpace(in.ResourceID)

	if u.needInspectMerge(in) {
		if in.ResourceID != "" {
			row, err := u.repo.GetProviderModel(ctx, in.ResourceID)
			if err == nil && row.ConfigJSON != "" {
				mergeInspectConfigJSON(prepareProviderModelForRuntime(ctx, row).ConfigJSON, &in)
			}
		} else if in.ProviderCode != "" && in.ModelAPIID != "" {
			row, err := u.repo.GetProviderModelByProviderAndModel(ctx, in.ProviderCode, in.ModelAPIID)
			if err == nil && row.ConfigJSON != "" {
				mergeInspectConfigJSON(prepareProviderModelForRuntime(ctx, row).ConfigJSON, &in)
			}
		}
	}

	if u.inspector == nil {
		return LLMInspectResult{}, errors.New(500, "LLM_INSPECT", "llm inspector not configured")
	}
	return u.inspector.Run(in)
}

func (u *LlmProviderModelUsecase) needInspectMerge(in InspectMerge) bool {
	if in.ProviderType == "" || in.APIBaseURL == "" {
		return true
	}
	return !inspectCredentialsComplete(in)
}

func inspectCredentialsComplete(in InspectMerge) bool {
	pt := strings.ToLower(strings.TrimSpace(in.ProviderType))
	if strings.TrimSpace(in.APIKey) != "" {
		return true
	}
	if pt == "hunyuan" {
		return strings.TrimSpace(in.SecretID) != "" && strings.TrimSpace(in.SecretKey) != ""
	}
	if pt == "bedrock" {
		return strings.TrimSpace(in.AWSRegion) != ""
	}
	if pt == "ollama" {
		return true
	}
	return false
}

func mergeInspectConfigJSON(cfg string, in *InspectMerge) {
	var c struct {
		ProviderType string `json:"provider_type"`
		APIBaseURL   string `json:"api_base_url"`
		APIKey       string `json:"api_key"`
		Variant      string `json:"variant"`
		SecretID     string `json:"secret_id"`
		SecretKey    string `json:"secret_key"`
		AWSRegion    string `json:"aws_region"`
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
	if in.Variant == "" {
		in.Variant = c.Variant
	}
	if in.SecretID == "" {
		in.SecretID = c.SecretID
	}
	if in.SecretKey == "" {
		in.SecretKey = c.SecretKey
	}
	if in.AWSRegion == "" {
		in.AWSRegion = c.AWSRegion
	}
}

func parsePricingConfig(configJSON string) (providerPricingConfig, bool) {
	var cfg providerPricingConfig
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return cfg, false
	}
	return cfg, true
}

// costBlockHasValue checks all 6 pricing dimensions in the cost block.
// Must stay consistent with the fields mapped in costBlockToCostUSD;
// the original code only checked Input/Output which caused costUSD to
// ignore CacheRead/CacheWrite/Reasoning/Embedding when Input and Output were zero.
func costBlockHasValue(c providerCostBlock) bool {
	return c.InputUSDPer1M > 0 || c.OutputUSDPer1M > 0 || c.CacheReadUSDPer1M > 0 ||
		c.CacheWriteUSDPer1M > 0 || c.ReasoningUSDPer1M > 0 || c.EmbeddingUSDPer1M > 0
}

func costBlockToCostUSD(c providerCostBlock) modelcatalog.CostUSDPer1M {
	return modelcatalog.CostUSDPer1M{
		Input:      c.InputUSDPer1M,
		Output:     c.OutputUSDPer1M,
		CacheRead:  c.CacheReadUSDPer1M,
		CacheWrite: c.CacheWriteUSDPer1M,
		Reasoning:  c.ReasoningUSDPer1M,
		Embedding:  c.EmbeddingUSDPer1M,
	}
}

func buildMicroPricing(cfg providerPricingConfig) modelcatalog.MicroPricing {
	micro := modelcatalog.MicroPricing{
		Input:     cfg.InputPriceMicroUSDPer1K,
		Output:    cfg.OutputPriceMicroUSDPer1K,
		CacheRead: cfg.CachedInputPriceMicroUSDPer1K,
		Reasoning: cfg.ReasoningPriceMicroUSDPer1K,
		Embedding: cfg.EmbeddingPriceMicroUSDPer1K,
	}
	if costBlockHasValue(cfg.Cost) {
		micro = modelcatalog.MicroPricingFromCostBlock(costBlockToCostUSD(cfg.Cost))
	}
	return micro
}

func isValidPricing(micro modelcatalog.MicroPricing) bool {
	return micro.Input != 0 || micro.Output != 0 || micro.CacheRead != 0 ||
		micro.CacheWrite != 0 || micro.Reasoning != 0 || micro.Embedding != 0
}

func buildCostUSD(cfg providerPricingConfig, micro modelcatalog.MicroPricing) modelcatalog.CostUSDPer1M {
	if costBlockHasValue(cfg.Cost) {
		return costBlockToCostUSD(cfg.Cost)
	}
	return modelcatalog.CostUSDPer1M{
		Input:      modelcatalog.MicroPer1KToUSDPer1M(micro.Input),
		Output:     modelcatalog.MicroPer1KToUSDPer1M(micro.Output),
		CacheRead:  modelcatalog.MicroPer1KToUSDPer1M(micro.CacheRead),
		CacheWrite: modelcatalog.MicroPer1KToUSDPer1M(micro.CacheWrite),
		Reasoning:  modelcatalog.MicroPer1KToUSDPer1M(micro.Reasoning),
		Embedding:  modelcatalog.MicroPer1KToUSDPer1M(micro.Embedding),
	}
}

func buildModelPricingRule(row ProviderModel, micro modelcatalog.MicroPricing, costUSD modelcatalog.CostUSDPer1M) ModelPricingRule {
	return ModelPricingRule{
		ProviderCode:                  row.Provider,
		ModelAPIID:                    row.Model,
		Currency:                      "USD",
		InputPriceMicroUSDPer1K:       micro.Input,
		OutputPriceMicroUSDPer1K:      micro.Output,
		CachedInputPriceMicroUSDPer1K: micro.CacheRead,
		CacheWritePriceMicroUSDPer1K:  micro.CacheWrite,
		ReasoningPriceMicroUSDPer1K:   micro.Reasoning,
		EmbeddingPriceMicroUSDPer1K:   micro.Embedding,
		InputPriceUSDPer1M:            costUSD.Input,
		OutputPriceUSDPer1M:           costUSD.Output,
		CachedInputPriceUSDPer1M:      costUSD.CacheRead,
		CacheWritePriceUSDPer1M:       costUSD.CacheWrite,
		ReasoningPriceUSDPer1M:        costUSD.Reasoning,
		EmbeddingPriceUSDPer1M:        costUSD.Embedding,
		Source:                        "model-inspect",
		MetadataJSON:                  "{}",
	}
}

func (u *LlmProviderModelUsecase) syncProviderModelPricing(ctx context.Context, row ProviderModel) error {
	cfg, ok := parsePricingConfig(row.ConfigJSON)
	if !ok {
		return nil
	}
	micro := buildMicroPricing(cfg)
	if !isValidPricing(micro) {
		return nil
	}
	costUSD := buildCostUSD(cfg, micro)
	rule := buildModelPricingRule(row, micro, costUSD)
	return u.repo.UpsertModelPricingRule(ctx, rule)
}

// RunHealthChecks probes enabled provider models and marks unhealthy rows.
func (u *LlmProviderModelUsecase) RunHealthChecks(ctx context.Context) error {
	items, err := u.repo.ListProviderModels(ctx)
	if err != nil {
		return err
	}
	client := outboundguard.NewClient(10 * time.Second)
	for _, row := range items {
		if !row.Enabled || row.DeletedAt != "" {
			continue
		}
		cfg := prepareProviderModelForRuntime(ctx, row)
		var c struct {
			APIBaseURL string `json:"api_base_url"`
		}
		_ = json.Unmarshal([]byte(cfg.ConfigJSON), &c)
		base := strings.TrimSpace(c.APIBaseURL)
		if base == "" {
			continue
		}
		if err := outboundguard.ValidateURL(base); err != nil {
			row.Status = "degraded"
			_, _ = u.repo.UpdateProviderModel(ctx, row)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, base, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode >= 500 {
			row.Status = "degraded"
			_, _ = u.repo.UpdateProviderModel(ctx, row)
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		resp.Body.Close()
		if row.Status == "degraded" {
			row.Status = "active"
			_, _ = u.repo.UpdateProviderModel(ctx, row)
		}
	}
	return nil
}
