package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/llmprovidermodel"
	"aranea-agents/internal/data/ent/modelpricingrule"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type llmProviderModelRepo struct {
	data *Data
}

var _ biz.LlmProviderModelRepo = (*llmProviderModelRepo)(nil)

func NewLlmProviderModelRepo(d *Data) biz.LlmProviderModelRepo {
	return &llmProviderModelRepo{data: d}
}

func entToBizPM(lg loggateway.Logger, e *ent.LlmProviderModel) biz.ProviderModel {
	if e == nil {
		return biz.ProviderModel{}
	}
	return biz.ProviderModel{
		ID:           e.ID,
		Key:          e.ModelKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		Provider:     e.Provider,
		Model:        e.Model,
		ConfigJSON:   e.ConfigJSON,
		MetadataJSON: e.MetadataJSON,
		Capabilities: biz.ModelCapabilities{
			Text:     e.CapabilityText,
			Vision:   e.CapabilityVision,
			Audio:    e.CapabilityAudio,
			File:     e.CapabilityFile,
			ToolCall: e.CapabilityToolCall,
			Cache:    e.CapabilityCache,
			Thinking: e.CapabilityThinking,
			TextOnly: e.CapabilityTextOnly,
		},
		CapabilitiesExplicit: e.CapabilitiesExplicit,
		PricingConfigured:    configJSONHasPricing(lg, e.ConfigJSON),
		CreatedAt:            e.CreatedAt,
		UpdatedAt:            e.UpdatedAt,
		DeletedAt:            e.DeletedAt,
	}
}

func (r *llmProviderModelRepo) readClient(ctx context.Context) *ent.Client {
	if c, ok := ctx.Value(txClientKey{}).(*ent.Client); ok {
		return c
	}
	return r.data.ReadEnt()
}

func (r *llmProviderModelRepo) ListProviderModels(ctx context.Context) ([]biz.ProviderModel, error) {
	rows, err := r.readClient(ctx).LlmProviderModel.Query().
		Where(llmprovidermodel.DeletedAtEQ("")).
		Order(
			llmprovidermodel.BySortOrder(),
			llmprovidermodel.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ProviderModel, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizPM(r.data.lg, e))
	}
	return out, nil
}

func (r *llmProviderModelRepo) GetProviderModel(ctx context.Context, id string) (biz.ProviderModel, error) {
	row, err := r.readClient(ctx).LlmProviderModel.Query().
		Where(llmprovidermodel.IDEQ(id), llmprovidermodel.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.ProviderModel{}, sql.ErrNoRows
		}
		return biz.ProviderModel{}, err
	}
	return entToBizPM(r.data.lg, row), nil
}

func (r *llmProviderModelRepo) GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error) {
	row, err := r.readClient(ctx).LlmProviderModel.Query().
		Where(
			llmprovidermodel.ProviderEQ(provider),
			llmprovidermodel.ModelEQ(model),
			llmprovidermodel.EnabledEQ(true),
			llmprovidermodel.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.ProviderModel{}, sql.ErrNoRows
		}
		return biz.ProviderModel{}, err
	}
	return entToBizPM(r.data.lg, row), nil
}

func (r *llmProviderModelRepo) ValidateProviderPair(ctx context.Context, provider, model string) (bool, error) {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return false, nil
	}
	n, err := r.readClient(ctx).LlmProviderModel.Query().
		Where(
			llmprovidermodel.ProviderEQ(strings.TrimSpace(provider)),
			llmprovidermodel.ModelEQ(strings.TrimSpace(model)),
			llmprovidermodel.EnabledEQ(true),
			llmprovidermodel.DeletedAtEQ(""),
		).
		Count(ctx)
	return n > 0, err
}

func (r *llmProviderModelRepo) CreateProviderModel(ctx context.Context, m biz.ProviderModel) (biz.ProviderModel, error) {
	now := nowRFC3339()
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	saved, err := r.data.entClient.LlmProviderModel.Create().
		SetID(m.ID).
		SetModelKey(m.Key).
		SetName(m.Name).
		SetDescription(m.Description).
		SetStatus(m.Status).
		SetEnabled(m.Enabled).
		SetSortOrder(m.SortOrder).
		SetProvider(m.Provider).
		SetModel(m.Model).
		SetConfigJSON(m.ConfigJSON).
		SetMetadataJSON(m.MetadataJSON).
		SetCapabilityText(m.Capabilities.Text).
		SetCapabilityVision(m.Capabilities.Vision).
		SetCapabilityAudio(m.Capabilities.Audio).
		SetCapabilityFile(m.Capabilities.File).
		SetCapabilityToolCall(m.Capabilities.ToolCall).
		SetCapabilityCache(m.Capabilities.Cache).
		SetCapabilityThinking(m.Capabilities.Thinking).
		SetCapabilityTextOnly(m.Capabilities.TextOnly).
		SetCapabilitiesExplicit(m.CapabilitiesExplicit).
		SetCreatedAt(m.CreatedAt).
		SetUpdatedAt(m.UpdatedAt).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.ProviderModel{}, err
	}
	return entToBizPM(r.data.lg, saved), nil
}

func (r *llmProviderModelRepo) UpdateProviderModel(ctx context.Context, m biz.ProviderModel) (biz.ProviderModel, error) {
	m.UpdatedAt = nowRFC3339()
	err := r.data.entClient.LlmProviderModel.UpdateOneID(m.ID).
		SetModelKey(m.Key).
		SetName(m.Name).
		SetDescription(m.Description).
		SetStatus(m.Status).
		SetEnabled(m.Enabled).
		SetSortOrder(m.SortOrder).
		SetProvider(m.Provider).
		SetModel(m.Model).
		SetConfigJSON(m.ConfigJSON).
		SetMetadataJSON(m.MetadataJSON).
		SetCapabilityText(m.Capabilities.Text).
		SetCapabilityVision(m.Capabilities.Vision).
		SetCapabilityAudio(m.Capabilities.Audio).
		SetCapabilityFile(m.Capabilities.File).
		SetCapabilityToolCall(m.Capabilities.ToolCall).
		SetCapabilityCache(m.Capabilities.Cache).
		SetCapabilityThinking(m.Capabilities.Thinking).
		SetCapabilityTextOnly(m.Capabilities.TextOnly).
		SetCapabilitiesExplicit(m.CapabilitiesExplicit).
		SetUpdatedAt(m.UpdatedAt).
		Exec(ctx)
	if err != nil {
		return biz.ProviderModel{}, err
	}
	return r.GetProviderModel(ctx, m.ID)
}

func (r *llmProviderModelRepo) DeleteProviderModel(ctx context.Context, id string) error {
	now := nowRFC3339()
	return r.data.entClient.LlmProviderModel.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}

func (r *llmProviderModelRepo) UpdateProviderModelStatus(ctx context.Context, id string, status string) error {
	now := nowRFC3339()
	return r.data.entClient.LlmProviderModel.UpdateOneID(id).
		SetStatus(status).
		SetUpdatedAt(now).
		Exec(ctx)
}

func (r *llmProviderModelRepo) UpsertModelPricingRule(ctx context.Context, rule biz.ModelPricingRule) error {
	if strings.TrimSpace(rule.ProviderCode) == "" || strings.TrimSpace(rule.ModelAPIID) == "" {
		return errors.New("provider_code and model_api_id are required")
	}
	now := nowRFC3339()
	if rule.Currency == "" {
		rule.Currency = "USD"
	}
	if rule.EffectiveFrom == "" {
		rule.EffectiveFrom = now
	}
	if rule.Source == "" {
		rule.Source = "manual"
	}
	if rule.MetadataJSON == "" {
		rule.MetadataJSON = "{}"
	}
	tx, err := r.data.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	client := tx.Client()
	row, err := client.ModelPricingRule.Query().
		Where(
			modelpricingrule.ProviderCodeEQ(rule.ProviderCode),
			modelpricingrule.ModelAPIIDEQ(rule.ModelAPIID),
			modelpricingrule.IsActiveEQ(true),
			modelpricingrule.EffectiveToEQ(""),
		).
		Only(ctx)
	if err == nil {
		if biz.PricingSourcePriority(rule.Source) < biz.PricingSourcePriority(row.Source) {
			_ = tx.Rollback()
			return nil
		}
		err = client.ModelPricingRule.UpdateOneID(row.ID).
			SetCurrency(rule.Currency).
			SetInputPriceMicroUsdPer1k(rule.InputPriceMicroUSDPer1K).
			SetOutputPriceMicroUsdPer1k(rule.OutputPriceMicroUSDPer1K).
			SetCachedInputPriceMicroUsdPer1k(rule.CachedInputPriceMicroUSDPer1K).
			SetCacheWritePriceMicroUsdPer1k(rule.CacheWritePriceMicroUSDPer1K).
			SetReasoningPriceMicroUsdPer1k(rule.ReasoningPriceMicroUSDPer1K).
			SetEmbeddingPriceMicroUsdPer1k(rule.EmbeddingPriceMicroUSDPer1K).
			SetInputPriceUsdPer1m(rule.InputPriceUSDPer1M).
			SetOutputPriceUsdPer1m(rule.OutputPriceUSDPer1M).
			SetCachedInputPriceUsdPer1m(rule.CachedInputPriceUSDPer1M).
			SetCacheWritePriceUsdPer1m(rule.CacheWritePriceUSDPer1M).
			SetReasoningPriceUsdPer1m(rule.ReasoningPriceUSDPer1M).
			SetEmbeddingPriceUsdPer1m(rule.EmbeddingPriceUSDPer1M).
			SetSource(rule.Source).
			SetMetadataJSON(rule.MetadataJSON).
			SetUpdatedAt(now).
			Exec(ctx)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	if !ent.IsNotFound(err) {
		_ = tx.Rollback()
		return err
	}
	rid := rule.ID
	if rid == "" {
		rid = fmt.Sprintf("pricing:%s:%s:%d", rule.ProviderCode, strings.ReplaceAll(rule.ModelAPIID, "/", "_"), time.Now().UTC().UnixNano())
	}
	rule.IsActive = true
	_, err = client.ModelPricingRule.Create().
		SetID(rid).
		SetProviderCode(rule.ProviderCode).
		SetModelAPIID(rule.ModelAPIID).
		SetCurrency(rule.Currency).
		SetInputPriceMicroUsdPer1k(rule.InputPriceMicroUSDPer1K).
		SetOutputPriceMicroUsdPer1k(rule.OutputPriceMicroUSDPer1K).
		SetCachedInputPriceMicroUsdPer1k(rule.CachedInputPriceMicroUSDPer1K).
		SetCacheWritePriceMicroUsdPer1k(rule.CacheWritePriceMicroUSDPer1K).
		SetReasoningPriceMicroUsdPer1k(rule.ReasoningPriceMicroUSDPer1K).
		SetEmbeddingPriceMicroUsdPer1k(rule.EmbeddingPriceMicroUSDPer1K).
		SetInputPriceUsdPer1m(rule.InputPriceUSDPer1M).
		SetOutputPriceUsdPer1m(rule.OutputPriceUSDPer1M).
		SetCachedInputPriceUsdPer1m(rule.CachedInputPriceUSDPer1M).
		SetCacheWritePriceUsdPer1m(rule.CacheWritePriceUSDPer1M).
		SetReasoningPriceUsdPer1m(rule.ReasoningPriceUSDPer1M).
		SetEmbeddingPriceUsdPer1m(rule.EmbeddingPriceUSDPer1M).
		SetEffectiveFrom(rule.EffectiveFrom).
		SetEffectiveTo("").
		SetIsActive(rule.IsActive).
		SetSource(rule.Source).
		SetMetadataJSON(rule.MetadataJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func configJSONHasPricing(lg loggateway.Logger, cfg string) bool {
	cfg = strings.TrimSpace(cfg)
	if cfg == "" || cfg == "{}" {
		return false
	}
	var m struct {
		InputPriceMicroUSDPer1K       int64 `json:"input_price_micro_usd_per_1k"`
		OutputPriceMicroUSDPer1K      int64 `json:"output_price_micro_usd_per_1k"`
		CachedInputPriceMicroUSDPer1K int64 `json:"cached_input_price_micro_usd_per_1k"`
		ReasoningPriceMicroUSDPer1K   int64 `json:"reasoning_price_micro_usd_per_1k"`
		EmbeddingPriceMicroUSDPer1K   int64 `json:"embedding_price_micro_usd_per_1k"`
		Cost                          *struct {
			InputUSDPer1M      float64 `json:"input_usd_per_1m"`
			OutputUSDPer1M     float64 `json:"output_usd_per_1m"`
			CacheReadUSDPer1M  float64 `json:"cache_read_usd_per_1m"`
			CacheWriteUSDPer1M float64 `json:"cache_write_usd_per_1m"`
			ReasoningUSDPer1M  float64 `json:"reasoning_usd_per_1m"`
			EmbeddingUSDPer1M  float64 `json:"embedding_usd_per_1m"`
		} `json:"cost"`
	}
	if err := json.Unmarshal([]byte(cfg), &m); err != nil {
		lg.Warn("unmarshal provider model config failed", loggateway.StepID("data.llm_provider_model"), loggateway.Err(err))
		return false
	}
	if m.InputPriceMicroUSDPer1K > 0 || m.OutputPriceMicroUSDPer1K > 0 ||
		m.CachedInputPriceMicroUSDPer1K > 0 || m.ReasoningPriceMicroUSDPer1K > 0 ||
		m.EmbeddingPriceMicroUSDPer1K > 0 {
		return true
	}
	if m.Cost != nil {
		if m.Cost.InputUSDPer1M > 0 || m.Cost.OutputUSDPer1M > 0 ||
			m.Cost.CacheReadUSDPer1M > 0 || m.Cost.CacheWriteUSDPer1M > 0 ||
			m.Cost.ReasoningUSDPer1M > 0 || m.Cost.EmbeddingUSDPer1M > 0 {
			return true
		}
	}
	return false
}
