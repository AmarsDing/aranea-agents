package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/plan.sql
var planDDL string

func EnsurePlanSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, planDDL, "plan")
}
