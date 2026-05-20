package plugintrpc

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// StatsRecorder persists plugin callback counters (async, best-effort).
type StatsRecorder interface {
	Record(ctx context.Context, pluginKey, point, status string)
}

// RepoStatsRecorder writes stats via PluginRepo.IncrementStats and optional run audit rows.
type RepoStatsRecorder struct {
	repo biz.PluginRepo
	runs biz.PluginRunRepo
}

// NewRepoStatsRecorder creates a StatsRecorder backed by the plugin repository.
func NewRepoStatsRecorder(repo biz.PluginRepo, runs biz.PluginRunRepo) *RepoStatsRecorder {
	if repo == nil {
		return nil
	}
	return &RepoStatsRecorder{repo: repo, runs: runs}
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
		bg := context.Background()
		if err := r.repo.IncrementStats(bg, pluginKey, delta); err != nil {
			_ = err
		}
		if r.runs != nil {
			detail, _ := json.Marshal(map[string]string{"point": point, "status": st})
			_ = r.runs.Insert(bg, biz.PluginRun{
				ID:            uuid.NewString(),
				PluginKey:     pluginKey,
				CallbackPoint: point,
				Status:        st,
				DetailJSON:    string(detail),
				CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			})
		}
	})
}
