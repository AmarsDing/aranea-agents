package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/skill_evolution.sql
var skillEvolutionDDL string

func EnsureSkillEvolutionSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, skillEvolutionDDL, "skill_evolution")
}
