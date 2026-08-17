package knowledge

import (
	"time"

	"aranea-agents/internal/metrics"
)

const (
	searchStageEmbed  = "embed"
	searchStageDense  = "dense"
	searchStageSparse = "sparse"
	searchStageTotal  = "total"
)

func observeSearchStage(stage string, start time.Time) {
	metrics.KnowledgeSearchStageDuration.WithLabelValues(stage).Observe(time.Since(start).Seconds())
}
