package memory_butler

import (
	"math"
	"strings"
	"time"

	"aranea-agents/pkg/jsonutil"
)

// qualityFact is the L3 row subset needed to compute butler quality metrics.
type qualityFact struct {
	ID          string
	Statement   string
	Fingerprint string
	LastUsedAt  string
	CreatedAt   string
}

type qualityMetrics struct {
	RedundancyScore  float64
	InactiveCount    int
	PredictableCount int
}

func parseQualityFacts(rows [][]byte) []qualityFact {
	out := make([]qualityFact, 0, len(rows))
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		id := jsonutil.IfaceStr(m, "id")
		stmt := jsonutil.IfaceStr(m, "statement")
		if id == "" || stmt == "" {
			continue
		}
		out = append(out, qualityFact{
			ID:          id,
			Statement:   stmt,
			Fingerprint: jsonutil.IfaceStr(m, "fingerprint"),
			LastUsedAt:  jsonutil.IfaceStr(m, "last_used_at"),
			CreatedAt:   jsonutil.IfaceStr(m, "created_at"),
		})
	}
	return out
}

// computeQualityMetrics derives redundancy / inactive / predictable from L3
// fact rows. A returned 0 means the formula produced 0 (no near-duplicates,
// no stale facts, no weaker copies) — it is never a hardcoded placeholder.
//
//	RedundancyScore  = |facts with ≥1 near-duplicate| / n   (trigram Jaccard
//	                   or overlap coefficient ≥ defaultSimilarityThreshold,
//	                   or identical fingerprint)
//	InactiveCount    = facts whose last retrieval (last_used_at) or, if never
//	                   recalled, created_at is older than inactiveDays.
//	                   Unparseable timestamps count as inactive.
//	PredictableCount = weaker member of each near-duplicate pair (shorter
//	                   statement; equal length → older created_at).
func computeQualityMetrics(facts []qualityFact, now time.Time, inactiveDays int) qualityMetrics {
	n := len(facts)
	if n == 0 {
		return qualityMetrics{}
	}
	if inactiveDays <= 0 {
		inactiveDays = defaultInactiveThresholdDays
	}
	cutoff := now.UTC().AddDate(0, 0, -inactiveDays)

	inactive := 0
	for i := range facts {
		if isInactiveFact(facts[i], cutoff) {
			inactive++
		}
	}

	if n == 1 {
		return qualityMetrics{InactiveCount: inactive}
	}

	redundant := make([]bool, n)
	predictable := make([]bool, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if factSimilarity(facts[i], facts[j]) < defaultSimilarityThreshold {
				continue
			}
			redundant[i] = true
			redundant[j] = true
			if weakerQualityFact(facts[i], facts[j]) {
				predictable[i] = true
			} else {
				predictable[j] = true
			}
		}
	}

	redCount := 0
	predCount := 0
	for i := 0; i < n; i++ {
		if redundant[i] {
			redCount++
		}
		if predictable[i] {
			predCount++
		}
	}
	score := math.Round((float64(redCount)/float64(n))*1000) / 1000
	return qualityMetrics{
		RedundancyScore:  score,
		InactiveCount:    inactive,
		PredictableCount: predCount,
	}
}

func factSimilarity(a, b qualityFact) float64 {
	if a.Fingerprint != "" && a.Fingerprint == b.Fingerprint {
		return 1.0
	}
	jaccard := stringSimilarity(a.Statement, b.Statement)
	overlap := trigramOverlap(a.Statement, b.Statement)
	if overlap > jaccard {
		return overlap
	}
	return jaccard
}

// trigramOverlap is the Simpson coefficient of rune trigrams:
// |A∩B| / min(|A|,|B|). A shorter restatement that is a subset of a longer
// fact scores ~1 even when Jaccard stays below defaultSimilarityThreshold.
func trigramOverlap(a, b string) float64 {
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)
	if aLower == "" || bLower == "" {
		return 0
	}
	if aLower == bLower {
		return 1
	}
	aSet := trigramSet(aLower)
	bSet := trigramSet(bLower)
	if len(aSet) == 0 || len(bSet) == 0 {
		return 0
	}
	shared := 0
	for k := range aSet {
		if bSet[k] {
			shared++
		}
	}
	denom := len(aSet)
	if len(bSet) < denom {
		denom = len(bSet)
	}
	return float64(shared) / float64(denom)
}

// weakerQualityFact reports whether a is the weaker copy of a near-duplicate
// pair (the one that can be predicted from b).
func weakerQualityFact(a, b qualityFact) bool {
	aLen := len([]rune(a.Statement))
	bLen := len([]rune(b.Statement))
	if aLen != bLen {
		return aLen < bLen
	}
	aTime, aOK := parseFactTime(a.CreatedAt)
	bTime, bOK := parseFactTime(b.CreatedAt)
	if aOK && bOK && !aTime.Equal(bTime) {
		return aTime.Before(bTime)
	}
	return a.ID < b.ID
}

func isInactiveFact(f qualityFact, cutoff time.Time) bool {
	t, ok := factActivityTime(f)
	if !ok {
		return true
	}
	return t.Before(cutoff)
}

// factActivityTime prefers last_used_at (actual retrieval). Never-recalled
// facts fall back to created_at so recently written unused facts are not
// marked inactive. updated_at is ignored: decay/cron rewrites would keep
// resetting "activity".
func factActivityTime(f qualityFact) (time.Time, bool) {
	if t, ok := parseFactTime(f.LastUsedAt); ok {
		return t, true
	}
	if t, ok := parseFactTime(f.CreatedAt); ok {
		return t, true
	}
	return time.Time{}, false
}

func parseFactTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
