package memory

import (
	"math"
	"testing"
	"time"
)

// TestComputeDecay_JustCreated verifies a freshly created memory has R_t = 1.0.
// When LastUsedAt is zero it falls back to CreatedAt; with CreatedAt == Now,
// n_t = 0 and the calculator returns full reachability.
func TestComputeDecay_JustCreated(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	now := time.Now()
	got := c.ComputeDecay(DecayInput{
		LastUsedAt:  time.Time{}, // no last used -> falls back to CreatedAt
		CreatedAt:   now,         // created exactly now -> n_t = 0
		AccessCount: 0,
		Now:         now,
	})
	if got != 1.0 {
		t.Fatalf("expected R_t = 1.0 for just-created memory (n_t=0), got %v", got)
	}
}

// TestComputeDecay_JustAccessed verifies a memory accessed right now has R_t = 1.0.
func TestComputeDecay_JustAccessed(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	now := time.Now()
	got := c.ComputeDecay(DecayInput{
		LastUsedAt:  now,
		CreatedAt:   now.Add(-24 * time.Hour),
		AccessCount: 5,
		Now:         now,
	})
	if got != 1.0 {
		t.Fatalf("expected R_t = 1.0 for just-accessed memory, got %v", got)
	}
}

// TestComputeDecay_OldNeverAccessed verifies an old, never-accessed memory has R_t < 1.0.
func TestComputeDecay_OldNeverAccessed(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	now := time.Now()
	created := now.Add(-30 * 24 * time.Hour) // 30 days old, never accessed
	got := c.ComputeDecay(DecayInput{
		LastUsedAt:  time.Time{}, // fall back to CreatedAt
		CreatedAt:   created,
		AccessCount: 0,
		Now:         now,
	})
	if got <= 0 || got >= 1.0 {
		t.Fatalf("expected 0 < R_t < 1.0 for old never-accessed memory, got %v", got)
	}
	// 30 days = 720h, S_t = 720 + 0 + 0.001*720 = 720.72
	// R_t = exp(-720/720.72) ≈ exp(-0.999) ≈ 0.368
	if got < 0.3 || got > 0.45 {
		t.Fatalf("expected R_t ≈ 0.368 for 30-day-old never-accessed memory, got %v", got)
	}
}

// TestComputeDecay_HighAccessCount verifies high-frequency access slows decay
// (higher AccessCount -> larger S_t -> higher R_t for the same elapsed time).
func TestComputeDecay_HighAccessCount(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	now := time.Now()
	lastUsed := now.Add(-48 * time.Hour) // 2 days since last access
	created := now.Add(-100 * 24 * time.Hour)

	low := c.ComputeDecay(DecayInput{
		LastUsedAt:  lastUsed,
		CreatedAt:   created,
		AccessCount: 1,
		Now:         now,
	})
	high := c.ComputeDecay(DecayInput{
		LastUsedAt:  lastUsed,
		CreatedAt:   created,
		AccessCount: 50,
		Now:         now,
	})
	if high <= low {
		t.Fatalf("expected high-access memory to decay slower (higher R_t): low=%v high=%v", low, high)
	}
	if low <= 0 || low >= 1.0 {
		t.Fatalf("low-access R_t out of (0,1): %v", low)
	}
	if high <= 0 || high >= 1.0 {
		t.Fatalf("high-access R_t out of (0,1): %v", high)
	}
}

// TestComputeDecay_ZeroTimestamps verifies no timestamps returns 1.0.
func TestComputeDecay_ZeroTimestamps(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	got := c.ComputeDecay(DecayInput{
		LastUsedAt:  time.Time{},
		CreatedAt:   time.Time{},
		AccessCount: 0,
		Now:         time.Now(),
	})
	if got != 1.0 {
		t.Fatalf("expected R_t = 1.0 for zero timestamps, got %v", got)
	}
}

// TestComputeDecay_FutureLastUsed verifies a future last-used time returns 1.0.
func TestComputeDecay_FutureLastUsed(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	now := time.Now()
	got := c.ComputeDecay(DecayInput{
		LastUsedAt:  now.Add(1 * time.Hour), // 1h in the future
		CreatedAt:   now.Add(-24 * time.Hour),
		AccessCount: 3,
		Now:         now,
	})
	if got != 1.0 {
		t.Fatalf("expected R_t = 1.0 for future last-used, got %v", got)
	}
}

// TestComputeDecay_NowZeroFallsBackToTimeNow verifies Now=zero falls back to time.Now().
func TestComputeDecay_NowZeroFallsBackToTimeNow(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	created := time.Now().Add(-1 * time.Hour)
	got := c.ComputeDecay(DecayInput{
		LastUsedAt:  time.Time{},
		CreatedAt:   created,
		AccessCount: 0,
		Now:         time.Time{}, // zero -> falls back to time.Now()
	})
	if got <= 0 || got >= 1.0 {
		t.Fatalf("expected 0 < R_t < 1.0 when Now falls back to time.Now(), got %v", got)
	}
}

// TestFuseWithScore_NoDecayWeight verifies decayWeight=0 returns the original score.
func TestFuseWithScore_NoDecayWeight(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	original := 0.85
	got := c.FuseWithScore(original, 0.0, 0.0)
	if got != original {
		t.Fatalf("expected original score %v when decayWeight=0, got %v", original, got)
	}
}

// TestFuseWithScore_FullDecay verifies decay=0 with decayWeight=0.5 halves the score.
func TestFuseWithScore_FullDecay(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	original := 0.80
	got := c.FuseWithScore(original, 0.0, 0.5)
	want := original * (1 - 0.5*(1-0.0)) // = 0.80 * 0.5 = 0.40
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %v for full decay with weight 0.5, got %v", want, got)
	}
}

// TestFuseWithScore_NoDecay verifies decay=1.0 returns the original score.
func TestFuseWithScore_NoDecay(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	original := 0.75
	got := c.FuseWithScore(original, 1.0, 0.5)
	if got != original {
		t.Fatalf("expected original score %v when decay=1.0, got %v", original, got)
	}
}

// TestFuseWithScore_Clamping verifies decay and decayWeight are clamped to [0,1].
func TestFuseWithScore_Clamping(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	original := 0.60

	// decayWeight > 1 clamped to 1; decay < 0 clamped to 0.
	// finalScore = original * (1 - 1*(1-0)) = 0
	got := c.FuseWithScore(original, -0.5, 2.0)
	if got != 0.0 {
		t.Fatalf("expected 0.0 for decay<0 and decayWeight>1 clamped, got %v", got)
	}

	// decay > 1 clamped to 1; decayWeight > 1 clamped to 1.
	// finalScore = original * (1 - 1*(1-1)) = original
	got = c.FuseWithScore(original, 1.5, 5.0)
	if got != original {
		t.Fatalf("expected original %v for decay>1 and decayWeight>1 clamped, got %v", original, got)
	}

	// decayWeight < 0 treated as 0 -> returns original.
	got = c.FuseWithScore(original, 0.0, -0.5)
	if got != original {
		t.Fatalf("expected original %v for decayWeight<0, got %v", original, got)
	}
}

// TestFuseWithScore_PartialDecay verifies the formula at an intermediate point.
func TestFuseWithScore_PartialDecay(t *testing.T) {
	c := NewEbbinghausDecayCalculator()
	original := 1.0
	decay := 0.5
	weight := 0.3
	// finalScore = 1.0 * (1 - 0.3*(1-0.5)) = 1 - 0.15 = 0.85
	got := c.FuseWithScore(original, decay, weight)
	want := 0.85
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %v for partial decay, got %v", want, got)
	}
}
