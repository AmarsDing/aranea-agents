package evaluation

import "testing"

func TestParseMetrics(t *testing.T) {
	t.Parallel()
	all := parseMetrics("")
	if len(all) != 4 {
		t.Fatalf("expected 4 metrics, got %d", len(all))
	}
	subset := parseMetrics("exact_match, llm_as_judge")
	if !subset[MetricExactMatch] || !subset[MetricLLMAsJudge] || subset[MetricContainsMatch] {
		t.Fatalf("unexpected subset: %+v", subset)
	}
}

func TestScoreToolCallAccuracy(t *testing.T) {
	t.Parallel()
	if got := scoreToolCallAccuracy("", "output"); got != 0 {
		t.Fatalf("empty meta: %v", got)
	}
	meta := `{"expected_tools":["get_weather","search"]}`
	got := scoreToolCallAccuracy(meta, "called get_weather tool")
	if got != 0.5 {
		t.Fatalf("expected 0.5, got %v", got)
	}
}

func TestBoolToFloat(t *testing.T) {
	t.Parallel()
	if boolToFloat(true) != 1 || boolToFloat(false) != 0 {
		t.Fatal("boolToFloat mismatch")
	}
}
