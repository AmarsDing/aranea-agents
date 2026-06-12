package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/apierror"
)

type modelRegistryApplyBackend struct {
	data *Data
	llm  biz.LlmProviderModelReaderWriter
}

var _ modelregistry.ApplyBackend = (*modelRegistryApplyBackend)(nil)

func NewModelRegistryApplyBackend(d *Data, llm biz.LlmProviderModelReaderWriter) modelregistry.ApplyBackend {
	return &modelRegistryApplyBackend{data: d, llm: llm}
}

func (b *modelRegistryApplyBackend) ListProviderModels(ctx context.Context) ([]modelregistry.ApplyRow, error) {
	rows, err := b.llm.ListProviderModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]modelregistry.ApplyRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, modelregistry.ApplyRow{
			ID:           row.ID,
			Key:          row.Key,
			Name:         row.Name,
			Provider:     row.Provider,
			Model:        row.Model,
			Enabled:      row.Enabled,
			ConfigJSON:   row.ConfigJSON,
			MetadataJSON: row.MetadataJSON,
		})
	}
	return out, nil
}

func (b *modelRegistryApplyBackend) SaveProviderModel(ctx context.Context, row modelregistry.ApplyRow) error {
	_, err := b.llm.UpdateProviderModel(ctx, biz.ProviderModel{
		ID:           row.ID,
		Key:          row.Key,
		Name:         row.Name,
		Provider:     row.Provider,
		Model:        row.Model,
		Enabled:      row.Enabled,
		ConfigJSON:   row.ConfigJSON,
		MetadataJSON: row.MetadataJSON,
	})
	return err
}

func (b *modelRegistryApplyBackend) countEvalBindings(ctx context.Context, provider string) (int, error) {
	setting, err := b.data.RW().Read(ctx).SystemSetting.Query().Only(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	if setting.EvalSimProvider == provider {
		n++
	}
	if setting.EvalJudgeProvider == provider {
		n++
	}
	return n, nil
}

func (b *modelRegistryApplyBackend) countRuntimeProviderBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.RW().Read(ctx), ctx,
		`SELECT COUNT(*) FROM agent_runtime_settings
		 WHERE l0_compress_provider = ? OR memory_worker_provider = ?`,
		[]any{provider, provider},
		&n,
	)
	return n, err
}

func (b *modelRegistryApplyBackend) countSkillProviderBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.RW().Read(ctx), ctx,
		`SELECT COUNT(*) FROM skill WHERE deleted_at = '' AND provider = ?`,
		[]any{provider},
		&n,
	)
	return n, err
}

func (b *modelRegistryApplyBackend) countWebResearchBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.RW().Read(ctx), ctx,
		`SELECT COUNT(*) FROM system_settings WHERE web_research_provider = ?`,
		[]any{provider},
		&n,
	)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
		return 0, nil
	}
	return n, err
}

func (b *modelRegistryApplyBackend) countKnowledgeEmbedBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.RW().Read(ctx), ctx,
		`SELECT COUNT(*) FROM system_settings WHERE knowledge_embed_provider = ?`,
		[]any{provider},
		&n,
	)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
		return 0, nil
	}
	return n, err
}

func (b *modelRegistryApplyBackend) CountProviderBindings(ctx context.Context, provider string) (modelregistry.ApplyMigrationStats, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return modelregistry.ApplyMigrationStats{}, nil
	}
	var stats modelregistry.ApplyMigrationStats
	agents, err := b.data.RW().Read(ctx).Agent.Query().Where(agent.ProviderEQ(provider)).Count(ctx)
	if err != nil {
		return stats, err
	}
	stats.Agents = agents
	sess, err := b.data.RW().Read(ctx).Session.Query().Where(
		session.Or(
			session.DefaultProviderEQ(provider),
			session.LastProviderEQ(provider),
		),
	).Count(ctx)
	if err != nil {
		return stats, err
	}
	stats.Sessions = sess
	stats.Eval, err = b.countEvalBindings(ctx, provider)
	if err != nil {
		return stats, err
	}
	stats.RuntimeSettings, err = b.countRuntimeProviderBindings(ctx, provider)
	if err != nil {
		return stats, err
	}
	stats.Skills, err = b.countSkillProviderBindings(ctx, provider)
	if err != nil {
		return stats, err
	}
	stats.KnowledgeEmbed, err = b.countKnowledgeEmbedBindings(ctx, provider)
	if err != nil {
		return stats, err
	}
	stats.WebResearch, err = b.countWebResearchBindings(ctx, provider)
	return stats, err
}

func (b *modelRegistryApplyBackend) MigrateProviderBindings(ctx context.Context, from, to string) (modelregistry.ApplyMigrationStats, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" || from == to {
		return modelregistry.ApplyMigrationStats{}, nil
	}

	var stats modelregistry.ApplyMigrationStats
	err := b.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := b.data.RWDB().WriteDB(txCtx)
		var err error
		stats, err = migrateOneRuleInTx(txCtx, e, modelregistry.ProviderMigrationRule{Legacy: from, Catalog: to}, nowRFC3339())
		return err
	})
	return stats, err
}

func (b *modelRegistryApplyBackend) UpsertModelPricing(ctx context.Context, provider, model string, micro modelregistry.MicroPricing, source string) error {
	rule := microPricingToBizRule(provider, model, micro, source)
	return b.llm.UpsertModelPricingRule(ctx, rule)
}

func (b *modelRegistryApplyBackend) BatchMigrateProviderBindings(
	ctx context.Context,
	rules []modelregistry.ProviderMigrationRule,
	skipRules []string,
) modelregistry.BatchMigrationResult {
	var result modelregistry.BatchMigrationResult
	err := b.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := b.data.RWDB().WriteDB(txCtx)
		now := nowRFC3339()

		for _, rule := range rules {
			rk := ruleKey(rule)
			if containsStr(skipRules, rk) {
				result.CompletedRules = append(result.CompletedRules, rk)
				continue
			}
			stats, err := migrateOneRuleInTx(txCtx, e, rule, now)
			if err != nil {
				result.FailedRules = append(result.FailedRules, rk)
				result.Errors = append(result.Errors, fmt.Sprintf("migrate %s->%s: %v", rule.Legacy, rule.Catalog, err))
				continue
			}
			result.CompletedRules = append(result.CompletedRules, rk)
			result.Stats.Agents += stats.Agents
			result.Stats.Sessions += stats.Sessions
			result.Stats.Eval += stats.Eval
			result.Stats.RuntimeSettings += stats.RuntimeSettings
			result.Stats.Skills += stats.Skills
			result.Stats.KnowledgeEmbed += stats.KnowledgeEmbed
			result.Stats.WebResearch += stats.WebResearch
		}
		return nil
	})
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}

func (b *modelRegistryApplyBackend) BatchApply(
	ctx context.Context,
	patches []modelregistry.ApplyRow,
	pricing []modelregistry.PricingUpsert,
) modelregistry.BatchApplyResult {
	var result modelregistry.BatchApplyResult
	err := b.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := b.data.RWDB().WriteDB(txCtx)
		now := nowRFC3339()

		for _, p := range patches {
			res, err := e.ExecContext(txCtx,
				`UPDATE llm_provider_models SET provider=?, model_key=?, name=?, enabled=?, config_json=?, metadata_json=?, updated_at=? WHERE id=?`,
				p.Provider, p.Key, p.Name, p.Enabled, p.ConfigJSON, p.MetadataJSON, now, p.ID,
			)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("save %s/%s: %v", p.Provider, p.Model, err))
				continue
			}
			n, _ := res.RowsAffected()
			if n > 0 {
				result.RowsUpdated++
			}
		}

		for _, p := range pricing {
			if err := b.upsertPricingInTx(txCtx, e, p); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("pricing %s/%s: %v", p.Provider, p.Model, err))
				continue
			}
			result.PricingUpdated++
		}

		return nil
	})
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}

func (b *modelRegistryApplyBackend) upsertPricingInTx(ctx context.Context, e execer, p modelregistry.PricingUpsert) error {
	now := nowRFC3339()
	var existingID string
	err := queryRowScan(ctx, e,
		`SELECT id FROM model_pricing_rules WHERE provider_code = ? AND model_api_id = ? AND is_active = true AND effective_to = ''`,
		[]any{p.Provider, p.Model},
		&existingID,
	)
	if err != nil {
		if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeNotFound {
			return err
		}
	}
	if existingID != "" {
		var existingSource string
		if err := queryRowScan(ctx, e, `SELECT source FROM model_pricing_rules WHERE id = ?`, []any{existingID}, &existingSource); err != nil {
			return err
		}
		if biz.PricingSourcePriority(p.Source) < biz.PricingSourcePriority(existingSource) {
			return nil
		}
		cost := modelregistry.MicroPer1KToCostUSDPer1M(p.Micro)
		_, err := e.ExecContext(ctx,
			`UPDATE model_pricing_rules SET currency='USD', input_price_micro_usd_per_1k=?, output_price_micro_usd_per_1k=?, cached_input_price_micro_usd_per_1k=?, cache_write_price_micro_usd_per_1k=?, reasoning_price_micro_usd_per_1k=?, embedding_price_micro_usd_per_1k=?, input_price_usd_per_1m=?, output_price_usd_per_1m=?, cached_input_price_usd_per_1m=?, cache_write_price_usd_per_1m=?, reasoning_price_usd_per_1m=?, embedding_price_usd_per_1m=?, source=?, metadata_json='{}', updated_at=? WHERE id=?`,
			p.Micro.Input, p.Micro.Output, p.Micro.CacheRead, p.Micro.CacheWrite, p.Micro.Reasoning, p.Micro.Embedding,
			cost.Input, cost.Output, cost.CacheRead, cost.CacheWrite, cost.Reasoning, cost.Embedding,
			p.Source, now, existingID,
		)
		return err
	}
	ruleID := fmt.Sprintf("pricing:%s:%s:%d", p.Provider, strings.ReplaceAll(p.Model, "/", "_"), time.Now().UTC().UnixNano())
	cost := modelregistry.MicroPer1KToCostUSDPer1M(p.Micro)
	_, err = e.ExecContext(ctx,
		`INSERT INTO model_pricing_rules (id, provider_code, model_api_id, currency, input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, cache_write_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k, input_price_usd_per_1m, output_price_usd_per_1m, cached_input_price_usd_per_1m, cache_write_price_usd_per_1m, reasoning_price_usd_per_1m, embedding_price_usd_per_1m, effective_from, effective_to, is_active, source, metadata_json, created_at, updated_at) VALUES (?, ?, ?, 'USD', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', true, ?, '{}', ?, ?)`,
		ruleID, p.Provider, p.Model,
		p.Micro.Input, p.Micro.Output, p.Micro.CacheRead, p.Micro.CacheWrite, p.Micro.Reasoning, p.Micro.Embedding,
		cost.Input, cost.Output, cost.CacheRead, cost.CacheWrite, cost.Reasoning, cost.Embedding,
		now, p.Source, now, now,
	)
	return err
}

func microPricingToBizRule(provider, model string, micro modelregistry.MicroPricing, source string) biz.ModelPricingRule {
	cost := modelregistry.MicroPer1KToCostUSDPer1M(micro)
	return biz.ModelPricingRule{
		ProviderCode:                  provider,
		ModelAPIID:                    model,
		Currency:                      "USD",
		InputPriceMicroUSDPer1K:       micro.Input,
		OutputPriceMicroUSDPer1K:      micro.Output,
		CachedInputPriceMicroUSDPer1K: micro.CacheRead,
		CacheWritePriceMicroUSDPer1K:  micro.CacheWrite,
		ReasoningPriceMicroUSDPer1K:   micro.Reasoning,
		EmbeddingPriceMicroUSDPer1K:   micro.Embedding,
		InputPriceUSDPer1M:            cost.Input,
		OutputPriceUSDPer1M:           cost.Output,
		CachedInputPriceUSDPer1M:      cost.CacheRead,
		CacheWritePriceUSDPer1M:       cost.CacheWrite,
		ReasoningPriceUSDPer1M:        cost.Reasoning,
		EmbeddingPriceUSDPer1M:        cost.Embedding,
		Source:                        source,
		MetadataJSON:                  "{}",
	}
}

func migrateOneRuleInTx(ctx context.Context, e execer, rule modelregistry.ProviderMigrationRule, now string) (modelregistry.ApplyMigrationStats, error) {
	var stats modelregistry.ApplyMigrationStats
	from, to := rule.Legacy, rule.Catalog

	res, err := e.ExecContext(ctx, `UPDATE agents SET provider = ?, updated_at = ? WHERE provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ := res.RowsAffected()
	stats.Agents = int(n)

	res, err = e.ExecContext(ctx, `UPDATE sessions SET default_provider = ? WHERE default_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Sessions = int(n)
	res, err = e.ExecContext(ctx, `UPDATE sessions SET last_provider = ? WHERE last_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Sessions += int(n)

	res, err = e.ExecContext(ctx, `UPDATE system_settings SET eval_sim_provider = ? WHERE eval_sim_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Eval = int(n)
	res, err = e.ExecContext(ctx, `UPDATE system_settings SET eval_judge_provider = ? WHERE eval_judge_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Eval += int(n)

	res, err = e.ExecContext(ctx,
		`UPDATE agent_runtime_settings SET l0_compress_provider = ? WHERE l0_compress_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.RuntimeSettings = int(n)
	res, err = e.ExecContext(ctx,
		`UPDATE agent_runtime_settings SET memory_worker_provider = ? WHERE memory_worker_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.RuntimeSettings += int(n)

	res, err = e.ExecContext(ctx,
		`UPDATE skill SET provider = ?, updated_at = ? WHERE deleted_at = '' AND provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Skills = int(n)

	res, err = e.ExecContext(ctx,
		`UPDATE system_settings SET knowledge_embed_provider = ?, update_time = ? WHERE knowledge_embed_provider = ?`,
		to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.KnowledgeEmbed = int(n)

	res, err = e.ExecContext(ctx,
		`UPDATE system_settings SET web_research_provider = ?, update_time = ? WHERE web_research_provider = ?`,
		to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.WebResearch = int(n)

	res, err = e.ExecContext(ctx,
		`UPDATE llm_provider_models SET provider = ?, model_key = ? || ':' || model, updated_at = ? WHERE provider = ?`,
		to, to, now, from)
	if err != nil {
		return stats, err
	}
	_, _ = res.RowsAffected()

	return stats, nil
}

func ruleKey(rule modelregistry.ProviderMigrationRule) string {
	return rule.Legacy + "->" + rule.Catalog
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
