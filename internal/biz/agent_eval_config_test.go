package biz

import "testing"

func TestParseAgentEvalAutoConfig(t *testing.T) {
	t.Parallel()
	cfg := ParseAgentEvalAutoConfig(`{"evaluation":{"auto_after_turn":true,"dataset_id":"ds-1","metrics":"exact_match","num_runs":3,"min_interval_sec":60}}`)
	if !cfg.Enabled || cfg.DatasetID != "ds-1" || cfg.NumRuns != 3 || cfg.MinIntervalSec != 60 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if ParseAgentEvalAutoConfig("").Enabled {
		t.Fatal("empty config should be disabled")
	}
}
