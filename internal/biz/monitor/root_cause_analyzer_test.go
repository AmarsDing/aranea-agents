package monitor_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

func TestRootCauseEngine_ImplementsRootCauseAnalyzer(t *testing.T) {
	// Compile-time check: *RootCauseEngine must satisfy RootCauseAnalyzer
	var _ monitor.RootCauseAnalyzer = (*monitor.RootCauseEngine)(nil)
}

func TestRootCauseAnalyzer_Analyze_DelegatesToEvaluate(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	// Test: Analyze returns the first result from Evaluate
	result, err := engine.Analyze(ctx, "llm.call", "error", nil, map[string]any{"error_message": "request timeout exceeded"})
	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Analyze() returned nil result, want non-nil for matching rule")
	}
	if result.RuleID != "rc-provider-timeout" {
		t.Errorf("Analyze() RuleID = %q, want %q", result.RuleID, "rc-provider-timeout")
	}
}

func TestRootCauseAnalyzer_Analyze_NoMatch(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	result, err := engine.Analyze(ctx, "unknown.step", "error", nil, nil)
	if err != nil {
		t.Fatalf("Analyze() returned error: %v", err)
	}
	if result != nil {
		t.Errorf("Analyze() returned non-nil result for no-match case, want nil; got RuleID=%q", result.RuleID)
	}
}

func TestRootCauseAnalyzer_Analyze_NilReceiver(t *testing.T) {
	var engine *monitor.RootCauseEngine
	ctx := context.Background()

	result, err := engine.Analyze(ctx, "llm.call", "error", nil, nil)
	if err != nil {
		t.Fatalf("Analyze() on nil receiver returned error: %v", err)
	}
	if result != nil {
		t.Errorf("Analyze() on nil receiver returned non-nil, want nil")
	}
}
