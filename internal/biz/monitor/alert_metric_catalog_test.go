package monitor_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
)

type stubCatalogMetric struct {
	keyVal  string
	descVal string
	info    monitor.AlertMetricInfo
	value   float64
	evalErr error
}

func (m *stubCatalogMetric) Key() string         { return m.keyVal }
func (m *stubCatalogMetric) Description() string { return m.descVal }
func (m *stubCatalogMetric) Evaluate(ctx context.Context, window time.Duration) (float64, error) {
	return m.value, m.evalErr
}
func (m *stubCatalogMetric) Catalog() monitor.AlertMetricInfo { return m.info }

func TestAlertMetricRegistry_ListCatalog_WithProvider(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	r.Register(&stubCatalogMetric{
		keyVal: "runner.error_rate",
		info: monitor.AlertMetricInfo{
			Key: "runner.error_rate", Name: "Runner error rate", Unit: "ratio",
			DefaultWindowMinutes: 60, SuggestedThreshold: 0.25,
		},
	})
	// Metric without Catalog() falls back to Key/Description.
	r.Register(&stubAlertMetric{keyVal: "legacy.metric", descVal: "Legacy desc"})

	infos := r.ListCatalog()
	if len(infos) != 2 {
		t.Fatalf("ListCatalog() = %d items, want 2", len(infos))
	}
	// Sorted by key: legacy.metric first.
	if infos[0].Key != "legacy.metric" || infos[0].Description != "Legacy desc" {
		t.Errorf("fallback entry = %+v", infos[0])
	}
	if infos[1].Key != "runner.error_rate" || infos[1].Unit != "ratio" || infos[1].SuggestedThreshold != 0.25 {
		t.Errorf("catalog entry = %+v", infos[1])
	}
}

func TestUsecase_ListAlertMetricCatalog(t *testing.T) {
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(&stubCatalogMetric{
		keyVal: "runner.error_rate",
		info: monitor.AlertMetricInfo{
			Key: "runner.error_rate", Name: "Runner error rate", Unit: "ratio",
			DefaultWindowMinutes: 60, SuggestedThreshold: 0.25,
		},
		value: 0.12,
	})
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil, monitor.WithRegistry(reg))

	entries := uc.ListAlertMetricCatalog(context.Background())
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1", len(entries))
	}
	e := entries[0]
	if e.Key != "runner.error_rate" || e.CurrentValue != 0.12 {
		t.Errorf("entry = %+v", e)
	}
	if e.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt is zero")
	}
}

func TestUsecase_ListAlertMetricCatalog_EvalErrorKeepsZero(t *testing.T) {
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(&stubCatalogMetric{
		keyVal:  "broken.metric",
		info:    monitor.AlertMetricInfo{Key: "broken.metric", Name: "Broken", Unit: "count"},
		evalErr: context.DeadlineExceeded,
	})
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil, monitor.WithRegistry(reg))

	entries := uc.ListAlertMetricCatalog(context.Background())
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1", len(entries))
	}
	if entries[0].CurrentValue != 0 {
		t.Errorf("CurrentValue=%v want 0 on eval error", entries[0].CurrentValue)
	}
}

func TestUsecase_ListAlertMetricCatalog_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	if got := uc.ListAlertMetricCatalog(context.Background()); got != nil {
		t.Errorf("nil usecase = %v, want nil", got)
	}
}

func TestBuiltinMetrics_HaveCatalog(t *testing.T) {
	// All built-in metrics must expose catalog metadata so the Alerts page
	// can render a human-readable metric directory.
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(monitor.NewRunnerErrorRateMetric(nil, nil))
	reg.Register(monitor.NewSkillFilesystemMissingMetric(nil))
	reg.Register(monitor.NewSequencerDeadLetterMetric(nil))

	for _, info := range reg.ListCatalog() {
		if info.Name == "" {
			t.Errorf("metric %q missing catalog Name", info.Key)
		}
		if info.Unit == "" {
			t.Errorf("metric %q missing catalog Unit", info.Key)
		}
		if info.Description == "" {
			t.Errorf("metric %q missing catalog Description", info.Key)
		}
	}
}
