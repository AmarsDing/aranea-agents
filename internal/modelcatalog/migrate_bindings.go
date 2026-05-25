package modelcatalog

import (
	"context"
	"fmt"
)

// RunProviderMigrations applies all built-in legacy→catalog provider renames (idempotent).
func RunProviderMigrations(ctx context.Context, backend ApplyBackend) (ApplyMigrationStats, []string) {
	if backend == nil {
		return ApplyMigrationStats{}, nil
	}
	var stats ApplyMigrationStats
	var errs []string
	for _, rule := range ListProviderMigrationRules() {
		row, err := backend.MigrateProviderBindings(ctx, rule.Legacy, rule.Catalog)
		if err != nil {
			errs = append(errs, fmt.Sprintf("migrate %s→%s: %v", rule.Legacy, rule.Catalog, err))
			continue
		}
		stats.Agents += row.Agents
		stats.Sessions += row.Sessions
		stats.Eval += row.Eval
		stats.RuntimeSettings += row.RuntimeSettings
		stats.Skills += row.Skills
		stats.KnowledgeEmbed += row.KnowledgeEmbed
		stats.WebResearch += row.WebResearch
	}
	return stats, errs
}
