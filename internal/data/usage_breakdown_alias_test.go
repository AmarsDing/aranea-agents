package data

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestMergeUsageBreakdownByAlias(t *testing.T) {
	rows := []biz.UsageBreakdownRow{
		{ProviderCode: "aliyun-qwen", ModelAPIID: "qwen-max", CallCount: 2, TotalCostMicroUSD: 100},
		{ProviderCode: "alibaba-cn", ModelAPIID: "qwen-max", CallCount: 3, TotalCostMicroUSD: 200},
	}
	out := mergeUsageBreakdownByAlias(rows)
	if len(out) != 1 {
		t.Fatalf("expected merge to 1 row, got %d", len(out))
	}
	if out[0].ProviderCode != "alibaba-cn" || out[0].CallCount != 5 || out[0].TotalCostMicroUSD != 300 {
		t.Fatalf("unexpected merge: %+v", out[0])
	}
}

func TestUsageProviderWhere_expandsLegacy(t *testing.T) {
	clause, args := usageProviderWhere("alibaba-cn")
	if clause == "" || len(args) < 2 {
		t.Fatalf("expected IN clause with legacy codes, got %q %v", clause, args)
	}
}

func TestAliasUsageEvent(t *testing.T) {
	ev := aliasUsageEvent(biz.TokenUsageEvent{ProviderCode: "gemini", ModelAPIID: "gemini-1.5"})
	if ev.ProviderCode != "google" {
		t.Fatalf("expected google alias, got %q", ev.ProviderCode)
	}
	ev2 := aliasUsageEvent(biz.TokenUsageEvent{ProviderCode: "gemini", CanonicalProviderCode: "google", ModelAPIID: "gemini-1.5"})
	if ev2.ProviderCode != "google" {
		t.Fatalf("expected canonical column, got %q", ev2.ProviderCode)
	}
}
