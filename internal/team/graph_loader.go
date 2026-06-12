package team

import (
	"context"

	"aranea-agents/internal/biz"
)

// GraphBuildConfigLoader resolves a persisted graph asset (linked_graph_id) to a build config.
// Stability:stable
type GraphBuildConfigLoader interface {
	LoadGraphBuildConfig(ctx context.Context, graphID string) (biz.GraphBuildConfig, error)
}
