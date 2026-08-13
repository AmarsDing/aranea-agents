package monitor_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/usage"
)

var errStubCacheHitStats = errors.New("stub cache hit stats error")

type stubCacheHitStatsRepo struct {
	stats []usage.CacheHitRatioStat
	err   error
}

func (s *stubCacheHitStatsRepo) CacheHitRatioStats(context.Context, time.Duration) ([]usage.CacheHitRatioStat, error) {
	return s.stats, s.err
}

func cacheHitGroup(provider, model string, samples int, prompt, cached int64) usage.CacheHitRatioStat {
	ratio := 0.0
	if prompt > 0 {
		ratio = float64(cached) / float64(prompt)
	}
	return usage.CacheHitRatioStat{
		Provider: provider, Model: model,
		Samples: samples, PromptTok: prompt, CachedTok: cached,
		WeightedRatio: ratio, P50Ratio: ratio,
	}
}

// cacheHitGroupP50 builds a group whose median per-turn ratio diverges from
// its token-weighted ratio (e.g. a compaction turn busted the cache on a very
// large rewritten prompt, sinking the weighted ratio while typical turns stay
// healthy).
func cacheHitGroupP50(provider, model string, samples int, prompt, cached int64, p50 float64) usage.CacheHitRatioStat {
	s := cacheHitGroup(provider, model, samples, prompt, cached)
	s.P50Ratio = p50
	return s
}

func TestCacheHitRatioLowMetric_KeyAndCatalog(t *testing.T) {
	m := monitor.NewCacheHitRatioLowMetric(nil)
	if m.Key() != "llm.cache_hit_ratio_low" {
		t.Errorf("Key() = %q, want %q", m.Key(), "llm.cache_hit_ratio_low")
	}
	if m.Description() == "" {
		t.Error("Description() is empty, want non-empty")
	}
	c := m.Catalog()
	if c.Key != "llm.cache_hit_ratio_low" || c.Unit != "count" {
		t.Errorf("Catalog() = %+v, want key llm.cache_hit_ratio_low unit count", c)
	}
	if c.DefaultWindowMinutes != 60 || c.SuggestedThreshold != 1 {
		t.Errorf("Catalog() window/threshold = %d/%v, want 60/1", c.DefaultWindowMinutes, c.SuggestedThreshold)
	}
}

func TestCacheHitRatioLowMetric_Evaluate_NilRepo(t *testing.T) {
	m := monitor.NewCacheHitRatioLowMetric(nil)
	v, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 0 {
		t.Errorf("Evaluate() with nil repo = %v, want 0", v)
	}
}

func TestCacheHitRatioLowMetric_Evaluate_InsufficientSamples(t *testing.T) {
	// Low ratio but only 19 samples: must not breach.
	m := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroup("deepseek", "deepseek-chat", 19, 19000, 1000),
		},
	})
	v, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 0 {
		t.Errorf("Evaluate() = %v, want 0 (samples < 20)", v)
	}
}

func TestCacheHitRatioLowMetric_Evaluate_ThresholdBoundary(t *testing.T) {
	// ratio exactly 0.5 is NOT below the default 0.5 threshold.
	m := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroup("deepseek", "deepseek-chat", 30, 30000, 15000),
		},
	})
	v, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 0 {
		t.Errorf("Evaluate() at exact threshold = %v, want 0", v)
	}

	// ratio 0.4 < 0.5: breach.
	m2 := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroup("deepseek", "deepseek-chat", 30, 30000, 12000),
	},
	})
	v, err = m2.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 1 {
		t.Errorf("Evaluate() below threshold = %v, want 1 breaching group", v)
	}
}

func TestCacheHitRatioLowMetric_Evaluate_UsesP50NotWeighted(t *testing.T) {
	// N7 (2026-08-13 链路审查): compaction turns rewrite history into a fresh
	// prompt that busts the cache; their large token counts dominate the
	// token-weighted ratio and would false-positive whenever compression
	// fires. The alert must key on the median per-turn ratio (P50), which is
	// robust to those outliers.
	m := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroupP50("deepseek", "deepseek-chat", 30, 300000, 60000, 0.8), // weighted 0.2, p50 0.8
		},
	})
	v, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 0 {
		t.Errorf("Evaluate() = %v, want 0 (P50 0.8 healthy despite weighted 0.2)", v)
	}

	// Inverse: a healthy weighted ratio must not mask a low median.
	m2 := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroupP50("openai", "gpt-4o", 30, 300000, 270000, 0.3), // weighted 0.9, p50 0.3
		},
	})
	v, err = m2.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 1 {
		t.Fatalf("Evaluate() = %v, want 1 (P50 0.3 below threshold)", v)
	}
	_, payload := m2.BreachDetails()
	breaches, ok := payload["breaches"].([]map[string]any)
	if !ok || len(breaches) != 1 {
		t.Fatalf("breaches payload = %v, want 1 entry", payload["breaches"])
	}
	b := breaches[0]
	if b["provider"] != "openai" || b["model"] != "gpt-4o" {
		t.Errorf("breach key = %v/%v, want openai/gpt-4o", b["provider"], b["model"])
	}
	if b["samples"] != 30 {
		t.Errorf("breach samples = %v, want 30", b["samples"])
	}
	if ratio, _ := b["hit_ratio"].(float64); ratio != 0.3 {
		t.Errorf("breach hit_ratio = %v, want P50 0.3", ratio)
	}
}

func TestCacheHitRatioLowMetric_BreachDetails_NoBreach(t *testing.T) {
	m := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{})
	if _, err := m.Evaluate(context.Background(), time.Hour); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	summary, payload := m.BreachDetails()
	if summary != "" || payload != nil {
		t.Errorf("BreachDetails() = %q/%v, want empty without breach", summary, payload)
	}
}

func TestCacheHitRatioLowMetric_Evaluate_RepoError(t *testing.T) {
	m := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{err: errStubCacheHitStats})
	if _, err := m.Evaluate(context.Background(), time.Hour); err == nil {
		t.Error("Evaluate() with repo error = nil, want error")
	}
}

func TestCacheHitRatioLowMetric_EnvThresholdOverride(t *testing.T) {
	// Ratio 0.4 breaches the default 0.5 but not an env-overridden 0.3.
	t.Setenv("MONITOR_LLM_CACHE_HIT_RATIO_THRESHOLD", "0.3")
	m := monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroup("deepseek", "deepseek-chat", 30, 30000, 12000),
	},
	})
	v, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 0 {
		t.Errorf("Evaluate() with env threshold 0.3 = %v, want 0 (ratio 0.4 >= 0.3)", v)
	}
}

func TestEvaluateAlerts_CacheHitRatioLowFires(t *testing.T) {
	var firedEvent *monitor.EventWrite
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r-cache",
					Name:            "LLM cache hit ratio low",
					MetricKey:       "llm.cache_hit_ratio_low",
					Threshold:       1,
					WindowMinutes:   60,
					Enabled:         true,
					Severity:        "warning",
					CooldownMinutes: 60,
				},
			}, nil
		},
		insertMonitorEventFn: func(_ context.Context, ev monitor.EventWrite) error {
			if ev.EventKey == "alert.fired" {
				cp := ev
				firedEvent = &cp
			}
			return nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	var notifyPayload map[string]any
	notifier := &mockNotifier{
		notifyFn: func(_ context.Context, _ monitor.AlertRule, payload map[string]any) {
			notifyPayload = payload
		},
	}
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(monitor.NewCacheHitRatioLowMetric(&stubCacheHitStatsRepo{
		stats: []usage.CacheHitRatioStat{
			cacheHitGroup("deepseek", "deepseek-chat", 25, 50000, 13000),
		},
	}))
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.SetRegistry(reg)

	uc.EvaluateAlerts(context.Background())

	if firedEvent == nil {
		t.Fatal("no alert.fired event recorded")
	}
	if firedEvent.Status != "warning" {
		t.Errorf("fired event status = %q, want warning", firedEvent.Status)
	}
	if !strings.Contains(firedEvent.Description, "deepseek") || !strings.Contains(firedEvent.Description, "deepseek-chat") {
		t.Errorf("fired description %q missing provider/model", firedEvent.Description)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(firedEvent.MetadataJSON), &meta); err != nil {
		t.Fatalf("fired metadata unmarshal: %v", err)
	}
	if _, ok := meta["breaches"]; !ok {
		t.Errorf("fired metadata missing breaches: %s", firedEvent.MetadataJSON)
	}
	if notifyPayload == nil {
		t.Fatal("notifier not called")
	}
	if _, ok := notifyPayload["breaches"]; !ok {
		t.Errorf("notify payload missing breaches: %v", notifyPayload)
	}
}

func TestDefaultAlertRules_IncludesCacheHitRatioLow(t *testing.T) {
	found := false
	for _, r := range monitor.DefaultAlertRules() {
		if r.MetricKey == "llm.cache_hit_ratio_low" {
			found = true
			if r.Threshold != 1 || r.WindowMinutes != 60 || r.Severity != "warning" || !r.Enabled {
				t.Errorf("default rule = %+v, want threshold 1, window 60, warning, enabled", r)
			}
		}
	}
	if !found {
		t.Error("DefaultAlertRules() missing llm.cache_hit_ratio_low rule")
	}
}
