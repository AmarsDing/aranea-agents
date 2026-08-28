package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

func TestMergeUsageSourceMetadata(t *testing.T) {
	t.Run("empty_source_passthrough", func(t *testing.T) {
		if got := MergeUsageSourceMetadata(`{"a":1}`, ""); got != `{"a":1}` {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("sets_key_on_empty_object", func(t *testing.T) {
		got := MergeUsageSourceMetadata("{}", "estimated")
		var payload map[string]any
		if err := json.Unmarshal([]byte(got), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload[MetadataKeyUsageSource] != "estimated" {
			t.Fatalf("usage_source=%v", payload[MetadataKeyUsageSource])
		}
	})
	t.Run("preserves_existing_keys_and_overwrites_source", func(t *testing.T) {
		got := MergeUsageSourceMetadata(`{"trace_id":"t1","usage_source":"streaming"}`, "estimated")
		var payload map[string]any
		if err := json.Unmarshal([]byte(got), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload["trace_id"] != "t1" {
			t.Fatalf("trace_id lost: %v", payload)
		}
		if payload[MetadataKeyUsageSource] != "estimated" {
			t.Fatalf("usage_source=%v want estimated", payload[MetadataKeyUsageSource])
		}
	})
	t.Run("invalid_json_replaced_with_fresh_payload", func(t *testing.T) {
		got := MergeUsageSourceMetadata("not-json", "streaming")
		var payload map[string]any
		if err := json.Unmarshal([]byte(got), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload[MetadataKeyUsageSource] != "streaming" {
			t.Fatalf("usage_source=%v", payload[MetadataKeyUsageSource])
		}
	})
	t.Run("whitespace_source_passthrough", func(t *testing.T) {
		if got := MergeUsageSourceMetadata(`{"a":1}`, "  "); got != `{"a":1}` {
			t.Fatalf("got %q", got)
		}
	})
}

func TestMergeWaitMSMetadata(t *testing.T) {
	if got := MergeWaitMSMetadata(`{"a":1}`, 0, 1000); got != `{"a":1}` {
		t.Fatalf("zero wait passthrough: %q", got)
	}
	got := MergeWaitMSMetadata("{}", 300000, 320000)
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload[MetadataKeyWaitMS] != float64(300000) {
		t.Fatalf("wait_ms=%v", payload[MetadataKeyWaitMS])
	}
	if payload[MetadataKeyModelLatencyMS] != float64(20000) {
		t.Fatalf("model_latency_ms=%v", payload[MetadataKeyModelLatencyMS])
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"ok_to_success", "ok", "success"},
		{"success_passthrough", "success", "success"},
		{"empty_to_success", "", "success"},
		{"fail_to_failed", "fail", "failed"},
		{"failed_passthrough", "failed", "failed"},
		{"error_to_failed", "error", "failed"},
		{"timeout_passthrough", "timeout", "timeout"},
		{"timed_out_to_timeout", "timed_out", "timeout"},
		{"cancelled_passthrough", "cancelled", "cancelled"},
		{"canceled_to_cancelled", "canceled", "cancelled"},
		{"unknown_value_passthrough", "unknown_value", "unknown_value"},
		{"uppercase_ok", "OK", "success"},
		{"mixed_case_Fail", "Fail", "failed"},
		{"spaced_ok", " ok ", "success"},
		{"spaced_fail", " fail ", "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeStatus(tt.input)
			if got != tt.expect {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestApplyTokenUsageCosts(t *testing.T) {
	t.Run("nil_event", func(t *testing.T) {
		ApplyTokenUsageCosts(nil)
	})

	t.Run("input_cost_from_usd_per_1m", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:        1000,
			InputPriceUSDPer1M: 3.0,
		}
		ApplyTokenUsageCosts(e)
		want := int64(math.Round(float64(1000) * 3.0))
		if e.InputCostMicroUSD != want {
			t.Errorf("InputCostMicroUSD = %d, want %d", e.InputCostMicroUSD, want)
		}
	})

	t.Run("output_cost_from_usd_per_1m", func(t *testing.T) {
		e := &TokenUsageEvent{
			OutputTokens:        500,
			OutputPriceUSDPer1M: 15.0,
		}
		ApplyTokenUsageCosts(e)
		want := int64(math.Round(float64(500) * 15.0))
		if e.OutputCostMicroUSD != want {
			t.Errorf("OutputCostMicroUSD = %d, want %d", e.OutputCostMicroUSD, want)
		}
	})

	t.Run("input_cost_micro_per_1k_fallback", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:             2000,
			InputPriceMicroUSDPer1K: 500,
		}
		ApplyTokenUsageCosts(e)
		want := int64(2000) * 500 / 1000
		if e.InputCostMicroUSD != want {
			t.Errorf("InputCostMicroUSD = %d, want %d", e.InputCostMicroUSD, want)
		}
	})

	t.Run("usd_per_1m_takes_priority_over_micro_per_1k", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:             1000,
			InputPriceMicroUSDPer1K: 9999,
			InputPriceUSDPer1M:      3.0,
		}
		ApplyTokenUsageCosts(e)
		want := int64(math.Round(float64(1000) * 3.0))
		if e.InputCostMicroUSD != want {
			t.Errorf("InputCostMicroUSD = %d, want %d (should use USDPer1M path)", e.InputCostMicroUSD, want)
		}
	})

	t.Run("zero_tokens_skips_cost", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:         0,
			InputPriceUSDPer1M:  3.0,
			OutputTokens:        0,
			OutputPriceUSDPer1M: 15.0,
		}
		ApplyTokenUsageCosts(e)
		if e.InputCostMicroUSD != 0 {
			t.Errorf("InputCostMicroUSD = %d, want 0 for zero tokens", e.InputCostMicroUSD)
		}
		if e.OutputCostMicroUSD != 0 {
			t.Errorf("OutputCostMicroUSD = %d, want 0 for zero tokens", e.OutputCostMicroUSD)
		}
	})

	t.Run("total_cost_is_sum_of_all_fields", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:         1000,
			InputPriceUSDPer1M:  3.0,
			OutputTokens:        500,
			OutputPriceUSDPer1M: 15.0,
		}
		ApplyTokenUsageCosts(e)
		wantTotal := e.InputCostMicroUSD + e.OutputCostMicroUSD +
			e.CachedInputCostMicroUSD + e.CacheWriteCostMicroUSD +
			e.ReasoningCostMicroUSD + e.EmbeddingCostMicroUSD
		if e.TotalCostMicroUSD != wantTotal {
			t.Errorf("TotalCostMicroUSD = %d, want %d", e.TotalCostMicroUSD, wantTotal)
		}
	})

	t.Run("all_six_cost_kinds", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:             100,
			InputPriceUSDPer1M:      2.0,
			OutputTokens:            200,
			OutputPriceUSDPer1M:     4.0,
			CachedInputTokens:       50,
			CacheReadPriceUSDPer1M:  1.0,
			CacheWriteTokens:        80,
			CacheWritePriceUSDPer1M: 5.0,
			ReasoningTokens:         300,
			ReasoningPriceUSDPer1M:  6.0,
			EmbeddingTokens:         400,
			EmbeddingPriceUSDPer1M:  0.5,
		}
		ApplyTokenUsageCosts(e)

		// OpenAI/DeepSeek semantics: InputTokens includes the cached portion,
		// so full-price billable input = 100 - 50 = 50.
		wantInput := int64(math.Round(50 * 2.0))
		wantOutput := int64(math.Round(200 * 4.0))
		wantCached := int64(math.Round(50 * 1.0))
		wantCacheWrite := int64(math.Round(80 * 5.0))
		wantReasoning := int64(math.Round(300 * 6.0))
		wantEmbedding := int64(math.Round(400 * 0.5))

		if e.InputCostMicroUSD != wantInput {
			t.Errorf("InputCostMicroUSD = %d, want %d", e.InputCostMicroUSD, wantInput)
		}
		if e.OutputCostMicroUSD != wantOutput {
			t.Errorf("OutputCostMicroUSD = %d, want %d", e.OutputCostMicroUSD, wantOutput)
		}
		if e.CachedInputCostMicroUSD != wantCached {
			t.Errorf("CachedInputCostMicroUSD = %d, want %d", e.CachedInputCostMicroUSD, wantCached)
		}
		if e.CacheWriteCostMicroUSD != wantCacheWrite {
			t.Errorf("CacheWriteCostMicroUSD = %d, want %d", e.CacheWriteCostMicroUSD, wantCacheWrite)
		}
		if e.ReasoningCostMicroUSD != wantReasoning {
			t.Errorf("ReasoningCostMicroUSD = %d, want %d", e.ReasoningCostMicroUSD, wantReasoning)
		}
		if e.EmbeddingCostMicroUSD != wantEmbedding {
			t.Errorf("EmbeddingCostMicroUSD = %d, want %d", e.EmbeddingCostMicroUSD, wantEmbedding)
		}

		wantTotal := wantInput + wantOutput + wantCached + wantCacheWrite + wantReasoning + wantEmbedding
		if e.TotalCostMicroUSD != wantTotal {
			t.Errorf("TotalCostMicroUSD = %d, want %d", e.TotalCostMicroUSD, wantTotal)
		}
	})

	t.Run("deepseek_realworld_cached_billing", func(t *testing.T) {
		// Mirrors prod row: in=41783 out=2339, official hit rate ~70%.
		e := &TokenUsageEvent{
			InputTokens:            41783,
			OutputTokens:           2339,
			CachedInputTokens:      29248,
			InputPriceUSDPer1M:     0.14,
			OutputPriceUSDPer1M:    0.28,
			CacheReadPriceUSDPer1M: 0.014,
		}
		ApplyTokenUsageCosts(e)
		wantInput := int64(math.Round(float64(41783-29248) * 0.14))
		wantCached := int64(math.Round(float64(29248) * 0.014))
		wantOutput := int64(math.Round(float64(2339) * 0.28))
		if e.InputCostMicroUSD != wantInput {
			t.Errorf("InputCostMicroUSD = %d, want %d", e.InputCostMicroUSD, wantInput)
		}
		if e.CachedInputCostMicroUSD != wantCached {
			t.Errorf("CachedInputCostMicroUSD = %d, want %d", e.CachedInputCostMicroUSD, wantCached)
		}
		if e.OutputCostMicroUSD != wantOutput {
			t.Errorf("OutputCostMicroUSD = %d, want %d", e.OutputCostMicroUSD, wantOutput)
		}
		if e.TotalCostMicroUSD != wantInput+wantCached+wantOutput {
			t.Errorf("TotalCostMicroUSD = %d, want %d", e.TotalCostMicroUSD, wantInput+wantCached+wantOutput)
		}
	})

	t.Run("cached_clamped_to_input", func(t *testing.T) {
		// Corrupt data guard: cached > input must not produce negative billable input.
		e := &TokenUsageEvent{
			InputTokens:            100,
			CachedInputTokens:      150,
			InputPriceUSDPer1M:     2.0,
			CacheReadPriceUSDPer1M: 1.0,
		}
		ApplyTokenUsageCosts(e)
		if e.InputCostMicroUSD != 0 {
			t.Errorf("InputCostMicroUSD = %d, want 0 (cached exceeds input)", e.InputCostMicroUSD)
		}
		wantCached := int64(math.Round(100 * 1.0))
		if e.CachedInputCostMicroUSD != wantCached {
			t.Errorf("CachedInputCostMicroUSD = %d, want %d (cached clamped to input)", e.CachedInputCostMicroUSD, wantCached)
		}
	})

	t.Run("existing_cost_not_overwritten", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:        1000,
			InputPriceUSDPer1M: 3.0,
			InputCostMicroUSD:  42,
		}
		ApplyTokenUsageCosts(e)
		if e.InputCostMicroUSD != 42 {
			t.Errorf("InputCostMicroUSD = %d, want 42 (should not overwrite existing)", e.InputCostMicroUSD)
		}
	})

	t.Run("no_price_no_cost", func(t *testing.T) {
		e := &TokenUsageEvent{
			InputTokens:  1000,
			OutputTokens: 500,
		}
		ApplyTokenUsageCosts(e)
		if e.InputCostMicroUSD != 0 {
			t.Errorf("InputCostMicroUSD = %d, want 0 with no price", e.InputCostMicroUSD)
		}
		if e.OutputCostMicroUSD != 0 {
			t.Errorf("OutputCostMicroUSD = %d, want 0 with no price", e.OutputCostMicroUSD)
		}
	})
}

func TestMapRepoErr(t *testing.T) {
	tests := []struct {
		name        string
		input       error
		wantNil     bool
		wantReason  string
		wantMessage string
		wantCode    apierror.Code
		wantAPIErr  bool
	}{
		{"nil_returns_nil", nil, true, "", "", "", false},
		{
			"usage_scope_required",
			shared.ErrUsageScopeRequired,
			false,
			"USAGE",
			"scope_type and scope_id are required",
			apierror.CodeBadRequest,
			true,
		},
		{
			"budget_alert_not_found",
			shared.ErrBudgetAlertNotFound,
			false,
			"USAGE_ALERT",
			"budget alert not found",
			apierror.CodeNotFound,
			true,
		},
		{
			"unknown_error_passthrough",
			errors.New("something broke"),
			false,
			"",
			"something broke",
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapRepoErr(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("MapRepoErr(%v) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("MapRepoErr(%v) = nil, want non-nil", tt.input)
			}
			if se, ok := apierror.From(got); ok && tt.wantAPIErr {
				if se.Domain != tt.wantReason {
					t.Errorf("domain = %q, want %q", se.Domain, tt.wantReason)
				}
				if se.Message != tt.wantMessage {
					t.Errorf("message = %q, want %q", se.Message, tt.wantMessage)
				}
				if se.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", se.Code, tt.wantCode)
				}
			} else if got.Error() != tt.wantMessage {
				t.Errorf("error message = %q, want %q", got.Error(), tt.wantMessage)
			}
		})
	}

	t.Run("wrapped_usage_scope_required", func(t *testing.T) {
		wrapped := fmt.Errorf("wrap: %w", shared.ErrUsageScopeRequired)
		got := MapRepoErr(wrapped)
		if got == nil {
			t.Fatal("MapRepoErr(wrapped) = nil, want non-nil")
		}
		se, ok := apierror.From(got)
		if !ok {
			t.Fatalf("expected apierror, got %T", got)
		}
		if se.Domain != "USAGE" {
			t.Errorf("domain = %q, want %q", se.Domain, "USAGE")
		}
		if se.Code != apierror.CodeBadRequest {
			t.Errorf("code = %s, want %s", se.Code, apierror.CodeBadRequest)
		}
	})
}
