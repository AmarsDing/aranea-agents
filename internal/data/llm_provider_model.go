package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/llmprovidermodel"
	"aranea-agents/internal/data/ent/modelpricingrule"

	entsql "entgo.io/ent/dialect/sql"
)

type llmProviderModelRepo struct {
	data *Data
}

func NewLlmProviderModelRepo(d *Data) biz.LlmProviderModelRepo {
	return &llmProviderModelRepo{data: d}
}

func entToBizPM(e *ent.LlmProviderModel) biz.ProviderModel {
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
		CreatedAt:            e.CreatedAt,
		UpdatedAt:            e.UpdatedAt,
		DeletedAt:            e.DeletedAt,
	}
}

func (r *llmProviderModelRepo) ListProviderModels(ctx context.Context) ([]biz.ProviderModel, error) {
	rows, err := r.data.entClient.LlmProviderModel.Query().
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
		out = append(out, entToBizPM(e))
	}
	return out, nil
}

func (r *llmProviderModelRepo) GetProviderModel(ctx context.Context, id string) (biz.ProviderModel, error) {
	row, err := r.data.entClient.LlmProviderModel.Query().
		Where(llmprovidermodel.IDEQ(id), llmprovidermodel.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.ProviderModel{}, sql.ErrNoRows
		}
		return biz.ProviderModel{}, err
	}
	return entToBizPM(row), nil
}

func (r *llmProviderModelRepo) GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error) {
	row, err := r.data.entClient.LlmProviderModel.Query().
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
	return entToBizPM(row), nil
}

func (r *llmProviderModelRepo) ValidateProviderPair(ctx context.Context, provider, model string) (bool, error) {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return false, nil
	}
	n, err := r.data.entClient.LlmProviderModel.Query().
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
	return entToBizPM(saved), nil
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
	row, err := r.data.entClient.ModelPricingRule.Query().
		Where(
			modelpricingrule.ProviderCodeEQ(rule.ProviderCode),
			modelpricingrule.ModelAPIIDEQ(rule.ModelAPIID),
			modelpricingrule.IsActiveEQ(true),
			modelpricingrule.EffectiveToEQ(""),
		).
		Only(ctx)
	if err == nil {
		return r.data.entClient.ModelPricingRule.UpdateOneID(row.ID).
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
	}
	if !ent.IsNotFound(err) {
		return err
	}
	rid := rule.ID
	if rid == "" {
		rid = fmt.Sprintf("pricing:%s:%s:%d", rule.ProviderCode, strings.ReplaceAll(rule.ModelAPIID, "/", "_"), time.Now().UTC().UnixNano())
	}
	rule.IsActive = true
	_, err = r.data.entClient.ModelPricingRule.Create().
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
	return err
}
