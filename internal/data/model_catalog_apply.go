package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/modelcatalog"
)

type modelCatalogApplyBackend struct {
	data *Data
	llm  biz.LlmProviderModelRepo
}

// NewModelCatalogApplyBackend implements modelcatalog.ApplyBackend.
func NewModelCatalogApplyBackend(d *Data, llm biz.LlmProviderModelRepo) modelcatalog.ApplyBackend {
	return &modelCatalogApplyBackend{data: d, llm: llm}
}

func (b *modelCatalogApplyBackend) ListProviderModels(ctx context.Context) ([]modelcatalog.ApplyRow, error) {
	rows, err := b.llm.ListProviderModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]modelcatalog.ApplyRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, modelcatalog.ApplyRow{
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

func (b *modelCatalogApplyBackend) SaveProviderModel(ctx context.Context, row modelcatalog.ApplyRow) error {
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

func (b *modelCatalogApplyBackend) countEvalBindings(ctx context.Context, provider string) (int, error) {
	setting, err := b.data.entClient.SystemSetting.Query().Only(ctx)
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

func (b *modelCatalogApplyBackend) migrateEvalBindings(ctx context.Context, from, to string) (int, error) {
	setting, err := b.data.entClient.SystemSetting.Query().Only(ctx)
	if err != nil {
		return 0, err
	}
	upd := b.data.entClient.SystemSetting.UpdateOneID(setting.ID)
	changed := 0
	if setting.EvalSimProvider == from {
		upd.SetEvalSimProvider(to)
		changed++
	}
	if setting.EvalJudgeProvider == from {
		upd.SetEvalJudgeProvider(to)
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := upd.Exec(ctx); err != nil {
		return 0, err
	}
	return changed, nil
}

func (b *modelCatalogApplyBackend) countRuntimeProviderBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.Ent(), ctx,
		`SELECT COUNT(*) FROM agent_runtime_settings
		 WHERE l0_compress_provider = ? OR memory_worker_provider = ?`,
		[]any{provider, provider},
		&n,
	)
	return n, err
}

func (b *modelCatalogApplyBackend) countSkillProviderBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.Ent(), ctx,
		`SELECT COUNT(*) FROM skill WHERE deleted_at = '' AND provider = ?`,
		[]any{provider},
		&n,
	)
	return n, err
}

func (b *modelCatalogApplyBackend) countWebResearchBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.Ent(), ctx,
		`SELECT COUNT(*) FROM system_settings WHERE web_research_provider = ?`,
		[]any{provider},
		&n,
	)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

func (b *modelCatalogApplyBackend) migrateWebResearchProvider(ctx context.Context, from, to string) (int, error) {
	res, err := b.data.Ent().ExecContext(ctx,
		`UPDATE system_settings SET web_research_provider = ?, update_time = ? WHERE web_research_provider = ?`,
		to, nowRFC3339(), from,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (b *modelCatalogApplyBackend) countKnowledgeEmbedBindings(ctx context.Context, provider string) (int, error) {
	var n int
	err := entQueryRowScan(b.data.Ent(), ctx,
		`SELECT COUNT(*) FROM system_settings WHERE knowledge_embed_provider = ?`,
		[]any{provider},
		&n,
	)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

func (b *modelCatalogApplyBackend) migrateRuntimeProviders(ctx context.Context, from, to string) (int, error) {
	res, err := b.data.Ent().ExecContext(ctx,
		`UPDATE agent_runtime_settings SET l0_compress_provider = ? WHERE l0_compress_provider = ?`,
		to, from,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	res, err = b.data.Ent().ExecContext(ctx,
		`UPDATE agent_runtime_settings SET memory_worker_provider = ? WHERE memory_worker_provider = ?`,
		to, from,
	)
	if err != nil {
		return int(n), err
	}
	n2, _ := res.RowsAffected()
	return int(n + n2), nil
}

func (b *modelCatalogApplyBackend) migrateSkillProviders(ctx context.Context, from, to string) (int, error) {
	res, err := b.data.Ent().ExecContext(ctx,
		`UPDATE skill SET provider = ?, updated_at = ? WHERE deleted_at = '' AND provider = ?`,
		to, nowRFC3339(), from,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (b *modelCatalogApplyBackend) migrateKnowledgeEmbedProvider(ctx context.Context, from, to string) (int, error) {
	res, err := b.data.Ent().ExecContext(ctx,
		`UPDATE system_settings SET knowledge_embed_provider = ?, update_time = ? WHERE knowledge_embed_provider = ?`,
		to, nowRFC3339(), from,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (b *modelCatalogApplyBackend) CountProviderBindings(ctx context.Context, provider string) (modelcatalog.ApplyMigrationStats, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return modelcatalog.ApplyMigrationStats{}, nil
	}
	var stats modelcatalog.ApplyMigrationStats
	agents, err := b.data.entClient.Agent.Query().Where(agent.ProviderEQ(provider)).Count(ctx)
	if err != nil {
		return stats, err
	}
	stats.Agents = agents
	sess, err := b.data.entClient.Session.Query().Where(
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

func (b *modelCatalogApplyBackend) MigrateProviderBindings(ctx context.Context, from, to string) (modelcatalog.ApplyMigrationStats, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" || from == to {
		return modelcatalog.ApplyMigrationStats{}, nil
	}

	tx, err := b.data.RawDB().BeginTx(ctx, nil)
	if err != nil {
		return modelcatalog.ApplyMigrationStats{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var stats modelcatalog.ApplyMigrationStats
	now := nowRFC3339()

	res, err := tx.ExecContext(ctx, `UPDATE agents SET provider = ?, updated_at = ? WHERE provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ := res.RowsAffected()
	stats.Agents = int(n)

	res, err = tx.ExecContext(ctx, `UPDATE sessions SET default_provider = ? WHERE default_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Sessions = int(n)
	res, err = tx.ExecContext(ctx, `UPDATE sessions SET last_provider = ? WHERE last_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Sessions += int(n)

	res, err = tx.ExecContext(ctx, `UPDATE system_settings SET eval_sim_provider = ? WHERE eval_sim_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Eval += int(n)
	res, err = tx.ExecContext(ctx, `UPDATE system_settings SET eval_judge_provider = ? WHERE eval_judge_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Eval += int(n)

	res, err = tx.ExecContext(ctx,
		`UPDATE agent_runtime_settings SET l0_compress_provider = ? WHERE l0_compress_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.RuntimeSettings = int(n)
	res, err = tx.ExecContext(ctx,
		`UPDATE agent_runtime_settings SET memory_worker_provider = ? WHERE memory_worker_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.RuntimeSettings += int(n)

	res, err = tx.ExecContext(ctx,
		`UPDATE skill SET provider = ?, updated_at = ? WHERE deleted_at = '' AND provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Skills = int(n)

	res, err = tx.ExecContext(ctx,
		`UPDATE system_settings SET knowledge_embed_provider = ?, update_time = ? WHERE knowledge_embed_provider = ?`,
		to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.KnowledgeEmbed = int(n)

	res, err = tx.ExecContext(ctx,
		`UPDATE system_settings SET web_research_provider = ?, update_time = ? WHERE web_research_provider = ?`,
		to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.WebResearch = int(n)

	res, err = tx.ExecContext(ctx,
		`UPDATE llm_provider_models SET provider = ?, model_key = ? || ':' || model, updated_at = ? WHERE provider = ?`,
		to, to, now, from)
	if err != nil {
		return stats, err
	}
	_, _ = res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

func (b *modelCatalogApplyBackend) UpsertModelPricing(ctx context.Context, provider, model string, micro modelcatalog.MicroPricing, source string) error {
	cost := modelcatalog.CostUSDPer1M{
		Input:      modelcatalog.MicroPer1KToUSDPer1M(micro.Input),
		Output:     modelcatalog.MicroPer1KToUSDPer1M(micro.Output),
		CacheRead:  modelcatalog.MicroPer1KToUSDPer1M(micro.CacheRead),
		CacheWrite: modelcatalog.MicroPer1KToUSDPer1M(micro.CacheWrite),
		Reasoning:  modelcatalog.MicroPer1KToUSDPer1M(micro.Reasoning),
		Embedding:  modelcatalog.MicroPer1KToUSDPer1M(micro.Embedding),
	}
	return b.llm.UpsertModelPricingRule(ctx, biz.ModelPricingRule{
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
	})
}
