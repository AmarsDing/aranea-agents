package modelregistry

import "context"

type MigrationPreviewItem struct {
	LegacyProvider  string
	CatalogProvider string
	LLMRows         int
	Agents          int
	Sessions        int
	Eval            int
	RuntimeSettings int
	Skills          int
	KnowledgeEmbed  int
	WebResearch     int
}

type MigrationPreview struct {
	Items []MigrationPreviewItem
}

func PreviewMigration(ctx context.Context, backend ApplyBackend) (MigrationPreview, error) {
	if backend == nil {
		return MigrationPreview{}, nil
	}
	out := MigrationPreview{Items: make([]MigrationPreviewItem, 0, len(ProviderMigration))}
	rows, err := backend.ListProviderModels(ctx)
	if err != nil {
		return MigrationPreview{}, err
	}
	llmCounts := map[string]int{}
	for _, row := range rows {
		llmCounts[row.Provider]++
	}
	for _, rule := range ListProviderMigrationRules() {
		stats, err := backend.CountProviderBindings(ctx, rule.Legacy)
		if err != nil {
			return MigrationPreview{}, err
		}
		item := MigrationPreviewItem{
			LegacyProvider:  rule.Legacy,
			CatalogProvider: rule.Catalog,
			LLMRows:         llmCounts[rule.Legacy],
			Agents:          stats.Agents,
			Sessions:        stats.Sessions,
			Eval:            stats.Eval,
			RuntimeSettings: stats.RuntimeSettings,
			Skills:          stats.Skills,
			KnowledgeEmbed:  stats.KnowledgeEmbed,
			WebResearch:     stats.WebResearch,
		}
		if item.LLMRows > 0 || item.Agents > 0 || item.Sessions > 0 || item.Eval > 0 ||
			item.RuntimeSettings > 0 || item.Skills > 0 || item.KnowledgeEmbed > 0 || item.WebResearch > 0 {
			out.Items = append(out.Items, item)
		}
	}
	return out, nil
}
