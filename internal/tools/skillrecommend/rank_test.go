package skillrecommend

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRank_BasicSorting(t *testing.T) {
	factors := DefaultRankFactors()
	candidates := []Candidate{
		{Slug: "low", SemanticSimilarity: 0.2, HistoricalSuccess: 0.2, LatencyInverse: 0.2, UserPreference: 0.2},
		{Slug: "high", SemanticSimilarity: 0.9, HistoricalSuccess: 0.9, LatencyInverse: 0.9, UserPreference: 0.9},
		{Slug: "mid", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}

	results := Rank(candidates, factors)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Slug != "high" {
		t.Errorf("expected first slug 'high', got %q", results[0].Slug)
	}
	if results[1].Slug != "mid" {
		t.Errorf("expected second slug 'mid', got %q", results[1].Slug)
	}
	if results[2].Slug != "low" {
		t.Errorf("expected third slug 'low', got %q", results[2].Slug)
	}
	// Verify descending order
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted descending: [%d]=%.3f > [%d]=%.3f", i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestRank_MissingDataDefaults(t *testing.T) {
	factors := DefaultRankFactors()
	candidates := []Candidate{
		{Slug: "all-zero"},
	}

	results := Rank(candidates, factors)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// All factors default to 0.5, so score = 0.5*(W1+W2+W3+W4) = 0.5*1.0 = 0.5
	expected := 0.5 * (factors.W1 + factors.W2 + factors.W3 + factors.W4)
	if results[0].Score != expected {
		t.Errorf("expected score %.3f for all-zero candidate, got %.3f", expected, results[0].Score)
	}
	// Verify factor snapshot reflects neutral defaults
	for key, val := range results[0].FactorSnap {
		if key == "exploration_bonus" {
			continue
		}
		if val != 0.5 {
			t.Errorf("expected factor %q to default to 0.5, got %.2f", key, val)
		}
	}
}

func TestRank_ExplorationBonus(t *testing.T) {
	factors := DefaultRankFactors()

	recent := time.Now().UTC().Add(-3 * 24 * time.Hour) // 3 days ago
	old := time.Now().UTC().Add(-30 * 24 * time.Hour)   // 30 days ago

	candidates := []Candidate{
		{Slug: "new-skill", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5, CreatedAt: recent},
		{Slug: "old-skill", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5, CreatedAt: old},
	}

	results := Rank(candidates, factors)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Find results by slug
	var newResult, oldResult RankResult
	for _, r := range results {
		if r.Slug == "new-skill" {
			newResult = r
		}
		if r.Slug == "old-skill" {
			oldResult = r
		}
	}

	// New skill should have exploration bonus
	if newResult.FactorSnap["exploration_bonus"] != ExplorationBonus {
		t.Errorf("expected exploration_bonus %.2f for new skill, got %.2f", ExplorationBonus, newResult.FactorSnap["exploration_bonus"])
	}
	// Old skill should NOT have exploration bonus
	if oldResult.FactorSnap["exploration_bonus"] != 0 {
		t.Errorf("expected exploration_bonus 0 for old skill, got %.2f", oldResult.FactorSnap["exploration_bonus"])
	}
	// New skill score should be higher by ExplorationBonus
	if newResult.Score != oldResult.Score+ExplorationBonus {
		t.Errorf("expected new skill score (%.3f) = old skill score (%.3f) + %.1f", newResult.Score, oldResult.Score, ExplorationBonus)
	}
}

func TestRank_WeightFactors(t *testing.T) {
	factors := DefaultRankFactors()

	// Test: higher semantic similarity → higher score
	candidates := []Candidate{
		{Slug: "high-sem", SemanticSimilarity: 0.9, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
		{Slug: "low-sem", SemanticSimilarity: 0.1, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}
	results := Rank(candidates, factors)
	if results[0].Slug != "high-sem" {
		t.Errorf("expected 'high-sem' ranked first, got %q", results[0].Slug)
	}

	// Test: higher historical success → higher score
	candidates = []Candidate{
		{Slug: "high-success", SemanticSimilarity: 0.5, HistoricalSuccess: 0.9, LatencyInverse: 0.5, UserPreference: 0.5},
		{Slug: "low-success", SemanticSimilarity: 0.5, HistoricalSuccess: 0.1, LatencyInverse: 0.5, UserPreference: 0.5},
	}
	results = Rank(candidates, factors)
	if results[0].Slug != "high-success" {
		t.Errorf("expected 'high-success' ranked first, got %q", results[0].Slug)
	}

	// Test: higher latency inverse (faster) → higher score
	candidates = []Candidate{
		{Slug: "fast", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.9, UserPreference: 0.5},
		{Slug: "slow", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.1, UserPreference: 0.5},
	}
	results = Rank(candidates, factors)
	if results[0].Slug != "fast" {
		t.Errorf("expected 'fast' ranked first, got %q", results[0].Slug)
	}
}

func TestRank_Stability(t *testing.T) {
	factors := DefaultRankFactors()
	// All candidates with identical scores → sorted by slug alphabetically
	candidates := []Candidate{
		{Slug: "charlie", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
		{Slug: "alpha", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
		{Slug: "bravo", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}

	results := Rank(candidates, factors)

	if results[0].Slug != "alpha" {
		t.Errorf("expected first slug 'alpha', got %q", results[0].Slug)
	}
	if results[1].Slug != "bravo" {
		t.Errorf("expected second slug 'bravo', got %q", results[1].Slug)
	}
	if results[2].Slug != "charlie" {
		t.Errorf("expected third slug 'charlie', got %q", results[2].Slug)
	}
}

func TestFormatSelectionReason(t *testing.T) {
	r := RankResult{
		Slug:  "test-skill",
		Score: 0.756,
		FactorSnap: map[string]float64{
			"semantic_similarity": 0.80,
			"historical_success":  0.70,
			"latency_inverse":     0.60,
			"user_preference":     0.50,
			"exploration_bonus":   0.10,
		},
	}

	reason := FormatSelectionReason(r)

	// Verify the format string contains expected fields
	expectedSubstrings := []string{
		"rank_score=",
		"sem=",
		"success=",
		"latency=",
		"pref=",
		"explore=",
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(reason, sub) {
			t.Errorf("expected reason to contain %q, got %q", sub, reason)
		}
	}

	// Verify actual values are present
	if !strings.Contains(reason, "0.756") {
		t.Errorf("expected reason to contain score '0.756', got %q", reason)
	}
}

// ── DynamicRankFactors Tests (TDD Red Phase) ───────────────────────────────────

type mockHealthProvider struct {
	successRates map[string]float64
	avgDurations map[string]float64
	err          error
}

func (m *mockHealthProvider) GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if rate, ok := m.successRates[skillID]; ok {
		return rate, nil
	}
	return 0, nil // no data
}

func (m *mockHealthProvider) GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if dur, ok := m.avgDurations[skillID]; ok {
		return dur, nil
	}
	return 0, nil // no data
}

func TestDynamicRankFactors_HighSuccessRate(t *testing.T) {
	// High success rate (>80%) should reduce W2 (historical success) and boost W1 (semantic).
	provider := &mockHealthProvider{
		successRates: map[string]float64{
			"high-success-skill": 0.85,
		},
	}
	candidates := []Candidate{
		{Slug: "high-success-skill", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}

	factors := DynamicRankFactors(context.Background(), provider, candidates)
	defaults := DefaultRankFactors()

	// With high success rate, W2 should decrease and W1 should increase.
	if factors.W2 >= defaults.W2 {
		t.Errorf("expected W2 to decrease for high-success skills, got W2=%.3f (default=%.3f)", factors.W2, defaults.W2)
	}
	if factors.W1 <= defaults.W1 {
		t.Errorf("expected W1 to increase for high-success skills, got W1=%.3f (default=%.3f)", factors.W1, defaults.W1)
	}
}

func TestDynamicRankFactors_LowSuccessRate(t *testing.T) {
	// Low success rate (<40%) should reduce W2 (historical success) and boost W1 (semantic).
	provider := &mockHealthProvider{
		successRates: map[string]float64{
			"low-success-skill": 0.25,
		},
	}
	candidates := []Candidate{
		{Slug: "low-success-skill", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}

	factors := DynamicRankFactors(context.Background(), provider, candidates)
	defaults := DefaultRankFactors()

	// With low success rate, W2 should decrease and W1 should increase.
	if factors.W2 >= defaults.W2 {
		t.Errorf("expected W2 to decrease for low-success skills, got W2=%.3f (default=%.3f)", factors.W2, defaults.W2)
	}
	if factors.W1 <= defaults.W1 {
		t.Errorf("expected W1 to increase for low-success skills, got W1=%.3f (default=%.3f)", factors.W1, defaults.W1)
	}
}

func TestDynamicRankFactors_NoData(t *testing.T) {
	// No health data → use default static RankFactors.
	provider := &mockHealthProvider{
		successRates: map[string]float64{}, // empty
	}
	candidates := []Candidate{
		{Slug: "no-data-skill", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}

	factors := DynamicRankFactors(context.Background(), provider, candidates)
	defaultFactors := DefaultRankFactors()

	// Without data, factors should equal default.
	if factors != defaultFactors {
		t.Errorf("expected default factors when no data, got %+v", factors)
	}
}

func TestDynamicRankFactors_NilProvider(t *testing.T) {
	// Nil provider → use default static RankFactors.
	candidates := []Candidate{
		{Slug: "test-skill", SemanticSimilarity: 0.5, HistoricalSuccess: 0.5, LatencyInverse: 0.5, UserPreference: 0.5},
	}

	factors := DynamicRankFactors(context.Background(), nil, candidates)
	defaultFactors := DefaultRankFactors()

	if factors != defaultFactors {
		t.Errorf("expected default factors when provider is nil, got %+v", factors)
	}
}
