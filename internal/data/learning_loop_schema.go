package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/learning_loop.sql
var learningLoopDDL string

func EnsureLearningLoopSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, learningLoopDDL, "learning_loop")
}
