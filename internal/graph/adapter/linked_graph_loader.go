package adapter

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// LinkedGraphBuildConfigLoader loads graph assets referenced by team linked_graph_id.
type LinkedGraphBuildConfigLoader struct {
	graphs *biz.GraphUsecase
}

func NewLinkedGraphBuildConfigLoader(graphs *biz.GraphUsecase) *LinkedGraphBuildConfigLoader {
	return &LinkedGraphBuildConfigLoader{graphs: graphs}
}

func (l *LinkedGraphBuildConfigLoader) LoadGraphBuildConfig(ctx context.Context, graphID string) (biz.GraphBuildConfig, error) {
	if l == nil || l.graphs == nil {
		return biz.GraphBuildConfig{}, biz.ErrNotFound
	}
	graphID = strings.TrimSpace(graphID)
	if graphID == "" {
		return biz.GraphBuildConfig{}, biz.ErrNotFound
	}
	def, err := l.graphs.GetGraph(ctx, graphID)
	if err != nil {
		return biz.GraphBuildConfig{}, err
	}
	return biz.BuildConfigFromGraphDefinition(def), nil
}
