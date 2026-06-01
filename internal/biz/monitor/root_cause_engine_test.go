package monitor_test

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

func TestNewRootCauseEngine(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	if e == nil {
		t.Fatal("NewRootCauseEngine(loggateway.NewNoop()) = nil, want non-nil")
	}
}

func TestRootCauseEngine_Evaluate_NilReceiver(t *testing.T) {
	var e *monitor.RootCauseEngine
	got := e.Evaluate(context.Background(), "llm.call", "error", nil)
	if got != nil {
		t.Fatalf("nil.Evaluate() = %v, want nil", got)
	}
}

func TestRootCauseEngine_Evaluate_BuiltinRules(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	tests := []struct {
		name      string
		stepID    string
		phase     string
		metadata  map[string]any
		wantMatch bool
		wantRuleID string
	}{
		{
			name:       "provider_timeout_llm_step",
			stepID:     "llm.call",
			phase:      "error",
			metadata:   map[string]any{"error_message": "request timeout exceeded"},
			wantMatch:  true,
			wantRuleID: "rc-provider-timeout",
		},
		{
			name:      "provider_timeout_wrong_phase",
			stepID:    "llm.call",
			phase:     "done",
			metadata:  map[string]any{"error_message": "request timeout exceeded"},
			wantMatch: false,
		},
		{
			name:      "provider_timeout_wrong_step",
			stepID:    "tool.exec",
			phase:     "error",
			metadata:  map[string]any{"error_message": "request timeout exceeded"},
			wantMatch: true,
			wantRuleID: "rc-tool-execution-failure",
		},
		{
			name:       "provider_rate_limit_429",
			stepID:     "llm.call",
			phase:      "error",
			metadata:   map[string]any{"error_code": "429"},
			wantMatch:  true,
			wantRuleID: "rc-provider-rate-limit",
		},
		{
			name:       "provider_rate_limit_code_field",
			stepID:     "llm.call",
			phase:      "error",
			metadata:   map[string]any{"code": "rate_limit"},
			wantMatch:  true,
			wantRuleID: "rc-provider-rate-limit",
		},
		{
			name:      "provider_rate_limit_wrong_step",
			stepID:    "tool.exec",
			phase:     "error",
			metadata:  map[string]any{"error_code": "429"},
			wantMatch: true,
			wantRuleID: "rc-tool-execution-failure",
		},
		{
			name:       "tool_execution_failure",
			stepID:     "tool.search",
			phase:      "error",
			metadata:   nil,
			wantMatch:  true,
			wantRuleID: "rc-tool-execution-failure",
		},
		{
			name:      "tool_execution_wrong_phase",
			stepID:    "tool.search",
			phase:     "done",
			metadata:  nil,
			wantMatch: false,
		},
		{
			name:       "memory_read_error",
			stepID:     "memory.retrieve",
			phase:      "error",
			metadata:   nil,
			wantMatch:  true,
			wantRuleID: "rc-memory-read-error",
		},
		{
			name:       "mcp_connection_failure",
			stepID:     "mcp.connect",
			phase:      "error",
			metadata:   map[string]any{"error_message": "connection refused"},
			wantMatch:  true,
			wantRuleID: "rc-mcp-connection-failure",
		},
		{
			name:      "mcp_connection_no_match",
			stepID:    "mcp.connect",
			phase:     "error",
			metadata:  map[string]any{"error_message": "some other error"},
			wantMatch: false,
		},
		{
			name:      "no_matching_step",
			stepID:    "unknown.step",
			phase:     "error",
			metadata:  nil,
			wantMatch: false,
		},
		{
			name:      "empty_step_id",
			stepID:    "",
			phase:     "error",
			metadata:  nil,
			wantMatch: false,
		},
		{
			name:       "provider_timeout_timed_out_variant",
			stepID:     "llm.complete",
			phase:      "error",
			metadata:   map[string]any{"error_message": "operation timed out"},
			wantMatch:  true,
			wantRuleID: "rc-provider-timeout",
		},
		{
			name:       "provider_timeout_deadline_exceeded",
			stepID:     "llm.request",
			phase:      "error",
			metadata:   map[string]any{"error_message": "deadline exceeded"},
			wantMatch:  true,
			wantRuleID: "rc-provider-timeout",
		},
		{
			name:       "mcp_dial_tcp",
			stepID:     "mcp.connect",
			phase:      "error",
			metadata:   map[string]any{"error_message": "dial tcp 127.0.0.1:8080: connection refused"},
			wantMatch:  true,
			wantRuleID: "rc-mcp-connection-failure",
		},
		{
			name:       "mcp_no_such_host",
			stepID:     "mcp.connect",
			phase:      "error",
			metadata:   map[string]any{"error_message": "no such host"},
			wantMatch:  true,
			wantRuleID: "rc-mcp-connection-failure",
		},
		{
			name:       "rate_limit_exceeded_code",
			stepID:     "llm.call",
			phase:      "error",
			metadata:   map[string]any{"error_code": "rate_limit_exceeded"},
			wantMatch:  true,
			wantRuleID: "rc-provider-rate-limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := e.Evaluate(ctx, tt.stepID, tt.phase, tt.metadata)
			if tt.wantMatch {
				if len(results) == 0 {
					t.Fatalf("Evaluate() returned no results, want at least one matching rule %q", tt.wantRuleID)
				}
				found := false
				for _, r := range results {
					if r.RuleID == tt.wantRuleID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Evaluate() did not return rule %q; got rule IDs: %v", tt.wantRuleID, ruleIDs(results))
				}
			} else {
				if len(results) > 0 {
					t.Errorf("Evaluate() returned %d results, want 0; rule IDs: %v", len(results), ruleIDs(results))
				}
			}
		})
	}
}

func TestRootCauseEngine_Evaluate_Confidence(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	results := e.Evaluate(ctx, "llm.call", "error", map[string]any{"error_code": "429"})
	if len(results) == 0 {
		t.Fatal("Evaluate() returned no results")
	}
	for _, r := range results {
		if r.Confidence < 0.6 {
			t.Errorf("Confidence = %.2f, want >= 0.6", r.Confidence)
		}
		if r.Confidence > 1.0 {
			t.Errorf("Confidence = %.2f, want <= 1.0", r.Confidence)
		}
	}
}

func TestRootCauseEngine_Evaluate_MultipleMatches(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	results := e.Evaluate(ctx, "llm.call", "error", map[string]any{
		"error_message": "timeout exceeded",
		"error_code":    "429",
	})
	if len(results) < 2 {
		t.Errorf("Evaluate() returned %d results, want >= 2 for multi-match scenario", len(results))
	}
}

func TestRootCauseEngine_Evaluate_MetadataPassedThrough(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()
	meta := map[string]any{"error_code": "429", "detail": "test"}

	results := e.Evaluate(ctx, "llm.call", "error", meta)
	if len(results) == 0 {
		t.Fatal("Evaluate() returned no results")
	}
	if results[0].Metadata == nil {
		t.Error("RootCauseResult.Metadata = nil, want non-nil")
	}
}

func TestRootCauseEngine_Evaluate_WildcardStepID(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	tests := []struct {
		name      string
		stepID    string
		phase     string
		metadata  map[string]any
		wantCount int
	}{
		{"llm_prefix_match", "llm.anything", "error", map[string]any{"error_message": "timeout"}, 1},
		{"llm_prefix_no_error_msg", "llm.anything", "error", nil, 0},
		{"tool_prefix", "tool.anything", "error", nil, 1},
		{"memory_prefix", "memory.anything", "error", nil, 1},
		{"mcp_prefix_no_pattern_match", "mcp.anything", "error", map[string]any{"error_message": "unknown"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := e.Evaluate(ctx, tt.stepID, tt.phase, tt.metadata)
			if len(results) != tt.wantCount {
				t.Errorf("Evaluate() returned %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestRootCauseEngine_Evaluate_ErrorCodeFallback(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	results := e.Evaluate(ctx, "llm.call", "error", map[string]any{
		"code": "429",
	})
	found := false
	for _, r := range results {
		if r.RuleID == "rc-provider-rate-limit" {
			found = true
		}
	}
	if !found {
		t.Error("Evaluate() did not match rate limit rule with 'code' field fallback")
	}
}

func TestRootCauseEngine_Evaluate_ErrorMessageFallback(t *testing.T) {
	e := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	results := e.Evaluate(ctx, "llm.call", "error", map[string]any{
		"message": "connection refused",
	})
	found := false
	for _, r := range results {
		if r.RuleID == "rc-mcp-connection-failure" {
			found = true
		}
	}
	if found {
		t.Error("Evaluate() should not match MCP rule for llm stepID even with 'message' fallback")
	}
}

func TestRootCauseResultsToJSON_Empty(t *testing.T) {
	got := monitor.RootCauseResultsToJSON(nil)
	if got != "[]" {
		t.Errorf("RootCauseResultsToJSON(nil) = %q, want %q", got, "[]")
	}

	got2 := monitor.RootCauseResultsToJSON([]monitor.RootCauseResult{})
	if got2 != "[]" {
		t.Errorf("RootCauseResultsToJSON(empty) = %q, want %q", got2, "[]")
	}
}

func TestRootCauseResultsToJSON_WithResults(t *testing.T) {
	results := []monitor.RootCauseResult{
		{RuleID: "rc-1", Name: "Test", RootCause: "cause", FixSuggest: "fix", Severity: "high", Confidence: 0.8},
	}
	got := monitor.RootCauseResultsToJSON(results)
	if got == "" || got == "[]" {
		t.Fatalf("RootCauseResultsToJSON() = %q, want non-empty JSON", got)
	}
	var parsed []monitor.RootCauseResult
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("RootCauseResultsToJSON() produced invalid JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Errorf("parsed length = %d, want 1", len(parsed))
	}
	if parsed[0].RuleID != "rc-1" {
		t.Errorf("parsed[0].RuleID = %q, want %q", parsed[0].RuleID, "rc-1")
	}
	if parsed[0].Confidence != 0.8 {
		t.Errorf("parsed[0].Confidence = %v, want 0.8", parsed[0].Confidence)
	}
}

func TestRootCauseResultsToJSON_MultipleResults(t *testing.T) {
	results := []monitor.RootCauseResult{
		{RuleID: "rc-1", Name: "A", Confidence: 0.7},
		{RuleID: "rc-2", Name: "B", Confidence: 0.9},
	}
	got := monitor.RootCauseResultsToJSON(results)
	var parsed []monitor.RootCauseResult
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("parsed length = %d, want 2", len(parsed))
	}
}

func TestMatchStepID(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		stepID  string
		want    bool
	}{
		{"exact_match", "llm.call", "llm.call", true},
		{"no_match", "llm.call", "tool.exec", false},
		{"wildcard_prefix_match", "llm*", "llm.call", true},
		{"wildcard_no_match", "tool*", "llm.call", false},
		{"wildcard_empty_step", "llm*", "", false},
		{"empty_both", "", "", true},
		{"empty_pattern_nonempty_step", "", "llm.call", false},
		{"wildcard_exact_prefix", "llm*", "llm", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.MatchStepID(tt.pattern, tt.stepID)
			if got != tt.want {
				t.Errorf("MatchStepID(%q, %q) = %v, want %v", tt.pattern, tt.stepID, got, tt.want)
			}
		})
	}
}

func TestMatchPrerequisite(t *testing.T) {
	tests := []struct {
		name string
		pre  monitor.Prerequisite
		meta map[string]any
		want bool
	}{
		{
			name: "step_id_match",
			pre:  monitor.Prerequisite{StepID: "llm.call"},
			meta: map[string]any{"step_id": "llm.call"},
			want: true,
		},
		{
			name: "step_id_no_match",
			pre:  monitor.Prerequisite{StepID: "llm.call"},
			meta: map[string]any{"step_id": "tool.exec"},
			want: false,
		},
		{
			name: "phase_match_case_insensitive",
			pre:  monitor.Prerequisite{Phase: "error"},
			meta: map[string]any{"flow_phase": "Error"},
			want: true,
		},
		{
			name: "phase_no_match",
			pre:  monitor.Prerequisite{Phase: "error"},
			meta: map[string]any{"flow_phase": "done"},
			want: false,
		},
		{
			name: "both_match",
			pre:  monitor.Prerequisite{StepID: "llm.call", Phase: "error"},
			meta: map[string]any{"step_id": "llm.call", "flow_phase": "error"},
			want: true,
		},
		{
			name: "nil_metadata",
			pre:  monitor.Prerequisite{StepID: "llm.call"},
			meta: nil,
			want: false,
		},
		{
			name: "empty_prerequisite",
			pre:  monitor.Prerequisite{},
			meta: map[string]any{},
			want: true,
		},
		{
			name: "nil_metadata_empty_pre",
			pre:  monitor.Prerequisite{},
			meta: nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.MatchPrerequisite(tt.pre, tt.meta)
			if got != tt.want {
				t.Errorf("MatchPrerequisite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ruleIDs(results []monitor.RootCauseResult) []string {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.RuleID
	}
	return ids
}
