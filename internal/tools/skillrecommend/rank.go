package skillrecommend

import (
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
