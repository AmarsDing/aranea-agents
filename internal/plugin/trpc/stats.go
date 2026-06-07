package plugintrpc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

const (
	statsChanSize   = 512
	statsFlushMs    = 500
	statsFlushBatch = 64
)

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

type StatsRecorder interface {
	Record(ctx context.Context, pluginKey, point, status string)
	RecordEvent(ctx context.Context, ev CallbackEvent)
	Close()
}

type noopStatsRecorder struct{}

func (noopStatsRecorder) Record(context.Context, string, string, string) {}
func (noopStatsRecorder) RecordEvent(context.Context, CallbackEvent)     {}
func (noopStatsRecorder) Close()                                         {}

// TECH-DEBT(BR1): RepoStatsRecorder 在 worker goroutine 中直接调用 repo 写库，
// 未经过 EventBus 统一管道。当前已通过 channel+worker 批量异步化，
// 但应迁移到 EventBus + consumer 模式以保持架构一致性。
type RepoStatsRecorder struct {
	repo         biz.PluginRepo
	runs         biz.PluginRunRepo
	resolveAgent AgentKeyResolver
	resolveMu    sync.RWMutex
	lg           loggateway.Logger

	ch   chan CallbackEvent
	done chan struct{}
	wg   sync.WaitGroup
	closeOnce sync.Once
}

func NewRepoStatsRecorder(repo biz.PluginRepo, runs biz.PluginRunRepo, lg loggateway.Logger) *RepoStatsRecorder {
	if repo == nil {
		return nil
	}
	r := &RepoStatsRecorder{
		repo: repo,
		runs: runs,
		lg:   lg,
		ch:   make(chan CallbackEvent, statsChanSize),
		done: make(chan struct{}),
	}
	r.wg.Add(1)
	safego.Go(appctx.Ctx(), "stats.worker", r.worker)
	return r
}

func (r *RepoStatsRecorder) SetAgentKeyResolver(fn AgentKeyResolver) {
	if r != nil {
		r.resolveMu.Lock()
		r.resolveAgent = fn
		r.resolveMu.Unlock()
	}
}

func (r *RepoStatsRecorder) RecordEvent(_ context.Context, ev CallbackEvent) {
	if r == nil {
		return
	}
	select {
	case r.ch <- ev:
	default:
		arametrics.PluginInvokeTotal.WithLabelValues("stats_recorder", "record", "dropped").Inc()
	}
}

func (r *RepoStatsRecorder) Record(ctx context.Context, pluginKey, point, status string) {
	r.RecordEvent(ctx, CallbackEvent{
		PluginKey: strings.TrimSpace(pluginKey),
		Point:     strings.TrimSpace(point),
		Status:    strings.TrimSpace(status),
		Action:    callbackAction(status),
	})
}

func (r *RepoStatsRecorder) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.done)
	})
	r.wg.Wait()
}

func (r *RepoStatsRecorder) worker() {
	defer r.wg.Done()
	ticker := time.NewTicker(time.Duration(statsFlushMs) * time.Millisecond)
	defer ticker.Stop()
	buf := make([]CallbackEvent, 0, statsFlushBatch)
	for {
		select {
		case ev, ok := <-r.ch:
			if !ok {
				r.flush(buf)
				return
			}
			buf = append(buf, ev)
			if len(buf) >= statsFlushBatch {
				r.flush(buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			if len(buf) > 0 {
				r.flush(buf)
				buf = buf[:0]
			}
		case <-r.done:
			r.drain(buf)
			return
		}
	}
}

func (r *RepoStatsRecorder) drain(buf []CallbackEvent) {
	for {
		select {
		case ev, ok := <-r.ch:
			if !ok {
				r.flush(buf)
				return
			}
			buf = append(buf, ev)
		default:
			r.flush(buf)
			return
		}
	}
}

func (r *RepoStatsRecorder) flush(batch []CallbackEvent) {
	if len(batch) == 0 {
		return
	}
	aggr := r.aggregate(batch)
	bg := context.Background()
	failedKeys := make(map[string]bool, len(aggr))
	for key, delta := range aggr {
		if err := r.repo.IncrementStats(bg, key, delta); err != nil {
			failedKeys[key] = true
		}
	}
	for _, ev := range batch {
		if failedKeys[strings.TrimSpace(ev.PluginKey)] {
			continue
		}
		r.persistRun(bg, ev)
	}
}

func (r *RepoStatsRecorder) aggregate(batch []CallbackEvent) map[string]biz.PluginStatUpdate {
	m := make(map[string]biz.PluginStatUpdate, 16)
	for _, ev := range batch {
		pluginKey := strings.TrimSpace(ev.PluginKey)
		if pluginKey == "" {
			continue
		}
		if strings.HasPrefix(pluginKey, "hook:") {
			continue
		}
		st := normalizeRunStatus(ev.Status)
		if st == "" {
			st = "success"
		}
		arametrics.PluginInvokeTotal.WithLabelValues(pluginKey, strings.TrimSpace(ev.Point), st).Inc()
		d, ok := m[pluginKey]
		if !ok {
			d = biz.PluginStatUpdate{LastStatus: st}
		}
		d.InvokeCount++
		switch st {
		case "blocked":
			d.BlockDelta++
		case "error":
			d.ErrorDelta++
		}
		d.LastStatus = st
		m[pluginKey] = d
	}
	return m
}

func (r *RepoStatsRecorder) persistRun(bg context.Context, ev CallbackEvent) {
	st := normalizeRunStatus(ev.Status)
	if st == "" {
		st = "success"
	}
	if !shouldPersistPluginRun(st) {
		return
	}
	if r.runs == nil {
		return
	}
	pluginKey := strings.TrimSpace(ev.PluginKey)
	pluginID := ""
	if p, err := r.repo.GetByKey(bg, pluginKey); err == nil {
		pluginID = strings.TrimSpace(p.ID)
	}
	action := strings.TrimSpace(ev.Action)
	if action == "" {
		action = callbackAction(st)
	}
	agentID := strings.TrimSpace(ev.AgentID)
	if agentID == "" {
		r.resolveMu.RLock()
		fn := r.resolveAgent
		r.resolveMu.RUnlock()
		if fn != nil {
			agentID = strings.TrimSpace(fn(bg, ""))
		}
	}
	detail, _ := json.Marshal(map[string]string{
		"point":   strings.TrimSpace(ev.Point),
		"status":  st,
		"action":  action,
		"summary": strings.TrimSpace(ev.Summary),
	})
	if err := r.runs.Insert(bg, biz.PluginRun{
		ID:            uuid.NewString(),
		PluginKey:     pluginKey,
		PluginID:      pluginID,
		AgentID:       agentID,
		SessionID:     strings.TrimSpace(ev.SessionID),
		CallbackPoint: strings.TrimSpace(ev.Point),
		Status:        st,
		DurationMS:    ev.DurationMS,
		DetailJSON:    string(detail),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		r.lg.Warn("PluginRun persist failed",
			loggateway.StepID("plugin.stats.persist_fail"),
			loggateway.Str("plugin", pluginKey),
			loggateway.Str("point", ev.Point),
			loggateway.Err(err))
	}
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
