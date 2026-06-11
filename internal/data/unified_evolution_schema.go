package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/unified_evolution.sql
var unifiedEvolutionDDL string

func EnsureUnifiedEvolutionSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, unifiedEvolutionDDL, "unified_evolution")
}
