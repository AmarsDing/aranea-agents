package service

import (
	"testing"
	"time"

	"aranea-agents/internal/biz/inbox"
	"aranea-agents/internal/biz/monitor/alert"
)

func TestInboxFiringAlerts(t *testing.T) {
	fired := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	ratio := func(key string, v float64) alert.AlertMetricCatalogEntry {
		return alert.AlertMetricCatalogEntry{AlertMetricInfo: alert.AlertMetricInfo{Key: key, Unit: "ratio"}, CurrentValue: v}
	}
	count := func(key string, v float64) alert.AlertMetricCatalogEntry {
		return alert.AlertMetricCatalogEntry{AlertMetricInfo: alert.AlertMetricInfo{Key: key, Unit: "count"}, CurrentValue: v}
	}
	rules := []alert.AlertRule{
		{ID: "r-errors", Name: "运行失败率", MetricKey: "runner.error_rate", Threshold: 0.25, Enabled: true, Severity: "warning", LastFiredAt: &fired},
		{ID: "r-dead", Name: "死信积压", MetricKey: "sequencer.dead_letter_count", Threshold: 1, Enabled: true, Severity: "critical"},
		// 引擎里还停在 firing，但当前没有证据（值低于阈值 / 没有数据）→ 不进收件箱
		{ID: "r-stale", MetricKey: "llm.cache_hit_ratio_low", Threshold: 1, Enabled: true, FiringState: alert.AlertFiringStateFiring},
		{ID: "r-nodata", MetricKey: "unknown.metric", Threshold: 1, Enabled: true, FiringState: alert.AlertFiringStateFiring},
		{ID: "r-disabled", MetricKey: "runner.error_rate", Threshold: 0.1, Enabled: false},
	}
	catalog := []alert.AlertMetricCatalogEntry{
		ratio("runner.error_rate", 0.5),
		count("sequencer.dead_letter_count", 3),
		count("llm.cache_hit_ratio_low", 0),
	}
	got := inboxFiringAlerts(rules, catalog, 0)
	if len(got) != 2 || got[0].NativeID != "r-errors" || got[1].NativeID != "r-dead" {
		t.Fatalf("want only enabled rules whose current value meets the threshold, got %+v", got)
	}
	first := got[0]
	if first.Kind != inbox.KindException || first.Risk != inbox.RiskMedium || len(first.Actions) != 0 ||
		first.Link != "/monitor/health" || !first.CreatedAt.Equal(fired) {
		t.Fatalf("unexpected item: %+v", first)
	}
	if first.Summary != "runner.error_rate 当前 50.0%，阈值 25.0%" {
		t.Fatalf("summary = %q", first.Summary)
	}
	if got[1].Risk != inbox.RiskHigh || got[1].Summary != "sequencer.dead_letter_count 当前 3，阈值 1" {
		t.Fatalf("critical item = %+v", got[1])
	}
	if n := len(inboxFiringAlerts(rules, catalog, 1)); n != 1 {
		t.Fatalf("limit not applied: %d", n)
	}
}

func TestAggregatorRegisterAfterConstruction(t *testing.T) {
	agg := inbox.NewAggregator()
	agg.Register(NewInboxAlertSource(nil)) // nil 服务 → 不注册
	agg.Register(&inboxAlertSource{})
	if got := agg.Sources(); len(got) != 1 || got[0] != inboxSourceMonitorAlert {
		t.Fatalf("sources = %v", got)
	}
}
