package memory

import (
	"math"
	"time"
)

// EbbinghausDecayCalculator implements R_t = exp(-n_t / S_t) decay scoring.
// Based on OBLIVION (2026) paper: forgetting is modeled as reachability decay,
// where reachability R_t decays exponentially with time since last access,
// modulated by memory stability S_t (which grows with creation age and access frequency).
type EbbinghausDecayCalculator struct{}

// NewEbbinghausDecayCalculator creates a new EbbinghausDecayCalculator.
func NewEbbinghausDecayCalculator() *EbbinghausDecayCalculator {
	return &EbbinghausDecayCalculator{}
}

// DecayInput carries the fields needed to compute R_t.
type DecayInput struct {
	LastUsedAt  time.Time // last access time (zero = use CreatedAt)
	CreatedAt   time.Time // memory creation time
	AccessCount int       // use_count (frequency factor)
	Now         time.Time // reference time (usually time.Now())
}

// ComputeDecay returns R_t = exp(-n_t / S_t) where:
//
//	n_t = hours since last access
//	S_t = stability = creationAgeHours + accessCount*24 + 0.001*creationAgeHours
//
// Returns 1.0 when no decay applies (just created, just accessed, or missing timestamps).
// Returns a value in [0, 1] — higher means more reachable (less forgotten).
func (c *EbbinghausDecayCalculator) ComputeDecay(in DecayInput) float64 {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	lastUsed := in.LastUsedAt
	if lastUsed.IsZero() {
		lastUsed = in.CreatedAt
	}
	if lastUsed.IsZero() {
		return 1.0 // no timestamps = no decay
	}

	nT := now.Sub(lastUsed).Hours()
	if nT <= 0 {
		return 1.0 // future or just now = full reachability
	}

	creationAgeHours := 0.0
	if !in.CreatedAt.IsZero() {
		creationAgeHours = now.Sub(in.CreatedAt).Hours()
		if creationAgeHours < 0 {
			creationAgeHours = 0
		}
	}

	// S_t = U_t + F_t + ε·T
	//   U_t = creation age (stability grows with age)
	//   F_t = access frequency factor (each access adds 24h of stability)
	//   ε·T = small epsilon to prevent division by zero
	sT := creationAgeHours + float64(in.AccessCount)*24 + 0.001*creationAgeHours
	if sT <= 0 {
		sT = 1.0 // safety guard against division by zero
	}

	return math.Exp(-nT / sT)
}

// FuseWithScore combines an original relevance score with the Ebbinghaus decay factor.
// decayWeight in [0, 1] controls how much decay influences the final score.
//
//	finalScore = originalScore * (1 - decayWeight * (1 - decay))
//
// When decay=1.0 (no forgetting), finalScore = originalScore.
// When decay=0.0 (fully forgotten), finalScore = originalScore * (1 - decayWeight).
// Inputs outside [0,1] are clamped; decayWeight <= 0 returns the original score unchanged.
func (c *EbbinghausDecayCalculator) FuseWithScore(originalScore, decay, decayWeight float64) float64 {
	if decayWeight <= 0 {
		return originalScore
	}
	if decayWeight > 1 {
		decayWeight = 1
	}
	// Clamp decay to [0, 1].
	if decay < 0 {
		decay = 0
	}
	if decay > 1 {
		decay = 1
	}
	return originalScore * (1 - decayWeight*(1-decay))
}
