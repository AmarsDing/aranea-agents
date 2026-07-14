package biz

import (
	"math"
	"testing"
)

// TestComputeHotness_EmptyStats 空统计列表返回空 map。
func TestComputeHotness_EmptyStats(t *testing.T) {
	got := ComputeHotness(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

// TestComputeHotness_SingleModel 单个模型：调用/Token/费用标准分给满分（min=max 时给 50），成功率直接按值映射。
// 预期分数 = 50*0.50 + 50*0.25 + 50*0.15 + success_rate*100*0.10 = 50 + success_rate*10
func TestComputeHotness_SingleModel(t *testing.T) {
	stats := []ModelUsageStats{
		{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.9},
	}
	got := ComputeHotness(stats)
	score, ok := got["openai/gpt-4o"]
	if !ok {
		t.Fatalf("expected key openai/gpt-4o, got %v", got)
	}
	// 50*0.50 + 50*0.25 + 50*0.15 + 90*0.10 = 25 + 12.5 + 7.5 + 9 = 54
	want := 25.0 + 12.5 + 7.5 + 9.0
	if math.Abs(score-want) > 0.01 {
		t.Fatalf("expected %.2f, got %.2f", want, score)
	}
}

// TestComputeHotness_TwoModelsHigherCallsWins 相同 Token/费用/成功率，调用次数多的模型热度更高。
func TestComputeHotness_TwoModelsHigherCallsWins(t *testing.T) {
	stats := []ModelUsageStats{
		{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 10, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.9},
		{ProviderCode: "openai", ModelAPIID: "gpt-4o-mini", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.9},
	}
	got := ComputeHotness(stats)
	if got["openai/gpt-4o"] >= got["openai/gpt-4o-mini"] {
		t.Fatalf("expected mini (more calls) hotter, got mini=%.2f full=%.2f", got["openai/gpt-4o-mini"], got["openai/gpt-4o"])
	}
}

// TestComputeHotness_AllEqualValues 所有维度值相同的多个模型：标准分给 50（中等），热度由成功率决定。
func TestComputeHotness_AllEqualValues(t *testing.T) {
	stats := []ModelUsageStats{
		{ProviderCode: "openai", ModelAPIID: "a", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.8},
		{ProviderCode: "openai", ModelAPIID: "b", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.8},
	}
	got := ComputeHotness(stats)
	// min=max 时标准分给 50：50*0.50 + 50*0.25 + 50*0.15 + 80*0.10 = 25+12.5+7.5+8 = 53
	want := 25.0 + 12.5 + 7.5 + 8.0
	if math.Abs(got["openai/a"]-want) > 0.01 {
		t.Fatalf("a: expected %.2f, got %.2f", want, got["openai/a"])
	}
	if math.Abs(got["openai/b"]-want) > 0.01 {
		t.Fatalf("b: expected %.2f, got %.2f", want, got["openai/b"])
	}
}

// TestComputeHotness_SuccessRateImpact 相同调用/Token/费用，成功率高的模型热度更高。
func TestComputeHotness_SuccessRateImpact(t *testing.T) {
	stats := []ModelUsageStats{
		{ProviderCode: "openai", ModelAPIID: "low", CallCount: 50, TotalTokens: 500, TotalCostMicroUSD: 2500, SuccessRate: 0.5},
		{ProviderCode: "openai", ModelAPIID: "high", CallCount: 50, TotalTokens: 500, TotalCostMicroUSD: 2500, SuccessRate: 1.0},
	}
	got := ComputeHotness(stats)
	if got["openai/low"] >= got["openai/high"] {
		t.Fatalf("expected high success rate hotter, got low=%.2f high=%.2f", got["openai/low"], got["openai/high"])
	}
}

// TestComputeHotness_ScoreRangeInBounds 多模型场景下所有分数在 [0, 100] 范围内。
func TestComputeHotness_ScoreRangeInBounds(t *testing.T) {
	stats := []ModelUsageStats{
		{ProviderCode: "p1", ModelAPIID: "m1", CallCount: 1000, TotalTokens: 100000, TotalCostMicroUSD: 500000, SuccessRate: 1.0},
		{ProviderCode: "p2", ModelAPIID: "m2", CallCount: 1, TotalTokens: 10, TotalCostMicroUSD: 1, SuccessRate: 0.0},
		{ProviderCode: "p3", ModelAPIID: "m3", CallCount: 500, TotalTokens: 50000, TotalCostMicroUSD: 250000, SuccessRate: 0.7},
	}
	got := ComputeHotness(stats)
	for k, v := range got {
		if v < 0 || v > 100 {
			t.Fatalf("score for %s out of bounds: %.2f", k, v)
		}
	}
}

// TestComputeHotness_KeyFormat 验证 key 格式为 "provider/model"。
func TestComputeHotness_KeyFormat(t *testing.T) {
	stats := []ModelUsageStats{
		{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.9},
	}
	got := ComputeHotness(stats)
	if _, ok := got["openai/gpt-4o"]; !ok {
		t.Fatalf("expected key 'openai/gpt-4o', got keys: %v", got)
	}
}

// TestStandardizeMinMax 验证 Min-Max 标准化函数。
func TestStandardizeMinMax(t *testing.T) {
	// value=min → 0, value=max → 100, value=mid → 50
	if got := standardize(0, 0, 100); math.Abs(got-0) > 0.01 {
		t.Fatalf("min: expected 0, got %.2f", got)
	}
	if got := standardize(100, 0, 100); math.Abs(got-100) > 0.01 {
		t.Fatalf("max: expected 100, got %.2f", got)
	}
	if got := standardize(50, 0, 100); math.Abs(got-50) > 0.01 {
		t.Fatalf("mid: expected 50, got %.2f", got)
	}
	// min=max 时给 50（中等）
	if got := standardize(42, 42, 42); math.Abs(got-50) > 0.01 {
		t.Fatalf("equal: expected 50, got %.2f", got)
	}
}
