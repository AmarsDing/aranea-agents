package skillrecommend

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// Candidate represents a skill candidate for ranking.
type Candidate struct {
	Slug string

	// Factors used for ranking (0-1 range, missing data → 0.5).
	SemanticSimilarity float64 // embedding or taxonomy match score
	HistoricalSuccess  float64 // 30d success rate
	LatencyInverse     float64 // 1 / normalized_duration (higher = faster)
	UserPreference     float64 // user affinity score

	// Metadata for exploration bonus.
	CreatedAt time.Time // skill creation time for new-skill bonus
}

// RankFactors holds the weights for each ranking factor.
type RankFactors struct {
	W1 float64 // semantic similarity weight
	W2 float64 // historical success rate weight
	W3 float64 // latency inverse weight
	W4 float64 // user preference weight
}

// DefaultRankFactors returns the v1 default ranking weights.
func DefaultRankFactors() RankFactors {
	return RankFactors{
		W1: 0.3,
		W2: 0.35,
		W3: 0.2,
		W4: 0.15,
	}
}

// ExplorationBonus is the score bonus for new skills (< 7d old).
const ExplorationBonus = 0.1

// NewSkillAgeThreshold is the age below which a skill gets an exploration bonus.
const NewSkillAgeThreshold = 7 * 24 * time.Hour

// RankResult holds the ranking result for a single candidate.
type RankResult struct {
	Slug       string
	Score      float64
	FactorSnap map[string]float64
}

// Rank reorders candidates by the weighted formula:
//
//	score = w1 * semantic_sim + w2 * success_rate + w3 * latency_inv + w4 * user_pref
//
// Missing data defaults to neutral value 0.5. New skills (< 7d) get an
// exploration bonus of +0.1 to prevent the "rich get richer" effect.
func Rank(candidates []Candidate, factors RankFactors) []RankResult {
	now := time.Now().UTC()
	results := make([]RankResult, 0, len(candidates))

	for _, c := range candidates {
		// Default missing data to 0.5.
		sem := neutralIfZero(c.SemanticSimilarity)
		sr := neutralIfZero(c.HistoricalSuccess)
		lat := neutralIfZero(c.LatencyInverse)
		up := neutralIfZero(c.UserPreference)

		score := factors.W1*sem + factors.W2*sr + factors.W3*lat + factors.W4*up

		// Exploration bonus for new skills.
		if !c.CreatedAt.IsZero() && now.Sub(c.CreatedAt) < NewSkillAgeThreshold {
			score += ExplorationBonus
		}

		// Clamp to [0, 1].
		score = math.Max(0, math.Min(1, score))

		snap := map[string]float64{
			"semantic_similarity": sem,
			"historical_success":  sr,
			"latency_inverse":     lat,
			"user_preference":     up,
			"exploration_bonus":   0,
		}
		if !c.CreatedAt.IsZero() && now.Sub(c.CreatedAt) < NewSkillAgeThreshold {
			snap["exploration_bonus"] = ExplorationBonus
		}

		results = append(results, RankResult{
			Slug:       c.Slug,
			Score:      score,
			FactorSnap: snap,
		})
	}

	// Sort by score descending, then by slug for stability.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Slug < results[j].Slug
	})

	return results
}

// FormatSelectionReason formats the ranking factor snapshot as a human-readable
// string suitable for writing to selection_reason.
func FormatSelectionReason(r RankResult) string {
	return fmt.Sprintf("rank_score=%.3f sem=%.2f success=%.2f latency=%.2f pref=%.2f explore=%.2f",
		r.Score,
		r.FactorSnap["semantic_similarity"],
		r.FactorSnap["historical_success"],
		r.FactorSnap["latency_inverse"],
		r.FactorSnap["user_preference"],
		r.FactorSnap["exploration_bonus"],
	)
}

func neutralIfZero(v float64) float64 {
	if v <= 0 {
		return 0.5
	}
	return v
}

// Dynamic weight adjustment thresholds.
const (
	HighSuccessThreshold = 0.8  // success rate above this → reduce exploration
	LowSuccessThreshold  = 0.4  // success rate below this → reduce historical weight
	WeightAdjustStep     = 0.05 // amount to shift weights per adjustment
	DynamicLookbackDays  = 30   // default lookback window for health metrics
)

// DynamicRankFactors computes adjusted RankFactors based on recent health
// metrics from the provider. If the provider is nil or no data is available
// for any candidate, it falls back to DefaultRankFactors.
//
// Adjustment rules:
//   - High success rate (>80%): reduce exploration bonus by shifting weight
//     from W2 (historical success) toward W1 (semantic similarity), since
//     the skill is already proven and doesn't need extra help.
//   - Low success rate (<40%): reduce W2 (historical success) weight and
//     redistribute to W1 (semantic similarity), preventing "rich get richer"
//     for failing skills while giving semantic relevance more say.
//   - No data or provider nil: return default static factors.
func DynamicRankFactors(ctx context.Context, provider HealthMetricsProvider, candidates []Candidate) RankFactors {
	if provider == nil || len(candidates) == 0 {
		return DefaultRankFactors()
	}

	factors := DefaultRankFactors()

	hasData := false
	highSuccessCount := 0
	lowSuccessCount := 0

	for _, c := range candidates {
		rate, err := provider.GetRecentSuccessRate(ctx, c.Slug, DynamicLookbackDays)
		if err != nil || rate <= 0 {
			continue
		}
		hasData = true
		if rate > HighSuccessThreshold {
			highSuccessCount++
		} else if rate < LowSuccessThreshold {
			lowSuccessCount++
		}
	}

	if !hasData {
		return factors
	}

	total := len(candidates)
	// Proportional adjustment: the more candidates in a bucket, the stronger the shift.
	highRatio := float64(highSuccessCount) / float64(total)
	lowRatio := float64(lowSuccessCount) / float64(total)

	// High success: reduce W2, boost W1 (skill proven, rely more on semantic match).
	if highSuccessCount > 0 {
		shift := WeightAdjustStep * highRatio
		factors.W2 = math.Max(0.1, factors.W2-shift)
		factors.W1 = math.Min(0.6, factors.W1+shift)
	}

	// Low success: reduce W2 further, boost W1 (don't let bad history dominate).
	if lowSuccessCount > 0 {
		shift := WeightAdjustStep * lowRatio
		factors.W2 = math.Max(0.1, factors.W2-shift)
		factors.W1 = math.Min(0.6, factors.W1+shift)
	}

	return factors
}
