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

// CallbackEvent is one plugin callback telemetry sample.
type CallbackEvent struct {
	PluginKey  string
	Point      string
	Status     string
	Action     string
	AgentID    string
	SessionID  string
	DurationMS int
	Summary    string
}

// StatsRecorder persists plugin callback counters and optional run audit rows (async).
type StatsRecorder interface {
	Record(ctx context.Context, pluginKey, point, status string)
	RecordEvent(ctx context.Context, ev CallbackEvent)
}

// noopStatsRecorder discards stats (tests / nil runtime).
type noopStatsRecorder struct{}

func (noopStatsRecorder) Record(context.Context, string, string, string) {}
func (noopStatsRecorder) RecordEvent(context.Context, CallbackEvent)     {}

// RepoStatsRecorder writes stats via PluginRepo.IncrementStats and plugin_runs audit.
type RepoStatsRecorder struct {
	repo         biz.PluginRepo
	runs         biz.PluginRunRepo
	resolveAgent AgentKeyResolver
}

// NewRepoStatsRecorder creates a StatsRecorder backed by the plugin repository.
func NewRepoStatsRecorder(repo biz.PluginRepo, runs biz.PluginRunRepo) *RepoStatsRecorder {
	if repo == nil {
		return nil
	}
	return &RepoStatsRecorder{repo: repo, runs: runs}
}

// SetAgentKeyResolver enables platform agent_id lookup when persisting runs.
func (r *RepoStatsRecorder) SetAgentKeyResolver(fn AgentKeyResolver) {
	if r != nil {
		r.resolveAgent = fn
	}
}

func (r *RepoStatsRecorder) RecordEvent(ctx context.Context, ev CallbackEvent) {
	r.recordEvent(ctx, ev)
}

func (r *RepoStatsRecorder) Record(ctx context.Context, pluginKey, point, status string) {
	r.RecordEvent(ctx, CallbackEvent{
		PluginKey: strings.TrimSpace(pluginKey),
		Point:     strings.TrimSpace(point),
		Status:    strings.TrimSpace(status),
		Action:    callbackAction(status),
	})
}

func callbackAction(status string) string {
	switch normalizeRunStatus(status) {
	case "blocked":
		return "block"
	case "error":
		return "error"
	default:
		return "pass"
	}
}

func normalizeRunStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "ok":
		return "success"
	case "queued":
		return "queued"
	default:
		return strings.TrimSpace(status)
	}
}

func shouldPersistPluginRun(status string) bool {
	switch normalizeRunStatus(status) {
	case "blocked", "error":
		return true
	default:
		return false
	}
}

func (r *RepoStatsRecorder) recordEvent(ctx context.Context, ev CallbackEvent) {
	if r == nil || r.repo == nil {
		return
	}
	pluginKey := strings.TrimSpace(ev.PluginKey)
	if pluginKey == "" {
		return
	}
	isHookRun := strings.HasPrefix(pluginKey, "hook:")
	st := normalizeRunStatus(ev.Status)
	if st == "" {
		st = "success"
	}
	point := strings.TrimSpace(ev.Point)
	// Hook audit rows are metered in executeHookAction defer; skip duplicate Prometheus samples.
	if !isHookRun {
		arametrics.PluginInvokeTotal.WithLabelValues(pluginKey, point, st).Inc()
	}

	delta := biz.PluginStatUpdate{InvokeCount: 1, LastStatus: st}
	switch st {
	case "blocked":
		delta.BlockDelta = 1
	case "error":
		delta.ErrorDelta = 1
	}

	sessionID, agentKey := invocationMeta(ctx)
	agentID := strings.TrimSpace(ev.AgentID)
	if agentID == "" && r.resolveAgent != nil && agentKey != "" {
		agentID = strings.TrimSpace(r.resolveAgent(ctx, agentKey))
	}
	action := strings.TrimSpace(ev.Action)
	if action == "" {
		action = callbackAction(st)
	}

	persistRun := shouldPersistPluginRun(st)

	safego.Go(ctx, "plugin.stats."+pluginKey, func() {
		bg := context.Background()
		if !isHookRun {
			if err := r.repo.IncrementStats(bg, pluginKey, delta); err != nil {
				_ = err
			}
		}
		if !persistRun || r.runs == nil {
			return
		}
		pluginID := ""
		if p, err := r.repo.GetByKey(bg, pluginKey); err == nil {
			pluginID = strings.TrimSpace(p.ID)
		}
		detail, _ := json.Marshal(map[string]string{
			"point":   point,
			"status":  st,
			"action":  action,
			"summary": strings.TrimSpace(ev.Summary),
		})
		_ = r.runs.Insert(bg, biz.PluginRun{
			ID:            uuid.NewString(),
			PluginKey:     pluginKey,
			PluginID:      pluginID,
			AgentID:       agentID,
			SessionID:     sessionID,
			CallbackPoint: point,
			Status:        st,
			DurationMS:    ev.DurationMS,
			DetailJSON:    string(detail),
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		})
	})
}
