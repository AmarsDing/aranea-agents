package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"
)

// StatsRecorder persists plugin callback counters (async, best-effort).
type StatsRecorder interface {
	Record(ctx context.Context, pluginKey, point, status string)
}

// RepoStatsRecorder writes stats via PluginRepo.IncrementStats.
type RepoStatsRecorder struct {
	repo biz.PluginRepo
}

// NewRepoStatsRecorder creates a StatsRecorder backed by the plugin repository.
func NewRepoStatsRecorder(repo biz.PluginRepo) *RepoStatsRecorder {
	if repo == nil {
		return nil
	}
	return &RepoStatsRecorder{repo: repo}
}

func (r *RepoStatsRecorder) Record(ctx context.Context, pluginKey, point, status string) {
	if r == nil || r.repo == nil {
		return
	}
	pluginKey = strings.TrimSpace(pluginKey)
	if pluginKey == "" {
		return
	}
	st := strings.TrimSpace(status)
	if st == "" {
		st = "ok"
	}
	arametrics.PluginInvokeTotal.WithLabelValues(pluginKey, point, st).Inc()

	delta := biz.PluginStatUpdate{InvokeCount: 1, LastStatus: st}
	switch st {
	case "blocked":
		delta.BlockDelta = 1
	case "error":
		delta.ErrorDelta = 1
	}

	safego.Go(ctx, "plugin.stats."+pluginKey, func() {
		if err := r.repo.IncrementStats(context.Background(), pluginKey, delta); err != nil {
			// best-effort; metrics already counted
			_ = err
		}
	})
}
