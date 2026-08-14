package memory_butler

import (
	"encoding/json"
	"testing"
	"time"
)

func TestComputeQualityMetrics_EmptyIsComputedZero(t *testing.T) {
	got := computeQualityMetrics(nil, time.Now(), 30)
	if got != (qualityMetrics{}) {
		t.Fatalf("empty facts: got %+v, want computed zeros", got)
	}
}

func TestComputeQualityMetrics_NoRedundancy(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	facts := []qualityFact{
		{ID: "a", Statement: "用户喜欢美式咖啡不加糖", LastUsedAt: now.AddDate(0, 0, -1).Format(time.RFC3339), CreatedAt: now.AddDate(0, 0, -10).Format(time.RFC3339)},
		{ID: "b", Statement: "部署窗口只在周二下午", LastUsedAt: now.AddDate(0, 0, -2).Format(time.RFC3339), CreatedAt: now.AddDate(0, 0, -10).Format(time.RFC3339)},
		{ID: "c", Statement: "告警升级给值班长而不是群机器人", LastUsedAt: now.Format(time.RFC3339), CreatedAt: now.AddDate(0, 0, -5).Format(time.RFC3339)},
	}
	got := computeQualityMetrics(facts, now, 30)
	if got.RedundancyScore != 0 {
		t.Fatalf("unique facts must yield computed redundancy 0, got %v", got.RedundancyScore)
	}
	if got.PredictableCount != 0 {
		t.Fatalf("unique facts must yield computed predictable 0, got %d", got.PredictableCount)
	}
	if got.InactiveCount != 0 {
		t.Fatalf("all recently used facts must yield computed inactive 0, got %d", got.InactiveCount)
	}
}

func TestComputeQualityMetrics_HasRedundancyAndPredictable(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -1).Format(time.RFC3339)
	facts := []qualityFact{
		{ID: "keep", Statement: "用户喜欢美式咖啡不加糖，温度要热", LastUsedAt: recent, CreatedAt: now.AddDate(0, 0, -20).Format(time.RFC3339)},
		{ID: "weak", Statement: "用户喜欢美式咖啡不加糖", LastUsedAt: recent, CreatedAt: now.AddDate(0, 0, -10).Format(time.RFC3339)},
		{ID: "other", Statement: "部署窗口只在周二下午", LastUsedAt: recent, CreatedAt: now.AddDate(0, 0, -3).Format(time.RFC3339)},
	}
	got := computeQualityMetrics(facts, now, 30)
	if got.RedundancyScore == 0 {
		t.Fatal("near-duplicate pair must not report hardcoded redundancy 0")
	}
	// 2 of 3 facts sit in the duplicate cluster → 0.667
	if got.RedundancyScore != 0.667 {
		t.Fatalf("redundancy_score = %v, want 0.667", got.RedundancyScore)
	}
	if got.PredictableCount != 1 {
		t.Fatalf("predictable_count = %d, want 1 (weaker shorter copy)", got.PredictableCount)
	}
	if got.InactiveCount != 0 {
		t.Fatalf("inactive_count = %d, want 0", got.InactiveCount)
	}
}

func TestComputeQualityMetrics_IdenticalFingerprintIsRedundant(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	recent := now.Format(time.RFC3339)
	facts := []qualityFact{
		{ID: "a", Statement: "alpha", Fingerprint: "fp-same", LastUsedAt: recent, CreatedAt: recent},
		{ID: "b", Statement: "beta-unrelated-text", Fingerprint: "fp-same", LastUsedAt: recent, CreatedAt: recent},
	}
	got := computeQualityMetrics(facts, now, 30)
	if got.RedundancyScore != 1 {
		t.Fatalf("same fingerprint must be redundancy 1, got %v", got.RedundancyScore)
	}
	if got.PredictableCount != 1 {
		t.Fatalf("one of two fingerprint twins must be predictable, got %d", got.PredictableCount)
	}
}

func TestComputeQualityMetrics_HasInactive(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	facts := []qualityFact{
		{ID: "stale", Statement: "很久没被检索的偏好", LastUsedAt: now.AddDate(0, 0, -45).Format(time.RFC3339), CreatedAt: now.AddDate(0, 0, -90).Format(time.RFC3339)},
		{ID: "fresh", Statement: "昨天刚召回的约束", LastUsedAt: now.AddDate(0, 0, -1).Format(time.RFC3339), CreatedAt: now.AddDate(0, 0, -90).Format(time.RFC3339)},
		{ID: "never-old", Statement: "写了两个月从未召回", CreatedAt: now.AddDate(0, 0, -60).Format(time.RFC3339)},
		{ID: "never-new", Statement: "今天刚写入尚未召回", CreatedAt: now.Format(time.RFC3339)},
	}
	got := computeQualityMetrics(facts, now, 30)
	if got.InactiveCount != 2 {
		t.Fatalf("inactive_count = %d, want 2 (stale last_used + never-recalled old)", got.InactiveCount)
	}
}

func TestComputeQualityMetrics_AllActive(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -3).Format(time.RFC3339)
	facts := []qualityFact{
		{ID: "a", Statement: "事实甲内容足够独特", LastUsedAt: recent, CreatedAt: now.AddDate(0, 0, -100).Format(time.RFC3339)},
		{ID: "b", Statement: "事实乙完全不同主题", LastUsedAt: now.Format(time.RFC3339), CreatedAt: now.AddDate(0, 0, -80).Format(time.RFC3339)},
	}
	got := computeQualityMetrics(facts, now, 30)
	if got.InactiveCount != 0 {
		t.Fatalf("all-active set must yield computed inactive 0, got %d", got.InactiveCount)
	}
}

func TestComputeQualityMetrics_MissingTimestampsCountInactive(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	got := computeQualityMetrics([]qualityFact{
		{ID: "no-time", Statement: "没有任何时间戳"},
	}, now, 30)
	if got.InactiveCount != 1 {
		t.Fatalf("unparseable timestamps must count as inactive, got %d", got.InactiveCount)
	}
	if got.RedundancyScore != 0 || got.PredictableCount != 0 {
		t.Fatalf("single fact must have computed redundancy/predictable 0, got %+v", got)
	}
}

func TestParseQualityFacts_SkipsBrokenRows(t *testing.T) {
	row, err := json.Marshal(map[string]any{
		"id": "f1", "statement": "hello", "fingerprint": "fp",
		"last_used_at": "2026-08-01T00:00:00Z", "created_at": "2026-07-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	facts := parseQualityFacts([][]byte{
		[]byte("not-json"),
		[]byte(`{"id":""}`),
		[]byte(`{"id":"x","statement":""}`),
		row,
	})
	if len(facts) != 1 || facts[0].ID != "f1" || facts[0].Fingerprint != "fp" {
		t.Fatalf("parseQualityFacts = %+v", facts)
	}
}

func TestStringSimilarity_IdenticalAndDistinct(t *testing.T) {
	if stringSimilarity("用户喜欢咖啡", "用户喜欢咖啡") != 1 {
		t.Fatal("identical statements must score 1")
	}
	if stringSimilarity("用户喜欢美式咖啡不加糖", "部署窗口只在周二下午") >= defaultSimilarityThreshold {
		t.Fatal("unrelated statements must stay below the redundancy threshold")
	}
	if stringSimilarity("用户喜欢美式咖啡不加糖", "用户喜欢美式咖啡不加糖，要热的") >= defaultSimilarityThreshold {
		t.Fatal("Jaccard of a tailed restatement stays below 0.8; overlap coefficient is what catches it")
	}
	if trigramOverlap("用户喜欢美式咖啡不加糖", "用户喜欢美式咖啡不加糖，要热的") < defaultSimilarityThreshold {
		t.Fatal("trigram overlap of a restatement must meet the redundancy threshold")
	}
	if factSimilarity(
		qualityFact{Statement: "用户喜欢美式咖啡不加糖"},
		qualityFact{Statement: "用户喜欢美式咖啡不加糖，要热的"},
	) < defaultSimilarityThreshold {
		t.Fatal("factSimilarity must treat a shorter restatement as near-duplicate")
	}
}
