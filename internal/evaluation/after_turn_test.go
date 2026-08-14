package evaluation

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestAfterTurnSamplePass(t *testing.T) {
	tr := NewAfterTurnTrigger(nil, nil, loggateway.NewNoop())

	// Out-of-range rates always pass (backward-compatible default).
	tr.randFn = func() float64 { return 0.99 }
	if !tr.samplePass(0) || !tr.samplePass(1.5) || !tr.samplePass(1) {
		t.Fatal("rate<=0 or >=1 must always pass")
	}

	// Deterministic coin: rand below rate passes, at/above rate fails.
	tr.randFn = func() float64 { return 0.09 }
	if !tr.samplePass(0.1) {
		t.Fatal("rand 0.09 < rate 0.1 must pass")
	}
	tr.randFn = func() float64 { return 0.1 }
	if tr.samplePass(0.1) {
		t.Fatal("rand 0.1 >= rate 0.1 must fail")
	}

	// Nil randFn falls back to math/rand without panicking.
	tr.randFn = nil
	_ = tr.samplePass(0.5)
}

func TestParseAgentEvalAutoConfigSampleAndAlert(t *testing.T) {
	cfg := biz.ParseAgentEvalAutoConfig(
		`{"evaluation":{"auto_after_turn":true,"dataset_id":"ds-1","sample_rate":0.1,"alert_consecutive_drops":3,"alert_metric":"exact_match"}}`)
	if cfg.SampleRate != 0.1 {
		t.Fatalf("sample_rate = %v, want 0.1", cfg.SampleRate)
	}
	if cfg.AlertConsecutiveDrops != 3 {
		t.Fatalf("alert_consecutive_drops = %d, want 3", cfg.AlertConsecutiveDrops)
	}
	if cfg.AlertMetric != "exact_match" {
		t.Fatalf("alert_metric = %q, want exact_match", cfg.AlertMetric)
	}

	// Defaults: no sample/alert keys → full sampling, alert disabled, judge metric.
	def := biz.ParseAgentEvalAutoConfig(`{"evaluation":{"auto_after_turn":true,"dataset_id":"ds-1"}}`)
	if def.SampleRate != 1 {
		t.Fatalf("default sample_rate = %v, want 1", def.SampleRate)
	}
	if def.AlertConsecutiveDrops != 0 {
		t.Fatalf("default alert_consecutive_drops = %d, want 0", def.AlertConsecutiveDrops)
	}
	if def.AlertMetric != "llm_as_judge" {
		t.Fatalf("default alert_metric = %q, want llm_as_judge", def.AlertMetric)
	}

	// Out-of-range values normalize.
	bad := biz.ParseAgentEvalAutoConfig(`{"evaluation":{"auto_after_turn":true,"dataset_id":"ds-1","sample_rate":7,"alert_consecutive_drops":-2}}`)
	if bad.SampleRate != 1 || bad.AlertConsecutiveDrops != 0 {
		t.Fatalf("out-of-range values must normalize, got %+v", bad)
	}
}
