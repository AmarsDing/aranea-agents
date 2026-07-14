package biz

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"
	"aranea-agents/pkg/safego"
)

var llmRandFallback uint64

// ErrProviderModelNotFound is returned when a provider-model row does not exist.
var ErrProviderModelNotFound = apierror.NotFound("LLM_PROVIDER_MODEL", "provider model not found")

func newLLMID() string {
	buf := make([]byte, 12)
	if _, err := cryptorand.Read(buf); err != nil {
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
	PricingConfigured    bool
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
}

// ModelCapabilities is the persisted provider-model capability catalog.
type ModelCapabilities struct {
	Text     bool `json:"text"`
	Vision   bool `json:"vision"`
	Audio    bool `json:"audio"`
	File     bool `json:"file"`
	ToolCall bool `json:"tool_call"`
	Cache    bool `json:"cache"`
	Thinking bool `json:"thinking"`
	TextOnly bool `json:"text_only"`
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

type LlmProviderModelReader interface {
	// Stability:stable
	ListProviderModels(ctx context.Context) ([]ProviderModel, error)
	GetProviderModel(ctx context.Context, id string) (ProviderModel, error)
	GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
}

type LlmProviderModelWriter interface {
	// Stability:stable
	CreateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
	UpdateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
	DeleteProviderModel(ctx context.Context, id string) error
	UpdateProviderModelStatus(ctx context.Context, id string, status string) error
}

type LlmProviderModelValidator interface {
	// Stability:stable
	ValidateProviderPair(ctx context.Context, provider, model string) (bool, error)
}

// ModelPricingRepo interface
type ModelPricingRepo interface {
	// Stability:stable
	UpsertModelPricingRule(ctx context.Context, rule ModelPricingRule) error
}

// LlmProviderModelReaderWriter combines Reader + Writer for consumers that need
// both read and write access. Each method count stays within the ≤5 limit
// because the sub-interfaces are already narrow.
type LlmProviderModelReaderWriter interface {
	LlmProviderModelReader
	LlmProviderModelWriter
}

// LlmProviderModelApplyBackend combines Reader + Writer + PricingRepo for the
// model-registry apply backend, which needs list/update and pricing upsert.
type LlmProviderModelApplyBackend interface {
	LlmProviderModelReader
	LlmProviderModelWriter
	ModelPricingRepo
}

// PricingSourcePriority returns the priority of a pricing source.
// Higher value = higher priority; lower-priority sources cannot overwrite higher ones.
// manual > model-inspect > models.dev-sync
func PricingSourcePriority(source string) int {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "manual":
		return 100
	case "model-inspect":
		return 50
	case "models.dev-sync":
		return 10
	default:
		return 0
	}
}

// LlmProviderModelRepo is the full data-layer interface that the repo struct
// satisfies. It exists only for the data-layer compile-time check and Wire
// provider; consumers should depend on the narrow sub-interfaces instead.
type LlmProviderModelRepo interface {
	LlmProviderModelReader
	LlmProviderModelWriter
	LlmProviderModelValidator
	ModelPricingRepo
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

// AgentReferenceChecker checks whether any agent references a given provider+model.
type AgentReferenceChecker interface {
	CountAgentsByProviderAndModel(ctx context.Context, provider, model string) (int, error)
}

type LlmProviderModelUsecase struct {
	reader    LlmProviderModelReader
	writer    LlmProviderModelWriter
	validator LlmProviderModelValidator
	pricing   ModelPricingRepo
	inspector LLMInspector
	crypto    *CredentialCrypto
	agentRefs AgentReferenceChecker
	// statsInjector 注入 30 天用量统计到 List 响应的 ConfigJSON（仅响应装饰，不持久化）。
	// 可为 nil：nil 时 List 跳过注入，保持向后兼容（用于不关心统计的调用方/测试）。
	statsInjector *ModelStatsInjector
	lg            loggateway.Logger
}

// NewLlmProviderModelUsecase 构造 Usecase。statsInjector 可为 nil。
// TECH-DEBT(CS-B7): 参数数量=9 超出 ≤5 上限。后续应迁移到 LlmProviderModelUsecaseDeps struct 模式
// （参考 AgentUsecaseDeps），本次仅追加 statsInjector 以解决 P1-1 统计注入需求，不顺带重构。
func NewLlmProviderModelUsecase(reader LlmProviderModelReader, writer LlmProviderModelWriter, validator LlmProviderModelValidator, pricing ModelPricingRepo, inspector LLMInspector, crypto *CredentialCrypto, agentRefs AgentReferenceChecker, statsInjector *ModelStatsInjector, lg loggateway.Logger) *LlmProviderModelUsecase {
	return &LlmProviderModelUsecase{reader: reader, writer: writer, validator: validator, pricing: pricing, inspector: inspector, crypto: crypto, agentRefs: agentRefs, statsInjector: statsInjector, lg: lg}
}

func (u *LlmProviderModelUsecase) List(ctx context.Context) ([]ProviderModel, error) {
	items, err := u.reader.ListProviderModels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i] = sanitizeProviderModelForAPI(items[i])
	}
	// 装饰阶段：注入 30 天用量统计到 ConfigJSON（仅响应装饰，不持久化）。
	// statsInjector 自身处理 nil reader / 错误（仅 Warn 日志，不影响主流程）。
	if u.statsInjector != nil {
		u.statsInjector.InjectStats(ctx, items)
	}
	return items, nil
}

func (u *LlmProviderModelUsecase) Get(ctx context.Context, id string) (ProviderModel, error) {
	m, err := u.reader.GetProviderModel(ctx, id)
	if err != nil {
		return ProviderModel{}, err
	}
	return sanitizeProviderModelForAPI(m), nil
}

// GetByProviderAndModel loads a catalog row by provider + model_api_id (decrypted for runtime).
func (u *LlmProviderModelUsecase) GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error) {
	m, err := u.reader.GetProviderModelByProviderAndModel(ctx, provider, model)
	if err != nil {
		return ProviderModel{}, err
	}
	return u.crypto.PrepareProviderModelForRuntime(ctx, m)
}

func (u *LlmProviderModelUsecase) Create(ctx context.Context, in ProviderModel) (ProviderModel, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return ProviderModel{}, apierror.BadRequest("LLM_PROVIDER_MODEL", "key and name are required")
	}
	if in.ID == "" {
		in.ID = newLLMID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := u.crypto.RequireKeyForPlaintext(ctx, in.ConfigJSON); err != nil {
		return ProviderModel{}, err
	}
	var err error
	in.ConfigJSON, err = u.crypto.ProcessConfigJSONForStorage(ctx, in.ConfigJSON)
	if err != nil {
		return ProviderModel{}, err
	}
	out, err := u.writer.CreateProviderModel(ctx, in)
	if err != nil {
		return ProviderModel{}, err
	}
	if err := u.syncProviderModelPricing(ctx, out); err != nil {
		// Pricing sync is a best-effort side effect — the model row itself is
		// already persisted, so we must not roll back. Log the failure so
		// operators can investigate; the PricingConfigured flag will remain
		// false until a subsequent update or inspect resolves the pricing.
		u.lg.Warn("syncProviderModelPricing failed, model created but pricing not synced",
			loggateway.StepID("llm_provider_model.pricing_sync"),
			loggateway.Str("model_id", out.ID),
			loggateway.Err(err))
	}
	return sanitizeProviderModelForAPI(out), nil
}

func (u *LlmProviderModelUsecase) Update(ctx context.Context, id string, patch ProviderModel) (ProviderModel, error) {
	cur, err := u.reader.GetProviderModel(ctx, id)
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
	if patch.Description != "" {
		merged.Description = patch.Description
	}
	merged.Enabled = patch.Enabled
	if patch.SortOrder != 0 {
		merged.SortOrder = patch.SortOrder
	}
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
		if err := u.crypto.RequireKeyForPlaintext(ctx, mergedCfg); err != nil {
			return ProviderModel{}, err
		}
		processed, err := u.crypto.ProcessConfigJSONForStorage(ctx, mergedCfg)
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
	out, err := u.writer.UpdateProviderModel(ctx, merged)
	if err != nil {
		return ProviderModel{}, err
	}
	if err := u.syncProviderModelPricing(ctx, out); err != nil {
		u.lg.Warn("syncProviderModelPricing failed, model updated but pricing not synced",
			loggateway.StepID("llm_provider_model.pricing_sync"),
			loggateway.Str("model_id", out.ID),
			loggateway.Err(err))
	}
	return sanitizeProviderModelForAPI(out), nil
}

func (u *LlmProviderModelUsecase) Delete(ctx context.Context, id string) error {
	m, err := u.reader.GetProviderModel(ctx, id)
	if err != nil {
		return err
	}
	// Fail-closed: a nil AgentReferenceChecker indicates misconfiguration.
	// Deleting a provider model that might be referenced by agents would
	// break runtime resolution, so we refuse to proceed.
	if u.agentRefs == nil {
		u.lg.Error("agent reference checker not configured, refusing to delete provider model",
			loggateway.StepID("llm_provider_model.agent_ref_check"),
			loggateway.Str("model_id", id))
		return apierror.Internal("LLM_PROVIDER_MODEL", "agent reference checker not configured; cannot safely delete provider model")
	}
	count, refErr := u.agentRefs.CountAgentsByProviderAndModel(ctx, m.Provider, m.Model)
	if refErr != nil {
		// Fail-closed: the reference check itself failed. Proceeding would
		// risk deleting a model still referenced by agents.
		u.lg.Error("agent reference check failed, refusing to delete provider model",
			loggateway.StepID("llm_provider_model.agent_ref_check"),
			loggateway.Str("model_id", id),
			loggateway.Err(refErr))
		return apierror.Internal("LLM_PROVIDER_MODEL", "agent reference check failed; cannot safely delete provider model")
	}
	if count > 0 {
		return apierror.Conflict("LLM_PROVIDER_MODEL", "cannot delete provider model referenced by %d agent(s); reassign agents first", count)
	}
	return u.writer.DeleteProviderModel(ctx, id)
}

// RevealCredentials returns decrypted credentials for admin edit UI (never logged).
func (u *LlmProviderModelUsecase) RevealCredentials(ctx context.Context, id string) (ProviderCredentialsReveal, error) {
	id, err := requireNonEmpty(id, "LLM_PROVIDER_MODEL", "id")
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	m, err := u.reader.GetProviderModel(ctx, id)
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	return u.crypto.RevealCredentialsFromConfig(ctx, m.ConfigJSON)
}

func (u *LlmProviderModelUsecase) ValidatePair(ctx context.Context, provider, model string) (bool, string, error) {
	ok, err := u.validator.ValidateProviderPair(ctx, provider, model)
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
			row, err := u.reader.GetProviderModel(ctx, in.ResourceID)
			if err == nil && row.ConfigJSON != "" {
				prepared, decErr := u.crypto.PrepareProviderModelForRuntime(ctx, row)
				if decErr != nil {
					u.lg.Warn("解密 config_json 失败", loggateway.StepID("llm_provider_model.inspect"), loggateway.Err(decErr))
				} else if mergeErr := mergeInspectConfigJSON(prepared.ConfigJSON, &in); mergeErr != nil {
					u.lg.Warn("合并 inspect config 失败", loggateway.StepID("llm_provider_model.inspect"), loggateway.Err(mergeErr))
				}
			}
		} else if in.ProviderCode != "" && in.ModelAPIID != "" {
			row, err := u.reader.GetProviderModelByProviderAndModel(ctx, in.ProviderCode, in.ModelAPIID)
			if err == nil && row.ConfigJSON != "" {
				prepared, decErr := u.crypto.PrepareProviderModelForRuntime(ctx, row)
				if decErr != nil {
					u.lg.Warn("解密 config_json 失败", loggateway.StepID("llm_provider_model.inspect"), loggateway.Err(decErr))
				} else if mergeErr := mergeInspectConfigJSON(prepared.ConfigJSON, &in); mergeErr != nil {
					u.lg.Warn("合并 inspect config 失败", loggateway.StepID("llm_provider_model.inspect"), loggateway.Err(mergeErr))
				}
			}
		}
	}

	if u.inspector == nil {
		return LLMInspectResult{}, apierror.Internal("LLM_INSPECT", "llm inspector not configured")
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

func mergeInspectConfigJSON(cfg string, in *InspectMerge) error {
	var c struct {
		ProviderType string `json:"provider_type"`
		APIBaseURL   string `json:"api_base_url"`
		APIKey       string `json:"api_key"`
		Variant      string `json:"variant"`
		SecretID     string `json:"secret_id"`
		SecretKey    string `json:"secret_key"`
		AWSRegion    string `json:"aws_region"`
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return apierror.BadRequest(apierror.DomainLLMProvider, "invalid provider config JSON: %s", err.Error())
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
	return nil
}

func parsePricingConfig(configJSON string) (providerPricingConfig, error) {
	var cfg providerPricingConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return cfg, apierror.BadRequest(apierror.DomainLLMProvider, "invalid pricing config JSON: %s", err.Error())
	}
	return cfg, nil
}

// costBlockHasValue checks all 6 pricing dimensions in the cost block.
// Must stay consistent with the fields mapped in costBlockToCostUSD;
// the original code only checked Input/Output which caused costUSD to
// ignore CacheRead/CacheWrite/Reasoning/Embedding when Input and Output were zero.
func costBlockHasValue(c providerCostBlock) bool {
	return c.InputUSDPer1M > 0 || c.OutputUSDPer1M > 0 || c.CacheReadUSDPer1M > 0 ||
		c.CacheWriteUSDPer1M > 0 || c.ReasoningUSDPer1M > 0 || c.EmbeddingUSDPer1M > 0
}

func costBlockToCostUSD(c providerCostBlock) modelregistry.CostUSDPer1M {
	return modelregistry.CostUSDPer1M{
		Input:      c.InputUSDPer1M,
		Output:     c.OutputUSDPer1M,
		CacheRead:  c.CacheReadUSDPer1M,
		CacheWrite: c.CacheWriteUSDPer1M,
		Reasoning:  c.ReasoningUSDPer1M,
		Embedding:  c.EmbeddingUSDPer1M,
	}
}

func buildMicroPricing(cfg providerPricingConfig) modelregistry.MicroPricing {
	micro := modelregistry.MicroPricing{
		Input:     cfg.InputPriceMicroUSDPer1K,
		Output:    cfg.OutputPriceMicroUSDPer1K,
		CacheRead: cfg.CachedInputPriceMicroUSDPer1K,
		Reasoning: cfg.ReasoningPriceMicroUSDPer1K,
		Embedding: cfg.EmbeddingPriceMicroUSDPer1K,
	}
	if costBlockHasValue(cfg.Cost) {
		micro = modelregistry.MicroPricingFromCostBlock(costBlockToCostUSD(cfg.Cost))
	}
	return micro
}

func isValidPricing(micro modelregistry.MicroPricing) bool {
	return micro.Input != 0 || micro.Output != 0 || micro.CacheRead != 0 ||
		micro.CacheWrite != 0 || micro.Reasoning != 0 || micro.Embedding != 0
}

func buildCostUSD(cfg providerPricingConfig, micro modelregistry.MicroPricing) modelregistry.CostUSDPer1M {
	if costBlockHasValue(cfg.Cost) {
		return costBlockToCostUSD(cfg.Cost)
	}
	return modelregistry.CostUSDPer1M{
		Input:      modelregistry.MicroPer1KToUSDPer1M(micro.Input),
		Output:     modelregistry.MicroPer1KToUSDPer1M(micro.Output),
		CacheRead:  modelregistry.MicroPer1KToUSDPer1M(micro.CacheRead),
		CacheWrite: modelregistry.MicroPer1KToUSDPer1M(micro.CacheWrite),
		Reasoning:  modelregistry.MicroPer1KToUSDPer1M(micro.Reasoning),
		Embedding:  modelregistry.MicroPer1KToUSDPer1M(micro.Embedding),
	}
}

func buildModelPricingRule(row ProviderModel, micro modelregistry.MicroPricing, costUSD modelregistry.CostUSDPer1M) ModelPricingRule {
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
	cfg, err := parsePricingConfig(row.ConfigJSON)
	if err != nil {
		return err
	}
	micro := buildMicroPricing(cfg)
	if !isValidPricing(micro) {
		return nil
	}
	costUSD := buildCostUSD(cfg, micro)
	rule := buildModelPricingRule(row, micro, costUSD)
	return u.pricing.UpsertModelPricingRule(ctx, rule)
}

// RunHealthChecks probes enabled provider models and marks unhealthy rows.
const healthCheckPoolSize = 5

// PickTitleModel selects a lightweight model from the list suitable for
// generating session titles. It prefers models with "mini", "flash",
// "lite", or "small" in their name; otherwise falls back to the first model.
// Returns false when the list is empty.
func PickTitleModel(models []ProviderModel) (ProviderModel, bool) {
	if len(models) == 0 {
		return ProviderModel{}, false
	}
	for _, m := range models {
		name := strings.ToLower(m.Model)
		if strings.Contains(name, "mini") || strings.Contains(name, "flash") || strings.Contains(name, "lite") || strings.Contains(name, "small") {
			return m, true
		}
	}
	return models[0], true
}

func (u *LlmProviderModelUsecase) RunHealthChecks(ctx context.Context) error {
	items, err := u.reader.ListProviderModels(ctx)
	if err != nil {
		return err
	}
	client := outboundguard.NewClient(10 * time.Second)
	sem := make(chan struct{}, healthCheckPoolSize)
	var wg sync.WaitGroup
	for _, row := range items {
		if !row.Enabled || row.DeletedAt != "" {
			continue
		}
		cfg, decErr := u.crypto.PrepareProviderModelForRuntime(ctx, row)
		if decErr != nil {
			u.lg.Warn("解密 config_json 失败", loggateway.StepID("provider.health"), loggateway.Str("model_id", row.ID), loggateway.Err(decErr))
			continue
		}
		var c struct {
			APIBaseURL string `json:"api_base_url"`
		}
		if jsonErr := json.Unmarshal([]byte(cfg.ConfigJSON), &c); jsonErr != nil {
			u.lg.Warn("解析 config_json 失败，标记为 degraded", loggateway.StepID("provider.health"), loggateway.Str("model_id", row.ID), loggateway.Err(jsonErr))
			writeCtx := context.WithoutCancel(ctx)
			if updErr := u.writer.UpdateProviderModelStatus(writeCtx, row.ID, "degraded"); updErr != nil {
				u.lg.Warn("update degraded status failed", loggateway.StepID("provider.health"), loggateway.Str("model_id", row.ID), loggateway.Err(updErr))
			}
			continue
		}
		base := strings.TrimSpace(c.APIBaseURL)
		if base == "" {
			continue
		}
		if err := outboundguard.ValidateURL(base); err != nil {
			writeCtx := context.WithoutCancel(ctx)
			if updErr := u.writer.UpdateProviderModelStatus(writeCtx, row.ID, "degraded"); updErr != nil {
				u.lg.Warn("update degraded status failed", loggateway.StepID("provider.health"), loggateway.Str("model_id", row.ID), loggateway.Err(updErr))
			}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		checkBase := base
		checkRowID := row.ID
		checkCurrentStatus := row.Status
		safego.Go(ctx, "provider.health_check", func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			writeCtx := context.WithoutCancel(ctx)
			jitter := time.Duration(rand.IntN(500)) * time.Millisecond
			time.Sleep(jitter)
			// Use writeCtx (WithoutCancel) so the health check HTTP request
			// is not canceled when the parent request context is done.
			req, err := http.NewRequestWithContext(writeCtx, http.MethodHead, checkBase, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode >= 500 {
				if resp != nil {
					resp.Body.Close()
				}
				if updErr := u.writer.UpdateProviderModelStatus(writeCtx, checkRowID, "degraded"); updErr != nil {
					u.lg.Warn("update degraded status failed", loggateway.StepID("provider.health"), loggateway.Str("model_id", checkRowID), loggateway.Err(updErr))
				}
				return
			}
			resp.Body.Close()
			if checkCurrentStatus == "degraded" {
				if updErr := u.writer.UpdateProviderModelStatus(writeCtx, checkRowID, "active"); updErr != nil {
					u.lg.Warn("update active status failed", loggateway.StepID("provider.health"), loggateway.Str("model_id", checkRowID), loggateway.Err(updErr))
				}
			}
		})
	}
	wg.Wait()
	return nil
}
